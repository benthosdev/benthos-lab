// Copyright (c) 2019 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/config"
	"github.com/Jeffail/benthos/lib/input"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/manager"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/message/roundtrip"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/output"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/ratelimit"
	"github.com/Jeffail/benthos/lib/stream"
	"github.com/Jeffail/benthos/lib/types"
	labConfig "github.com/benthosdev/benthos-lab/lib/config"
	"github.com/benthosdev/benthos-lab/lib/connectors"
)

//------------------------------------------------------------------------------

// Build stamps.
var (
	Version string
)

//------------------------------------------------------------------------------

var writeFunc js.Value

func writeOutput(msg, style string) {
	writeFunc.Invoke(msg, style)
}

func reportErr(msg string, err error) {
	writeOutput("Error: "+fmt.Sprintf(msg, err), "errorMessage")
}

func reportLints(msg []string) {
	for _, m := range msg {
		writeOutput("Lint: "+m+"\n", "lintMessage")
	}
}

//------------------------------------------------------------------------------

type streamState struct {
	str           *stream.Type
	mgr           *manager.Type
	consumerChans []chan types.Message

	sync.RWMutex
}

func (s *streamState) Register(c chan types.Message) {
	s.Lock()
	s.consumerChans = append(s.consumerChans, c)
	s.Unlock()
}

func (s *streamState) SendAll(msgs []types.Message) {
	s.RLock()
	defer s.RUnlock()
	for _, inputMsg := range msgs {
		if inputMsg.Len() == 0 {
			continue
		}

		for _, c := range s.consumerChans {
			select {
			case c <- inputMsg:
			case <-time.After(time.Second * 30):
				reportErr("failed to execute: %v\n", errors.New("send timed out"))
				return
			}
		}
	}
}

func (s *streamState) Clear() {
	s.Lock()
	for _, c := range s.consumerChans {
		close(c)
	}
	s.consumerChans = nil
	if s.str != nil {
		exitTimeout := time.Second * 30
		timesOut := time.Now().Add(exitTimeout)
		if err := s.str.Stop(time.Until(timesOut)); err != nil {
			reportErr("failed to cleanly shut down pipeline: %v\n", err)
		}
		s.str = nil
	}
	if s.mgr != nil {
		s.mgr.CloseAsync()
		s.mgr = nil
	}
	s.Unlock()
}

func (s *streamState) Set(str *stream.Type, mgr *manager.Type) {
	s.Lock()
	s.str = str
	s.mgr = mgr
	s.Unlock()
}

var state = &streamState{}

//------------------------------------------------------------------------------

func registerConnectors() func() {
	input.RegisterPlugin(
		"benthos_lab",
		func() interface{} {
			s := struct{}{}
			return &s
		},
		func(_ interface{}, _ types.Manager, logger log.Modular, stats metrics.Type) (types.Input, error) {
			batchChan := make(chan types.Message)
			state.Register(batchChan)
			rdr := connectors.NewRoundTripReader(func() (types.Message, error) {
				select {
				case m, open := <-batchChan:
					if open {
						return m, nil
					}
				}
				return nil, types.ErrTypeClosed
			}, func(msgs []types.Message, err error) {
				if err != nil {
					reportErr("pipeline error: %v\n", err)
					return
				}
				if len(msgs) == 0 {
					writeOutput("Pipeline execution resulted in zero messages.\n", "infoMessage")
					return
				}
				for _, m := range msgs {
					for _, out := range message.GetAllBytes(m) {
						writeOutput(string(out)+"\n", "")
					}
					writeOutput("\n", "")
				}
			})
			return input.NewReader("benthos_lab", rdr, logger, stats)
		},
	)
	input.DocumentPlugin("benthos_lab", "", func(conf interface{}) interface{} { return nil })
	output.RegisterPlugin(
		"benthos_lab",
		func() interface{} {
			s := struct{}{}
			return &s
		},
		func(_ interface{}, _ types.Manager, logger log.Modular, stats metrics.Type) (types.Output, error) {
			wtr := roundtrip.Writer{}
			return output.NewWriter("benthos_lab", wtr, logger, stats)
		},
	)
	output.DocumentPlugin("benthos_lab", "", func(conf interface{}) interface{} { return nil })

	return func() {
		state.Clear()
	}
}

func compile(this js.Value, args []js.Value) interface{} {
	contents := args[0].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}

	successFunc := args[1]
	go func() {
		state.Clear()

		logger := log.WrapAtLevel(logWriter{}, log.LogInfo)
		mgr, err := manager.New(conf.Manager, types.NoopMgr(), logger, metrics.Noop())
		if err != nil {
			reportErr("failed to create pipeline resources: %v\n", err)
			return
		}

		str, err := stream.New(conf.Config, stream.OptSetLogger(logger), stream.OptSetManager(mgr))
		if err != nil {
			mgr.CloseAsync()
			reportErr("failed to create pipeline: %v\n", err)
			return
		}

		state.Set(str, mgr)

		if lints, err := config.Lint([]byte(contents), conf); err != nil {
			reportErr("failed to parse config for linter: %v\n", err)
		} else if len(lints) > 0 {
			reportLints(lints)
		}

		writeOutput("Compiled successfully.\n", "infoMessage")
		if successFunc.Type() == js.TypeFunction {
			successFunc.Invoke()
		}
	}()
	return nil
}

func execute(this js.Value, args []js.Value) interface{} {
	inputContent := args[0].String()
	lines := strings.Split(inputContent, "\n")

	inputMsgs := []types.Message{}
	inputMsgs = append(inputMsgs, message.New(nil))
	for _, line := range lines {
		if len(line) == 0 {
			if inputMsgs[len(inputMsgs)-1].Len() > 0 {
				inputMsgs = append(inputMsgs, message.New(nil))
			}
			continue
		}
		inputMsgs[len(inputMsgs)-1].Append(message.NewPart([]byte(line)))
	}

	go state.SendAll(inputMsgs)
	return nil
}

//------------------------------------------------------------------------------

type logWriter struct{}

func (l logWriter) Printf(format string, v ...interface{}) {
	writeOutput("Log: "+fmt.Sprintf(format, v...), "logMessage")
}

func (l logWriter) Println(v ...interface{}) {
	if str, ok := v[0].(string); ok {
		writeOutput("Log: "+fmt.Sprintf(str, v[1:]...)+"\n", "logMessage")
	} else {
		writeOutput("Log: "+fmt.Sprintf("%v\n", v), "logMessage")
	}
}

//------------------------------------------------------------------------------

func normalise(this js.Value, args []js.Value) interface{} {
	contents := args[0].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}

	sanitBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	if args[1].Type() == js.TypeFunction {
		args[1].Invoke(string(sanitBytes))
	}
	return nil
}

//------------------------------------------------------------------------------

func addInput(this js.Value, args []js.Value) interface{} {
	inputType, contents := args[0].String(), args[1].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	if err := labConfig.AddInput(inputType, &conf); err != nil {
		reportErr("Failed to add input: %v\n", err)
		return nil
	}

	resultBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}
	return string(resultBytes)
}

func addProcessor(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	if err := labConfig.AddProcessor(procType, &conf); err != nil {
		reportErr("Failed to add processor: %v\n", err)
		return nil
	}

	resultBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}
	return string(resultBytes)
}

func addOutput(this js.Value, args []js.Value) interface{} {
	outputType, contents := args[0].String(), args[1].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	if err := labConfig.AddOutput(outputType, &conf); err != nil {
		reportErr("Failed to add output: %v\n", err)
		return nil
	}

	resultBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}
	return string(resultBytes)
}

func addCache(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	if err := labConfig.AddCache(procType, &conf); err != nil {
		reportErr("Failed to add cache: %v\n", err)
		return nil
	}

	resultBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	return string(resultBytes)
}

func addRatelimit(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	conf, err := labConfig.Unmarshal(contents)
	if err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	if err := labConfig.AddRatelimit(procType, &conf); err != nil {
		reportErr("Failed to add cache: %v\n", err)
		return nil
	}

	resultBytes, err := labConfig.Marshal(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	return string(resultBytes)
}

//------------------------------------------------------------------------------

func getInputs(this js.Value, args []js.Value) interface{} {
	inputs := []string{"benthos_lab"}
	for k := range input.Constructors {
		inputs = append(inputs, k)
	}
	sort.Strings(inputs)
	generic := make([]interface{}, len(inputs))
	for i, v := range inputs {
		generic[i] = v
	}
	return generic
}

func getProcessors(this js.Value, args []js.Value) interface{} {
	procs := []string{}
	for k := range processor.Constructors {
		procs = append(procs, k)
	}
	sort.Strings(procs)
	generic := make([]interface{}, len(procs))
	for i, v := range procs {
		generic[i] = v
	}
	return generic
}

func getOutputs(this js.Value, args []js.Value) interface{} {
	outputs := []string{"benthos_lab"}
	for k := range output.Constructors {
		outputs = append(outputs, k)
	}
	sort.Strings(outputs)
	generic := make([]interface{}, len(outputs))
	for i, v := range outputs {
		generic[i] = v
	}
	return generic
}

func getCaches(this js.Value, args []js.Value) interface{} {
	caches := []string{}
	for k := range cache.Constructors {
		caches = append(caches, k)
	}
	sort.Strings(caches)
	generic := make([]interface{}, len(caches))
	for i, v := range caches {
		generic[i] = v
	}
	return generic
}

func getRatelimits(this js.Value, args []js.Value) interface{} {
	ratelimits := []string{}
	for k := range ratelimit.Constructors {
		ratelimits = append(ratelimits, k)
	}
	sort.Strings(ratelimits)
	generic := make([]interface{}, len(ratelimits))
	for i, v := range ratelimits {
		generic[i] = v
	}
	return generic
}

//------------------------------------------------------------------------------

var onLoad func()

func registerFunctions() func() {
	benthosLab := js.Global().Get("benthosLab")
	if benthosLab.Type() == js.TypeUndefined {
		benthosLab = js.ValueOf(map[string]interface{}{})
		js.Global().Set("benthosLab", benthosLab)
	}

	if len(Version) == 0 {
		Version = "Unknown"
	}
	benthosLab.Set("version", Version)

	if print := benthosLab.Get("print"); print.Type() == js.TypeFunction {
		writeFunc = print
	} else {
		writeFunc = js.Global().Get("console").Get("log")
		benthosLab.Set("print", writeFunc)
	}

	if onLoadVal := benthosLab.Get("onLoad"); onLoadVal.Type() == js.TypeFunction {
		onLoad = func() {
			onLoadVal.Invoke()
		}
	} else {
		onLoad = func() {}
	}

	var fields []string
	var funcs []js.Func
	addLabFunction := func(name string, fn js.Func) {
		funcs = append(funcs, fn)
		fields = append(fields, name)
		benthosLab.Set(name, fn)
	}

	addLabFunction("getInputs", js.FuncOf(getInputs))
	addLabFunction("getProcessors", js.FuncOf(getProcessors))
	addLabFunction("getOutputs", js.FuncOf(getOutputs))
	addLabFunction("getCaches", js.FuncOf(getCaches))
	addLabFunction("getRatelimits", js.FuncOf(getRatelimits))
	addLabFunction("addInput", js.FuncOf(addInput))
	addLabFunction("addProcessor", js.FuncOf(addProcessor))
	addLabFunction("addOutput", js.FuncOf(addOutput))
	addLabFunction("addCache", js.FuncOf(addCache))
	addLabFunction("addRatelimit", js.FuncOf(addRatelimit))
	addLabFunction("normalise", js.FuncOf(normalise))
	addLabFunction("compile", js.FuncOf(compile))
	addLabFunction("execute", js.FuncOf(execute))

	return func() {
		for _, field := range fields {
			benthosLab.Set(field, js.Undefined())
		}
		for _, fn := range funcs {
			fn.Release()
		}
	}
}

func main() {
	c := make(chan struct{}, 0)

	defer registerConnectors()()
	defer registerFunctions()()

	println("WASM Benthos Initialized")
	onLoad()

	/*
		TODO: Add this when Benthos stops being naughty.
		info, ok := debug.ReadBuildInfo()
		if ok {
			for _, mod := range info.Deps {
				if mod.Path == "github.com/Jeffail/benthos" {
					writeOutput(fmt.Sprintf("Benthos version: %v\n", mod.Version), "infoMessage")
				}
			}
		}
	*/

	js.Global().Call("addEventListener", "beforeunload", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c <- struct{}{}
		return nil
	}))

	<-c
}

//------------------------------------------------------------------------------
