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
	"strings"
	"syscall/js"
	"time"

	"github.com/Jeffail/benthos/lib/config"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/pipeline"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
	uconf "github.com/Jeffail/benthos/lib/util/config"
	"gopkg.in/yaml.v3"
)

//------------------------------------------------------------------------------

// Build stamps.
var (
	Version   string
	DateBuilt string
)

//------------------------------------------------------------------------------

// Create pipeline and output layers.
var pipelineLayer types.Pipeline
var transactionChan chan types.Transaction

func writeOutput(msg string) {
	js.Global().Call("writeOutput", msg)
}

func reportErr(msg string, err error) {
	writeOutput(fmt.Sprintf(msg, err))
}

func reportLints(msg []string) {
	for _, m := range msg {
		writeOutput("Lint: " + m + "\n")
	}
}

func compileConfig(confStr string) (config.Type, error) {
	conf := config.New()
	if err := yaml.Unmarshal([]byte(confStr), &conf); err != nil {
		return conf, err
	}
	return conf, nil
}

func closePipeline() {
	if pipelineLayer == nil {
		return
	}
	exitTimeout := time.Second * 30
	timesOut := time.Now().Add(exitTimeout)
	pipelineLayer.CloseAsync()
	if err := pipelineLayer.WaitForClose(time.Until(timesOut)); err != nil {
		reportErr("Error: Failed to shut down pipeline: %v\n", err)
	}
	pipelineLayer = nil
}

func compile(this js.Value, args []js.Value) interface{} {
	closePipeline()

	contents := js.Global().Get("configSession").Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("Error: Failed to create pipeline: %v\n", err)
		return nil
	}

	logger := log.WrapAtLevel(logWriter{}, log.LogError)
	if pipelineLayer, err = pipeline.New(conf.Pipeline, types.NoopMgr(), logger, metrics.Noop()); err == nil {
		err = pipelineLayer.Consume(transactionChan)
	}
	if err != nil {
		pipelineLayer = nil
		reportErr("Error: Failed to create pipeline: %v\n", err)
		return nil
	}
	if lints, err := config.Lint([]byte(contents), conf); err != nil {
		reportErr("Error: Failed to parse config for linter: %v\n", err)
	} else if len(lints) > 0 {
		reportLints(lints)
	}

	writeOutput("Compiled successfully.\n")
	compileBtn := js.Global().Get("document").Call("getElementById", "compileBtn")
	compileBtnClassList := compileBtn.Get("classList")
	compileBtnClassList.Call("add", "btn-disabled")
	compileBtnClassList.Call("remove", "btn-primary")
	compileBtn.Set("disabled", true)
	return nil
}

func normalise(this js.Value, args []js.Value) interface{} {
	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("Error: Failed to create pipeline: %v\n", err)
		return nil
	}

	sanit, err := conf.Sanitised()
	if err != nil {
		reportErr("Error: Failed to normalise config: %v\n", err)
		return nil
	}

	sanitBytes, err := uconf.MarshalYAML(map[string]interface{}{
		"pipeline": sanit.Pipeline,
	})
	if err != nil {
		reportErr("Error: Failed to marshal normalised config: %v\n", err)
		return nil
	}

	js.Global().Call("writeConfig", string(sanitBytes))
	return nil
}

func execute(this js.Value, args []js.Value) interface{} {
	if pipelineLayer == nil {
		reportErr("Error: Failed to execute: %v\n", errors.New("pipeline must be compiled first"))
		return nil
	}

	inputContent := js.Global().Get("inputSession").Call("getValue").String()
	lines := strings.Split(inputContent, "\n")
	inputMsg := message.New(nil)
	for _, line := range lines {
		inputMsg.Append(message.NewPart([]byte(line)))
	}

	resChan := make(chan types.Response, 1)
	select {
	case transactionChan <- types.NewTransaction(inputMsg, resChan):
	case <-time.After(time.Second * 30):
		reportErr("Error: Failed to execute: %v\n", errors.New("request timed out"))
		return nil
	}

	var outTran types.Transaction
	select {
	case outTran = <-pipelineLayer.TransactionChan():
	case <-time.After(time.Second * 30):
		reportErr("Error: Failed to execute: %v\n", errors.New("response timed out"))
		closePipeline()
		return nil
	}

	select {
	case outTran.ResponseChan <- response.NewAck():
	case <-time.After(time.Second * 30):
		reportErr("Error: Failed to execute: %v\n", errors.New("response 2 timed out"))
		closePipeline()
		return nil
	}

	for _, out := range message.GetAllBytes(outTran.Payload) {
		writeOutput(string(out) + "\n")
	}
	return nil
}

//------------------------------------------------------------------------------

type logWriter struct{}

func (l logWriter) Printf(format string, v ...interface{}) {
	writeOutput("Log: " + fmt.Sprintf(format, v...))
}

func (l logWriter) Println(v ...interface{}) {
	if str, ok := v[0].(string); ok {
		writeOutput("Log: " + fmt.Sprintf(str, v[1:]...) + "\n")
	} else {
		writeOutput("Log: " + fmt.Sprintf("%v\n", v))
	}
}

//------------------------------------------------------------------------------

// Blacklist of all non-permitted processor types.
var processorBlacklist = []string{
	"cache",  // TODO
	"dedupe", // TODO
	"http",
	"lambda",
	"sql",
	"subprocess",
}

//------------------------------------------------------------------------------

func registerFunctions() {
	executeFunc := js.FuncOf(execute)
	normaliseFunc := js.FuncOf(normalise)
	compileFunc := js.FuncOf(compile)

	js.Global().Get("document").Call("getElementById", "normaliseBtn").Call("addEventListener", "click", normaliseFunc)
	js.Global().Get("document").Call("getElementById", "executeBtn").Call("addEventListener", "click", executeFunc)

	compileBtn := js.Global().Get("document").Call("getElementById", "compileBtn")
	compileBtn.Call("addEventListener", "click", compileFunc)
	compileBtnClassList := compileBtn.Get("classList")

	js.Global().Get("configSession").Call("on", "change", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		compileBtnClassList.Call("add", "btn-primary")
		compileBtnClassList.Call("remove", "btn-disabled")
		compileBtn.Set("disabled", false)
		return nil
	}))
}

func main() {
	c := make(chan struct{}, 0)

	println("WASM Benthos Initialized")
	transactionChan = make(chan types.Transaction, 1)

	for _, k := range processorBlacklist {
		processor.Block(k, "benthos labs cannot execute it")
	}

	registerFunctions()
	<-c
}

//------------------------------------------------------------------------------
