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
	"path/filepath"
)

import (
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/baidu/bfe/bfe_util"
)

// module config
type ModuleConfig struct {
	path string // the path of config file
	root string // the directory of `path`

	Basic struct {
		AuthConfig
		ProductConfigPath string
	}

	Log struct {
		OpenDebug bool
	}
}

func NewModuleConfig(path string) (config *ModuleConfig, err error) {
	config = new(ModuleConfig)

	err = config.Update(path)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// Update updates config from the given new path
func (config *ModuleConfig) Update(path string) (err error) {
	root, _ := filepath.Split(path)
	// ensures atomicity
	ptr, config := config, &ModuleConfig{path: path, root: root}
	err = gcfg.ReadFileInto(config, path)
	if err != nil {
		return err
	}

	// perform check action
	err = config.Check()
	if err != nil {
		return err
	}

	// join the path parameters
	config.Basic.ProductConfigPath = bfe_util.ConfPathProc(config.Basic.ProductConfigPath, config.root)
	if len(config.Basic.SecretPath) > 0 {
		config.Basic.SecretPath = bfe_util.ConfPathProc(config.Basic.SecretPath, config.root)
		// build secret as JSON Web Key
		err = config.Basic.AuthConfig.BuildSecret()
		if err != nil {
			return err
		}
	}

	*ptr = *config // ensures atomicity

	return nil
}

// Reload reloads config from config.path
func (config *ModuleConfig) Reload() (err error) {
	return config.Update(config.path)
}

// Check checks if the config was valid
func (config *ModuleConfig) Check() (err error) {
	secretPath := config.Basic.SecretPath
	productConfigPath := config.Basic.ProductConfigPath

	if len(productConfigPath) == 0 {
		return fmt.Errorf("required config parameter `ProductConfigPath` missed")
	}

	if len(secretPath) > 0 {
		err = AssertsFile(bfe_util.ConfPathProc(config.Basic.SecretPath, config.root))
		if err != nil {
			return err
		}
	}

	return AssertsFile(bfe_util.ConfPathProc(config.Basic.ProductConfigPath, config.root))
}
