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
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

func hijackCode(code int, w http.ResponseWriter, r *http.Request, hijacker http.HandlerFunc) http.ResponseWriter {
	return &codeHijacker{
		w:        w,
		r:        r,
		code:     code,
		hijacker: hijacker,
	}
}

type codeHijacker struct {
	w http.ResponseWriter
	r *http.Request

	code     int
	hijacked bool

	hijacker http.HandlerFunc
}

func (c *codeHijacker) Header() http.Header {
	return c.w.Header()
}

func (c *codeHijacker) Write(msg []byte) (int, error) {
	if c.hijacked {
		c.hijacker(c.w, c.r)
		return len(msg), nil
	}
	return c.w.Write(msg)
}

func (c *codeHijacker) WriteHeader(statusCode int) {
	if statusCode == c.code {
		c.hijacked = true
	} else {
		c.w.WriteHeader(statusCode)
	}
}

//------------------------------------------------------------------------------

type labState struct {
	Config string `json:"config"`
	Input  string `json:"input"`
}

type benthosLabCache struct {
	path string
	log  log.Modular

	cache    []byte
	cachedAt time.Time

	sync.RWMutex
}

func newBenthosLabCache(path string, log log.Modular) *benthosLabCache {
	c := benthosLabCache{
		path: path,
		log:  log,
	}
	c.read()
	go c.loop()
	return &c
}

func (c *benthosLabCache) Get() []byte {
	c.RLock()
	cache := c.cache
	c.RUnlock()
	return cache
}

func (c *benthosLabCache) read() {
	finfo, err := os.Stat(c.path)
	if err != nil {
		c.log.Errorf("Failed to stat benthos-lab.wasm: %v\n", err)
		return
	}
	if finfo.ModTime().After(c.cachedAt) {
		c.log.Debugln("Reading modified benthos-lab.wasm")
		file, err := os.Open(c.path)
		if err != nil {
			c.log.Errorf("Failed to open benthos-lab.wasm: %v\n", err)
			return
		}
		var gzipBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&gzipBuf)
		if _, err = io.Copy(gzipWriter, file); err != nil {
			c.log.Errorf("Failed to compress benthos-lab.wasm: %v\n", err)
			return
		}
		gzipWriter.Close()
		c.Lock()
		c.cache = gzipBuf.Bytes()
		c.cachedAt = finfo.ModTime()
		c.Unlock()
	}
}

func (c *benthosLabCache) loop() {
	for {
		<-time.After(time.Second)
		c.read()
	}
}

func main() {
	cacheConf := cache.NewConfig()

	wwwPath := flag.String(
		"www", ".", "Path to the directory of client files to serve",
	)
	flag.StringVar(
		&cacheConf.Redis.URL, "redis-url", "", "Optional: Redis URL to use for caching",
	)
	flag.StringVar(
		&cacheConf.Redis.Expiration, "redis-ttl", cacheConf.Redis.Expiration, "Optional: Redis TTL to use for caching",
	)
	flag.Parse()

	if len(cacheConf.Redis.URL) > 0 {
		cacheConf.Type = "redis"
	}

	logConf := log.NewConfig()
	logConf.Prefix = "benthos-lab"
	log := log.New(os.Stdout, logConf)

	metricsConf := metrics.NewConfig()
	metricsConf.Prefix = "benthoslab"
	metricsConf.Type = "prometheus"
	stats, err := metrics.New(metricsConf)
	if err != nil {
		panic(err)
	}
	defer stats.Close()

	cacheConf.Memory.TTL = 259200
	cache, err := cache.New(cacheConf, types.DudMgr{}, log.NewModule(".cache"), metrics.Namespaced(stats, "cache"))
	if err != nil {
		panic(err)
	}

	labCache := newBenthosLabCache(filepath.Join(*wwwPath, "/wasm/benthos-lab.wasm"), log)

	mux := http.NewServeMux()
	fileServe := http.FileServer(http.Dir(*wwwPath))

	httpStats := metrics.Namespaced(stats, "http")
	mWASMGet200 := httpStats.GetCounter("wasm.get.200")
	mWASMGet304 := httpStats.GetCounter("wasm.get.304")
	mWASMGetNoGZIP := httpStats.GetCounter("wasm.no_gzip")

	notFoundHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Content-Type")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		notFoundFile, err := os.Open(filepath.Join(*wwwPath, "/404.html"))
		if err != nil {
			log.Errorf("Failed to open 404.html: %v\n", err)
			w.Write([]byte("Not found"))
			return
		}
		defer notFoundFile.Close()
		if _, err = io.Copy(w, notFoundFile); err != nil {
			log.Errorf("Failed to write 404.html: %v\n", err)
			w.Write([]byte("Not found"))
		}
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fileServe.ServeHTTP(hijackCode(http.StatusNotFound, w, r, notFoundHandler), r)
	})

	mux.HandleFunc("/wasm/benthos-lab.wasm", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			mWASMGetNoGZIP.Incr(1)
			fileServe.ServeHTTP(w, r)
			return
		}
		if since := r.Header.Get("If-Modified-Since"); len(since) > 0 {
			tSince, err := time.Parse(time.RFC1123, since)
			if err != nil {
				log.Errorf("Failed to parse time: %v\n", err)
			}
			if err == nil && labCache.cachedAt.Sub(tSince) < time.Second {
				mWASMGet304.Incr(1)
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		mWASMGet200.Incr(1)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Last-Modified", labCache.cachedAt.UTC().Format(time.RFC1123))
		w.Write(labCache.Get())
	})

	templateRegexp := regexp.MustCompile(`// BENTHOS LAB START([\n]|.)*// BENTHOS LAB END`)
	indexPath := filepath.Join(*wwwPath, "/index.html")

	mux.HandleFunc("/l/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/l/")
		if len(path) == 0 {
			http.Error(w, "Path required", http.StatusBadRequest)
			log.Warnf("Bad path: %v\n", path)
			return
		}

		stateBody, err := cache.Get(path)
		if err != nil {
			if err == types.ErrKeyNotFound {
				notFoundHandler(w, r)
			} else {
				http.Error(w, "Server failed", http.StatusBadGateway)
				log.Errorf("Failed to read state: %v\n", err)
			}
			return
		}

		index, err := ioutil.ReadFile(indexPath)
		if err != nil {
			http.Error(w, "Server failed", http.StatusBadGateway)
			log.Errorf("Failed to read index: %v\n", path)
			return
		}

		index = templateRegexp.ReplaceAllLiteral(index, stateBody)

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

		hasher := sha256.New()
		hasher.Write(reqBody)

		encoder := base64.NewEncoder(base64.URLEncoding, &buf)
		encoder.Write(hasher.Sum(nil))
		encoder.Close()

		hashBytes := buf.Bytes()

		if err = cache.Add(string(hashBytes), reqBody); err != nil && err != types.ErrKeyAlreadyExists {
			http.Error(w, "Save failed", http.StatusBadGateway)
			log.Errorf("Failed to store request body: %v\n", err)
			return
		}

		w.Write(hashBytes)
	})

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	if wHandlerFunc, ok := stats.(metrics.WithHandlerFunc); ok {
		adminMux.HandleFunc("/metrics", wHandlerFunc.HandlerFunc())
		adminMux.HandleFunc("/stats", wHandlerFunc.HandlerFunc())
	}

	log.Infoln("Listening for requests at :8080")
	go func() {
		log.Infoln("Listening for admin requests at :8081")
		if herr := http.ListenAndServe(":8081", adminMux); herr != nil {
			panic(herr)
		}
	}()
	if herr := http.ListenAndServe(":8080", mux); herr != nil {
		panic(herr)
	}
}

//------------------------------------------------------------------------------
