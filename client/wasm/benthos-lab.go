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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"syscall/js"
	"time"

	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/config"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/manager"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/pipeline"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/ratelimit"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
	uconf "github.com/Jeffail/benthos/lib/util/config"
	"gopkg.in/yaml.v3"
)

//------------------------------------------------------------------------------

// Create pipeline and output layers.
var pipelineLayer types.Pipeline
var closeFn = func() {}
var transactionChan chan types.Transaction

func writeOutput(msg, style string) {
	js.Global().Call("writeOutput", msg, style)
}

func reportErr(msg string, err error) {
	writeOutput("Error: "+fmt.Sprintf(msg, err), "errorMessage")
}

func reportLints(msg []string) {
	for _, m := range msg {
		writeOutput("Lint: "+m+"\n", "lintMessage")
	}
}

func compileConfig(confStr string) (config.Type, error) {
	conf := config.New()
	if err := yaml.Unmarshal([]byte(confStr), &conf); err != nil {
		return conf, err
	}
	return conf, nil
}

func share(this js.Value, args []js.Value) interface{} {
	config := js.Global().Get("configSession").Call("getValue").String()
	input := js.Global().Get("inputSession").Call("getValue").String()
	href := js.Global().Get("window").Get("location").Get("href").String()

	currentURL, err := url.Parse(href)
	if err != nil {
		reportErr("failed to parse current url: %v\n", err)
		return nil
	}
	currentURL.Path = "/share"

	state := struct {
		Config string `json:"config"`
		Input  string `json:"input"`
	}{
		Config: config,
		Input:  input,
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		reportErr("failed to marshal state: %v\n", err)
		return nil
	}

	go func() {
		res, err := http.Post(currentURL.String(), "application/json", bytes.NewReader(stateBytes))
		if err != nil {
			reportErr("failed to save state: %v\n", err)
			return
		}

		resBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			reportErr("failed to read save response: %v\n", err)
			return
		}

		if res.StatusCode != 200 {
			reportErr("failed to save state: %v\n", errors.New(string(resBytes)))
			return
		}

		currentURL.Path = "/l/" + string(resBytes)
		js.Global().Call("setShareURL", currentURL.String())
	}()
	return nil
}

func compile(this js.Value, args []js.Value) interface{} {
	closeFn()

	contents := js.Global().Get("configSession").Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}

	logger := log.WrapAtLevel(logWriter{}, log.LogInfo)
	mgr, err := manager.New(conf.Manager, types.NoopMgr(), logger, metrics.Noop())
	if err != nil {
		reportErr("failed to create pipeline resources: %v\n", err)
		return nil
	}

	if pipelineLayer, err = pipeline.New(conf.Pipeline, mgr, logger, metrics.Noop()); err == nil {
		err = pipelineLayer.Consume(transactionChan)
	}
	if err != nil {
		pipelineLayer = nil
		mgr.CloseAsync()
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}
	if lints, err := config.Lint([]byte(contents), conf); err != nil {
		reportErr("failed to parse config for linter: %v\n", err)
	} else if len(lints) > 0 {
		reportLints(lints)
	}

	writeOutput("Compiled successfully.\n", "infoMessage")
	compileBtn := js.Global().Get("document").Call("getElementById", "compileBtn")
	compileBtnClassList := compileBtn.Get("classList")
	compileBtnClassList.Call("add", "btn-disabled")
	compileBtnClassList.Call("remove", "btn-primary")
	compileBtn.Set("disabled", true)

	executeBtn := js.Global().Get("document").Call("getElementById", "executeBtn")
	executeClassList := executeBtn.Get("classList")
	executeClassList.Call("add", "btn-primary")
	executeClassList.Call("remove", "btn-disabled")
	executeBtn.Set("disabled", false)

	closeFn = func() {
		if pipelineLayer == nil {
			return
		}
		exitTimeout := time.Second * 30
		timesOut := time.Now().Add(exitTimeout)
		pipelineLayer.CloseAsync()
		if err := pipelineLayer.WaitForClose(time.Until(timesOut)); err != nil {
			reportErr("failed to shut down pipeline: %v\n", err)
		}
		if mgr != nil {
			mgr.CloseAsync()
		}
		pipelineLayer = nil
		mgr = nil
	}
	return nil
}

func marshalConf(conf config.Type) ([]byte, error) {
	sanit, err := conf.Sanitised()
	if err != nil {
		return nil, err
	}

	sanitBytes, err := uconf.MarshalYAML(map[string]interface{}{
		"pipeline":  sanit.Pipeline,
		"resources": sanit.Manager,
	})
	if err != nil {
		return nil, err
	}

	return sanitBytes, nil
}

func normalise(this js.Value, args []js.Value) interface{} {
	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()
	conf, err := compileConfig(contents)
	if err != nil {
		reportErr("failed to create pipeline: %v\n", err)
		return nil
	}

	sanitBytes, err := marshalConf(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	js.Global().Call("writeConfig", string(sanitBytes))
	return nil
}

func execute(this js.Value, args []js.Value) interface{} {
	if pipelineLayer == nil {
		reportErr("failed to execute: %v\n", errors.New("pipeline must be compiled first"))
		return nil
	}

	inputContent := js.Global().Get("inputSession").Call("getValue").String()
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

	go func(pipeLayer types.Pipeline) {
		resChan := make(chan types.Response, 1)
		for _, inputMsg := range inputMsgs {
			if inputMsg.Len() == 0 {
				continue
			}

			select {
			case transactionChan <- types.NewTransaction(inputMsg, resChan):
			case <-time.After(time.Second * 30):
				reportErr("failed to execute: %v\n", errors.New("request timed out"))
				return
			}

			var outTran types.Transaction
			select {
			case outTran = <-pipeLayer.TransactionChan():
			case res := <-resChan:
				if res.Error() != nil {
					reportErr("failed to execute: %v\n", res.Error())
					closeFn()
				} else {
					writeOutput("Pipeline executed without output.\n", "infoMessage")
				}
				return
			case <-time.After(time.Second * 30):
				reportErr("failed to execute: %v\n", errors.New("response timed out"))
				closeFn()
				return
			}

			select {
			case outTran.ResponseChan <- response.NewAck():
			case <-time.After(time.Second * 30):
				reportErr("failed to execute: %v\n", errors.New("response 2 timed out"))
				closeFn()
				return
			}

			select {
			case res := <-resChan:
				if res.Error() != nil {
					reportErr("failed to execute: %v\n", res.Error())
				}
			case <-time.After(time.Second * 30):
				reportErr("failed to execute: %v\n", errors.New("response 3 timed out"))
				closeFn()
				return
			}

			for _, out := range message.GetAllBytes(outTran.Payload) {
				writeOutput(string(out)+"\n", "")
			}
			writeOutput("\n", "")
		}
	}(pipelineLayer)
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

func addProc(this js.Value, args []js.Value) interface{} {
	value := this.Get("value").String()
	this.Set("value", "")
	if _, ok := processor.Constructors[value]; !ok {
		reportErr("Failed to add processor: %v\n", fmt.Errorf("processor type '%v' not recognised", value))
		return nil
	}
	procConf := processor.NewConfig()
	procConf.Type = value

	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()

	conf := config.New()
	if err := yaml.Unmarshal([]byte(contents), &conf); err != nil {
		reportErr("Failed to unmarshal current config: %v\n", err)
		return nil
	}

	conf.Pipeline.Processors = append(conf.Pipeline.Processors, procConf)
	resultBytes, err := marshalConf(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	js.Global().Call("writeConfig", string(resultBytes))
	return nil
}

func addCache(this js.Value, args []js.Value) interface{} {
	value := this.Get("value").String()
	this.Set("value", "")
	if _, ok := cache.Constructors[value]; !ok {
		reportErr("Failed to add cache: %v\n", fmt.Errorf("cache type '%v' not recognised", value))
		return nil
	}
	cacheConf := cache.NewConfig()
	cacheConf.Type = value

	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()

	conf := config.New()
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
	resultBytes, err := marshalConf(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	js.Global().Call("writeConfig", string(resultBytes))
	return nil
}

func addRatelimit(this js.Value, args []js.Value) interface{} {
	value := this.Get("value").String()
	this.Set("value", "")
	if _, ok := ratelimit.Constructors[value]; !ok {
		reportErr("Failed to add ratelimit: %v\n", fmt.Errorf("ratelimit type '%v' not recognised", value))
		return nil
	}
	ratelimitConf := ratelimit.NewConfig()
	ratelimitConf.Type = value

	session := js.Global().Get("configSession")
	contents := session.Call("getValue").String()

	conf := config.New()
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
	resultBytes, err := marshalConf(conf)
	if err != nil {
		reportErr("failed to normalise config: %v\n", err)
		return nil
	}

	js.Global().Call("writeConfig", string(resultBytes))
	return nil
}

//------------------------------------------------------------------------------

func addProcSelectOptions() {
	procSelect := js.Global().Get("document").Call("getElementById", "procSelect")
	procSelect.Call("addEventListener", "change", js.FuncOf(addProc))

	procs := []string{}
	for k := range processor.Constructors {
		procs = append(procs, k)
	}
	sort.Strings(procs)
	for _, k := range procs {
		option := js.Global().Get("document").Call("createElement", "option")
		option.Set("text", k)
		option.Set("value", k)
		procSelect.Get("options").Call("add", option)
	}
}

func addCacheSelectOptions() {
	cacheSelect := js.Global().Get("document").Call("getElementById", "cacheSelect")
	cacheSelect.Call("addEventListener", "change", js.FuncOf(addCache))

	caches := []string{}
	for k := range cache.Constructors {
		caches = append(caches, k)
	}
	sort.Strings(caches)
	for _, k := range caches {
		option := js.Global().Get("document").Call("createElement", "option")
		option.Set("text", k)
		option.Set("value", k)
		cacheSelect.Get("options").Call("add", option)
	}
}

func addRatelimitSelectOptions() {
	ratelimitSelect := js.Global().Get("document").Call("getElementById", "ratelimitSelect")
	ratelimitSelect.Call("addEventListener", "change", js.FuncOf(addRatelimit))

	ratelimits := []string{}
	for k := range ratelimit.Constructors {
		ratelimits = append(ratelimits, k)
	}
	sort.Strings(ratelimits)
	for _, k := range ratelimits {
		option := js.Global().Get("document").Call("createElement", "option")
		option.Set("text", k)
		option.Set("value", k)
		ratelimitSelect.Get("options").Call("add", option)
	}
}

//------------------------------------------------------------------------------

func registerFunctions() {
	executeFunc := js.FuncOf(execute)
	normaliseFunc := js.FuncOf(normalise)
	compileFunc := js.FuncOf(compile)
	shareFunc := js.FuncOf(share)

	js.Global().Get("document").Call("getElementById", "normaliseBtn").Call("addEventListener", "click", normaliseFunc)
	js.Global().Get("document").Call("getElementById", "executeBtn").Call("addEventListener", "click", executeFunc)
	js.Global().Get("document").Call("getElementById", "shareBtn").Call("addEventListener", "click", shareFunc)

	compileBtn := js.Global().Get("document").Call("getElementById", "compileBtn")
	compileBtn.Call("addEventListener", "click", compileFunc)
	compileBtnClassList := compileBtn.Get("classList")

	js.Global().Get("configSession").Call("on", "change", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		compileBtnClassList.Call("add", "btn-primary")
		compileBtnClassList.Call("remove", "btn-disabled")
		compileBtn.Set("disabled", false)
		return nil
	}))

	addProcSelectOptions()
	addCacheSelectOptions()
	addRatelimitSelectOptions()
}

func main() {
	c := make(chan struct{}, 0)

	println("WASM Benthos Initialized")
	transactionChan = make(chan types.Transaction, 1)

	registerFunctions()

	js.Global().Call("addEventListener", "beforeunload", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c <- struct{}{}
		return nil
	}))

	<-c
}

//------------------------------------------------------------------------------
