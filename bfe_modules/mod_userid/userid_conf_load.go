// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mod_userid

import (
	"fmt"
	"os"
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

// ConfigData the config for this module
type ConfigData struct {
	Version string
	Config  map[string][]ProductRuleData
}

// ProductRuleData config for one product line
type ProductRuleData struct {
	Cond   string
	Params *struct {
		Name   string
		Domain string
		Path   string
		MaxAge int
	}
}

func (cd *ConfigData) toConfig() (*Config, error) {
	config := &Config{
		Version:  cd.Version,
		Products: map[string][]ProductRule{},
	}

	for name, rules := range cd.Config {
		if len(rules) == 0 {
			return nil, fmt.Errorf("mod_user: product %s is nil", name)
		}

		for _, rule := range rules {
			cond, err := condition.Build(rule.Cond)
			if err != nil {
				return nil, err
			}

			if rule.Params == nil {
				return nil, fmt.Errorf("mod_user: product %s' Params is nil", name)
			}

			config.Products[name] = append(config.Products[name], ProductRule{
				Cond: cond,
				Params: ProductRuleParams{
					Name:   rule.Params.Name,
					Domain: rule.Params.Domain,
					Path:   rule.Params.Path,
					MaxAge: time.Duration(rule.Params.MaxAge) * time.Second,
				},
			})
		}
	}

	return config, nil

}

// ProductRuleParams config params
type ProductRuleParams struct {
	Name   string
	Domain string
	Path   string
	MaxAge time.Duration
}

// Config config
type Config struct {
	Version  string
	Products map[string][]ProductRule
}

// ProductRule  productRule
type ProductRule struct {
	Params ProductRuleParams
	Cond   condition.Condition
}

// NewConfigFromFile new one config
func NewConfigFromFile(fileName string) (*Config, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cd := &ConfigData{}
	if err := json.NewDecoder(file).Decode(cd); err != nil {
		return nil, err
	}

	return cd.toConfig()
}

// FindProductRules find by name。no locker because of nobody will write it
func (c *Config) FindProductRules(productName string) []ProductRule {
	return c.Products[productName]
}
