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
	"github.com/Jeffail/benthos/v3/lib/config"
	uconf "github.com/Jeffail/benthos/v3/lib/util/config"
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
	Input     yaml.Node `yaml:"input"`
	Buffer    yaml.Node `yaml:"buffer"`
	Pipeline  yaml.Node `yaml:"pipeline"`
	Output    yaml.Node `yaml:"output"`
	Resources yaml.Node `yaml:"resources,omitempty"`
}

// Marshal a config struct into a subset of fields relevant to the lab
// environment.
func Marshal(conf config.Type) ([]byte, error) {
	node, err := conf.SanitisedV2(config.SanitisedV2Config{
		RemoveTypeField:        false,
		RemoveDeprecatedFields: false,
	})
	if err != nil {
		return nil, err
	}
	nConf := normalisedLabConfig{}
	if err := node.Decode(&nConf); err != nil {
		return nil, err
	}
	return uconf.MarshalYAML(nConf)
}

//------------------------------------------------------------------------------
