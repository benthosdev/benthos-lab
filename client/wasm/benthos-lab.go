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
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/output"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/ratelimit"
	"github.com/Jeffail/benthos/lib/stream"
	"github.com/Jeffail/benthos/lib/types"
	uconf "github.com/Jeffail/benthos/lib/util/config"
	"github.com/benthosdev/benthos-lab/lib/connectors"
	"gopkg.in/yaml.v3"
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

func registerConnectors() {
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
			wtr := connectors.StoreWriter{}
			return output.NewWriter("benthos_lab", wtr, logger, stats)
		},
	)
	output.DocumentPlugin("benthos_lab", "", func(conf interface{}) interface{} { return nil })
}

func compile(this js.Value, args []js.Value) interface{} {
	contents := args[0].String()
	conf, err := unmarshalConfig(contents)
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

func newConfig() config.Type {
	conf := config.New()
	conf.Input.Type = "benthos_lab"
	conf.Output.Type = "benthos_lab"
	return conf
}

func unmarshalConfig(confStr string) (config.Type, error) {
	conf := newConfig()
	if err := yaml.Unmarshal([]byte(confStr), &conf); err != nil {
		return conf, err
	}
	return conf, nil
}

type normalisedLabConfig struct {
	Input     interface{} `yaml:"input"`
	Buffer    interface{} `yaml:"buffer"`
	Pipeline  interface{} `yaml:"pipeline"`
	Output    interface{} `yaml:"output"`
	Resources interface{} `yaml:"resources"`
}

func marshalConfig(conf config.Type) ([]byte, error) {
	sanit, err := conf.Sanitised()
	if err != nil {
		return nil, err
	}

	return uconf.MarshalYAML(normalisedLabConfig{
		Input:     sanit.Input,
		Buffer:    sanit.Buffer,
		Pipeline:  sanit.Pipeline,
		Output:    sanit.Output,
		Resources: sanit.Manager,
	})
}

func normalise(this js.Value, args []js.Value) interface{} {
	contents := args[0].String()
	conf, err := unmarshalConfig(contents)
	if err != nil {
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}

	sanitBytes, err := marshalConfig(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	return string(sanitBytes)
}

//------------------------------------------------------------------------------

func addProcessor(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	if _, ok := processor.Constructors[procType]; !ok {
		reportErr("Failed to add processor: %v\n", fmt.Errorf("processor type '%v' not recognised", procType))
		return nil
	}
	procConf := processor.NewConfig()
	procConf.Type = procType

	conf := newConfig()
	if err := yaml.Unmarshal([]byte(contents), &conf); err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	conf.Pipeline.Processors = append(conf.Pipeline.Processors, procConf)
	resultBytes, err := marshalConfig(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}
	return string(resultBytes)
}

func addCache(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	if _, ok := cache.Constructors[procType]; !ok {
		reportErr("Failed to add cache: %v\n", fmt.Errorf("cache type '%v' not recognised", procType))
		return nil
	}
	cacheConf := cache.NewConfig()
	cacheConf.Type = procType

	conf := newConfig()
	if err := yaml.Unmarshal([]byte(contents), &conf); err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	var cacheID string
	for i := 0; i < 10000; i++ {
		var candidate string
		if i == 0 {
			candidate = "example"
		} else {
			candidate = fmt.Sprintf("example%v", i)
		}
		if _, exists := conf.Manager.Caches[candidate]; !exists {
			cacheID = candidate
			break
		}
	}
	if len(cacheID) == 0 {
		reportErr("Failed to find an ID for your new cache", errors.New("what the hell are you doing?"))
		return nil
	}

	conf.Manager.Caches[cacheID] = cacheConf
	resultBytes, err := marshalConfig(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	return string(resultBytes)
}

func addRatelimit(this js.Value, args []js.Value) interface{} {
	procType, contents := args[0].String(), args[1].String()
	if _, ok := ratelimit.Constructors[procType]; !ok {
		reportErr("Failed to add ratelimit: %v\n", fmt.Errorf("ratelimit type '%v' not recognised", procType))
		return nil
	}
	ratelimitConf := ratelimit.NewConfig()
	ratelimitConf.Type = procType

	conf := newConfig()
	if err := yaml.Unmarshal([]byte(contents), &conf); err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	var ratelimitID string
	for i := 0; i < 10000; i++ {
		var candidate string
		if i == 0 {
			candidate = "example"
		} else {
			candidate = fmt.Sprintf("example%v", i)
		}
		if _, exists := conf.Manager.RateLimits[candidate]; !exists {
			ratelimitID = candidate
			break
		}
	}
	if len(ratelimitID) == 0 {
		reportErr("Failed to find an ID for your new ratelimit", errors.New("what the hell are you doing?"))
		return nil
	}

	conf.Manager.RateLimits[ratelimitID] = ratelimitConf
	resultBytes, err := marshalConfig(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	return string(resultBytes)
}

//------------------------------------------------------------------------------

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

func registerFunctions() {
	benthosLab := js.Global().Get("benthosLab")
	if benthosLab.Type() == js.TypeUndefined {
		benthosLab = js.ValueOf(map[string]interface{}{})
		js.Global().Set("benthosLab", benthosLab)
	}

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

	benthosLab.Set("getProcessors", js.FuncOf(getProcessors))
	benthosLab.Set("getCaches", js.FuncOf(getCaches))
	benthosLab.Set("getRatelimits", js.FuncOf(getRatelimits))
	benthosLab.Set("addProcessor", js.FuncOf(addProcessor))
	benthosLab.Set("addCache", js.FuncOf(addCache))
	benthosLab.Set("addRatelimit", js.FuncOf(addRatelimit))
	benthosLab.Set("normalise", js.FuncOf(normalise))
	benthosLab.Set("compile", js.FuncOf(compile))
	benthosLab.Set("execute", js.FuncOf(execute))
}

func main() {
	c := make(chan struct{}, 0)

	registerConnectors()
	registerFunctions()

	println("WASM Benthos Initialized")
	onLoad()

	js.Global().Call("addEventListener", "beforeunload", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c <- struct{}{}
		return nil
	}))

	<-c
}

//------------------------------------------------------------------------------
