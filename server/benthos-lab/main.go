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
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

type labState struct {
	Config string `json:"config"`
	Input  string `json:"input"`
}

func main() {
	logConf := log.NewConfig()
	logConf.Prefix = "benthos-lab"
	log := log.New(os.Stdout, logConf)

	cacheConf := cache.NewConfig()
	cache, err := cache.New(cacheConf, types.DudMgr{}, log, metrics.Noop())
	if err != nil {
		panic(err)
	}

	templateRegexp := regexp.MustCompile(`// BENTHOS LAB START([\n]|.)*// BENTHOS LAB END`)

	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir(".")))

	mux.HandleFunc("/l/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/l/")
		if len(path) == 0 {
			http.Error(w, "Path required", http.StatusBadRequest)
			log.Warnf("Bad path: %v\n", path)
			return
		}

		stateBody, err := cache.Get(path)
		if err != nil {
			http.Error(w, "Server failed", http.StatusBadGateway)
			log.Errorf("Failed to read state: %v\n", err)
			return
		}

		index, err := ioutil.ReadFile("./index.html")
		if err != nil {
			http.Error(w, "Server failed", http.StatusBadGateway)
			log.Errorf("Failed to read index: %v\n", path)
			return
		}

		index = templateRegexp.ReplaceAll(index, stateBody)

		w.Header().Set("Content-Type", "text/html")
		w.Write(index)
	})

	mux.HandleFunc("/share", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not supported", http.StatusBadRequest)
			log.Warnf("Bad method: %v\n", r.Method)
			return
		}
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			log.Errorf("Failed to read request body: %v\n", err)
			return
		}
		defer r.Body.Close()

		state := labState{}
		if err = json.Unmarshal(reqBody, &state); err != nil {
			http.Error(w, "Failed to parse body", http.StatusBadRequest)
			log.Errorf("Failed to parse request body: %v\n", err)
			return
		}

		if reqBody, err = json.Marshal(state); err != nil {
			http.Error(w, "Failed to parse body", http.StatusBadRequest)
			log.Errorf("Failed to normalise request body: %v\n", err)
			return
		}

		var buf bytes.Buffer

		hasher := md5.New()
		hasher.Write(reqBody)

		encoder := base64.NewEncoder(base64.URLEncoding, &buf)
		encoder.Write(hasher.Sum(nil))
		encoder.Close()

		hashBytes := buf.Bytes()

		if err = cache.Set(string(hashBytes), reqBody); err != nil {
			http.Error(w, "Save failed", http.StatusBadGateway)
			log.Errorf("Failed to store request body: %v\n", err)
			return
		}

		w.Write(hashBytes)
	})

	http.ListenAndServe(":8080", mux)
}

//------------------------------------------------------------------------------
