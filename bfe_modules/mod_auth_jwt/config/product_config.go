// Copyright (c) 2019 Baidu, Inc.
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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

import (
	"github.com/baidu/bfe/bfe_basic/condition"
	"github.com/baidu/bfe/bfe_util"
)

// auth rule (config for each product)
type AuthRule struct {
	AuthConfig
	keys []string // keys that represented explicitly in config file
	Cond condition.Condition
}

// product config
type ProductConfig struct {
	path    string // config file path
	root    string // the directory of `path`
	Version string
	Config  map[string]*AuthRule
}

func NewProductConfig(path string) (config *ProductConfig, err error) {
	config = new(ProductConfig)

	err = config.Update(path)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// Update updates config from the given new path
func (config *ProductConfig) Update(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	defer file.Close()

	data := make(map[string]interface{})
	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return err
	}

	root, _ := filepath.Split(path)
	// ensures atomicity
	ptr, config := config, &ProductConfig{root: root, path: path}

	// convert map to struct ProductConfig
	err = config.convert(data)
	if err != nil {
		return err
	}

	// perform check action
	err = config.Check()
	if err != nil {
		return err
	}

	*ptr = *config // ensures atomicity

	return nil
}

// Reload reloads config from config.path
func (config *ProductConfig) Reload() (err error) {
	return config.Update(config.path)
}

// convert converts map to ProductConfig and update to self
func (config *ProductConfig) convert(m map[string]interface{}) (err error) {
	version, ok := m["Version"].(string)
	if !ok {
		return fmt.Errorf("invalid value type for config parameter `Version`, expected string")
	}

	config.Version = version

	// {product: rule}
	rules, ok := m["Config"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for config parameter `Config`, expected map[string]interface{}")
	}

	newConfig := make(map[string]*AuthRule, len(rules))

	for product, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid value type for value of `Config`, expected map[string]interface{}")
		}

		condStr, ok := ruleMap["Cond"].(string)
		if !ok {
			return fmt.Errorf("invalid value type for `Cond` in product rule: %s", product)
		}

		// build condition
		cond, err := condition.Build(condStr)
		if err != nil {
			return err
		}

		authRule := &AuthRule{keys: MapKeys(ruleMap), Cond: cond}
		err = MapConvert(ruleMap, &authRule.AuthConfig) // convert ruleMap to struct AuthConfig
		if err != nil {
			return err
		}

		// join the path parameter
		if len(authRule.SecretPath) > 0 {
			authRule.SecretPath = bfe_util.ConfPathProc(authRule.SecretPath, config.root)
		}

		newConfig[product] = authRule
	}

	config.Config = newConfig

	return nil
}

// Check checks whether the config item were valid or not
func (config *ProductConfig) Check() (err error) {
	for _, rule := range config.Config {
		if len(rule.SecretPath) == 0 {
			continue
		}

		err = AssertsFile(rule.SecretPath)
		if err != nil {
			return err
		}

		err = rule.BuildSecret()
		if err != nil {
			return err
		}
	}

	return nil
}

// Merge merging missed AuthRule item for each product from ModuleConfig
func (config *ProductConfig) Merge(moduleConfig *ModuleConfig) {
	src := reflect.ValueOf(moduleConfig.Basic.AuthConfig)

	for _, rule := range config.Config {
		dst := reflect.Indirect(reflect.ValueOf(&rule.AuthConfig))

		for i, l := 0, src.NumField(); i < l; i++ {
			field := src.Type().Field(i)
			target := dst.FieldByName(field.Name)
			if Contains(rule.keys, field.Name) || !target.CanSet() {
				continue
			}

			if field.Name == "Secret" && rule.SecretPath != moduleConfig.Basic.SecretPath {
				continue
			}

			target.Set(src.FieldByName(field.Name))
		}
	}
}
