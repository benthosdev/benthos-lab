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

package config

import (
	"errors"
	"fmt"

	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/config"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/ratelimit"
	uconf "github.com/Jeffail/benthos/lib/util/config"
	"gopkg.in/yaml.v3"
)

//------------------------------------------------------------------------------

// New creates a fresh Benthos config with defaults that suit the lab
// environment.
func New() config.Type {
	conf := config.New()
	conf.Input.Type = "benthos_lab"
	conf.Output.Type = "benthos_lab"
	return conf
}

// Unmarshal a config string into a config struct with defaults that suit the
// lab environment.
func Unmarshal(confStr string) (config.Type, error) {
	conf := New()
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

// Marshal a config struct into a subset of fields relevant to the lab
// environment.
func Marshal(conf config.Type) ([]byte, error) {
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

//------------------------------------------------------------------------------

// AddProcessor inserts a default processor of a type to an existing config.
func AddProcessor(cType string, conf *config.Type) error {
	if _, ok := processor.Constructors[cType]; !ok {
		return fmt.Errorf("processor type '%v' not recognised", cType)
	}
	procConf := processor.NewConfig()
	procConf.Type = cType

	conf.Pipeline.Processors = append(conf.Pipeline.Processors, procConf)
	return nil
}

// AddCache inserts a default cache of a type to an existing config.
func AddCache(cType string, conf *config.Type) error {
	if _, ok := cache.Constructors[cType]; !ok {
		return fmt.Errorf("cache type '%v' not recognised", cType)
	}
	cacheConf := cache.NewConfig()
	cacheConf.Type = cType

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
		return errors.New("what the hell are you doing?")
	}

	conf.Manager.Caches[cacheID] = cacheConf
	return nil
}

// AddRatelimit inserts a default rate limit of a type to an existing config.
func AddRatelimit(cType string, conf *config.Type) error {
	if _, ok := ratelimit.Constructors[cType]; !ok {
		return fmt.Errorf("ratelimit type '%v' not recognised", cType)
	}
	ratelimitConf := ratelimit.NewConfig()
	ratelimitConf.Type = cType

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
		return errors.New("what the hell are you doing?")
	}

	conf.Manager.RateLimits[ratelimitID] = ratelimitConf
	return nil
}

//------------------------------------------------------------------------------
