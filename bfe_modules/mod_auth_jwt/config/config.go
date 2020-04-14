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
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var (
	invalid = reflect.Value{}
)

type Config struct {
	lock          sync.RWMutex
	moduleConfig  reflect.Value
	ModuleConfig  *ModuleConfig
	ProductConfig *ProductConfig
}

func New(path string) (config *Config, err error) {
	config = new(Config)
	config.ModuleConfig, err = NewModuleConfig(path)
	if err != nil {
		return nil, err
	}

	config.ProductConfig, err = NewProductConfig(config.ModuleConfig.Basic.ProductConfigPath)
	if err != nil {
		return nil, err
	}

	err = config.init()
	if err != nil {
		return nil, err
	}

	return config, nil
}

// initialize for the config
func (config *Config) init() (err error) {
	config.moduleConfig = reflect.Indirect(reflect.ValueOf(config.ModuleConfig))
	config.ProductConfig.Merge(config.ModuleConfig)

	err = config.Check()
	if err != nil {
		return err
	}

	return nil
}

// Check checks whether the config item were valid
func (config *Config) Check() (err error) {
	for product, rule := range config.ProductConfig.Config {
		if len(rule.SecretPath) == 0 {
			return fmt.Errorf("required parameter `SecretPath` for product rule missed (product: %s)", product)
		}
	}

	return nil
}

// Update updates config by the given new path
func (config *Config) Update(path string) (err error) {
	config.lock.Lock()
	defer config.lock.Unlock()

	// ensures atomicity
	originModuleConfig := *config.ModuleConfig
	err = config.ModuleConfig.Update(path)
	if err != nil {
		return err
	}

	err = config.ProductConfig.Update(config.ModuleConfig.Basic.ProductConfigPath)
	if err != nil {
		*config.ModuleConfig = originModuleConfig
		return err
	}

	err = config.init()
	if err != nil {
		*config.ModuleConfig = originModuleConfig
		return err
	}

	return nil
}

// Reload reloads config
func (config *Config) Reload() (err error) {
	return config.Update(config.ModuleConfig.path)
}

// Search searches auth rule for a product
func (config *Config) Search(name string) (rule *AuthRule, ok bool) {
	config.lock.RLock()
	defer config.lock.RUnlock()

	rule, ok = config.ProductConfig.Config[name]

	return rule, ok
}

// Get gets member from ModuleConfig with RLock
func (config *Config) Get(name string) (v reflect.Value, ok bool) {
	config.lock.RLock()
	defer config.lock.RUnlock()

	v = config.moduleConfig
	getters := strings.Split(name, ".")

	// support for getter as a.b.c
	for _, getter := range getters {
		v = v.FieldByName(getter)
		if !v.IsValid() {
			return invalid, false
		}
	}

	return v, true
}
