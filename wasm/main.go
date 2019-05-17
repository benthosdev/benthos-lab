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

func reportErr(msg string, err error) {
	js.Global().Call("writeOutput", fmt.Sprintf(msg, err))
}

func reportLints(msg []string) {
	for _, m := range msg {
		js.Global().Call("writeOutput", m)
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
}

func compile(this js.Value, args []js.Value) interface{} {
	closePipeline()

	contents := js.Global().Get("configSession").Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("Error: Failed to create pipeline: %v", err)
		return nil
	}

	if pipelineLayer, err = pipeline.New(conf.Pipeline, types.NoopMgr(), log.Noop(), metrics.Noop()); err == nil {
		err = pipelineLayer.Consume(transactionChan)
	}
	if err != nil {
		reportErr("Error: Failed to create pipeline: %v\n", err)
		return nil
	}
	lints, err := config.Lint([]byte(contents), conf)
	if err != nil {
		reportErr("Error: Failed to parse config: %v\n", err)
		return nil
	}

	if len(lints) > 0 {
		reportLints(lints)
	}

	js.Global().Call("writeOutput", "Compiled successfully.\n")
	return nil
}

func normalise(this js.Value, args []js.Value) interface{} {
	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("Error: Failed to create pipeline: %v", err)
		return nil
	}

	sanit, err := conf.Sanitised()
	if err != nil {
		reportErr("Error: Failed to normalise config: %v", err)
		return nil
	}

	sanitBytes, err := uconf.MarshalYAML(map[string]interface{}{
		"pipeline": sanit.Pipeline,
	})
	if err != nil {
		reportErr("Error: Failed to marshal normalised config: %v", err)
		return nil
	}

	js.Global().Call("writeConfig", string(sanitBytes))
	return nil
}

func execute(this js.Value, args []js.Value) interface{} {
	if pipelineLayer == nil {
		reportErr("Failed to execute: %v\n", errors.New("pipeline must be compiled first"))
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
		js.Global().Call("writeOutput", string(out)+"\n")
	}
	return nil
}

func registerFunctions() {
	executeFunc := js.FuncOf(execute)
	normaliseFunc := js.FuncOf(normalise)
	compileFunc := js.FuncOf(compile)

	js.Global().Get("document").Call("getElementById", "normaliseBtn").Call("addEventListener", "click", normaliseFunc)
	js.Global().Get("document").Call("getElementById", "compileBtn").Call("addEventListener", "click", compileFunc)
	js.Global().Get("document").Call("getElementById", "executeBtn").Call("addEventListener", "click", executeFunc)
}

func main() {
	c := make(chan struct{}, 0)

	println("WASM Benthos Initialized")
	transactionChan = make(chan types.Transaction, 1)

	registerFunctions()
	<-c
}

//------------------------------------------------------------------------------
