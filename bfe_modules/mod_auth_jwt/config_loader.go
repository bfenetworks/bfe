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

package mod_auth_jwt

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwt"
	"os"
	"path/filepath"
	"reflect"
)

import (
	"github.com/baidu/bfe/bfe_basic/condition"
	util "github.com/baidu/bfe/bfe_util"
	"gopkg.in/gcfg.v1"
)

type ModuleConfig struct {
	Basic struct {
		jwt.Config
		ProductConfigPath string
	}

	Log struct {
		OpenDebug bool
	}
}

// Config items
type ProductConfigItem struct {
	jwt.Config
	Cond condition.Condition
}

type productConfig map[string]ProductConfigItem

type ProductConfig struct {
	Version string
	Config  productConfig
}

func LoadModuleConfig(path string) (config *ModuleConfig, err *TypedError) {
	config = new(ModuleConfig)
	rawErr := gcfg.ReadFileInto(config, path)
	if rawErr != nil {
		return nil, NewTypedError(ConfigLoadFailed, rawErr)
	}

	// check for required Config item
	secretPath, ProductConfigPath := config.Basic.SecretPath, config.Basic.ProductConfigPath
	if len(secretPath) == 0 || len(ProductConfigPath) == 0 {
		err = NewTypedError(ConfigItemRequired,
			errors.New("config item SecretPath and ProductConfigPath cannot be left blank"))

		return nil, err
	}

	// ensure path parameters are absolute path
	root, _ := filepath.Split(path)
	config.Basic.SecretPath = util.ConfPathProc(secretPath, root)
	config.Basic.ProductConfigPath = util.ConfPathProc(ProductConfigPath, root)

	// validation for Config item
	if err = validateModuleConfig(config); err != nil {
		return nil, err
	}

	// read secret Config
	config.Basic.Secret, rawErr = readSecret(config.Basic.SecretPath)
	if rawErr != nil {
		return nil, NewTypedError(BadSecretConfig, rawErr)
	}

	return config, nil
}

// validation for Config item
func validateModuleConfig(config *ModuleConfig) (err *TypedError) {
	if isFile, err := isFile(config.Basic.SecretPath); !isFile || err != nil {
		if err != nil {
			return NewTypedError(ConfigItemInvalid, err)
		}
		return NewTypedError(ConfigItemInvalid, errors.New("the SecretPath should be a file, not directory"))
	}
	if isFile, err := isFile(config.Basic.ProductConfigPath); !isFile || err != nil {
		if err != nil {
			return NewTypedError(ConfigItemInvalid, err)
		}
		return NewTypedError(ConfigItemInvalid,
			errors.New("the ProductConfigPath should be a file, not directory"))
	}

	return nil
}

func isFile(path string) (result bool, err error) {
	stat, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return !stat.IsDir(), nil
}

func readSecret(path string) (mJWK *jwk.JWK, err error) {
	if isFile, err := isFile(path); !isFile || err != nil {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("secret path should be a file")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	keyMap := make(map[string]interface{})
	err = json.NewDecoder(file).Decode(&keyMap)
	if err != nil {
		return nil, err
	}

	return jwk.NewJWK(keyMap)
}

func LoadProductConfig(modConfig *ModuleConfig) (config *ProductConfig, err *TypedError) {
	file, rawErr := os.Open(modConfig.Basic.ProductConfigPath)
	if rawErr != nil {
		return nil, NewTypedError(ConfigLoadFailed, rawErr)
	}
	defer file.Close()

	rawData := make(map[string]interface{})
	decoder := json.NewDecoder(file)
	// read Config from file
	rawErr = decoder.Decode(&rawData)
	if rawErr != nil {
		return nil, NewTypedError(JsonDecoderError, rawErr)
	}

	return buildProductConfig(rawData, modConfig)
}

// build product Config from type map[string]T to type ProductConfig
// and merge overridable item from module Config
func buildProductConfig(data map[string]interface{}, modConfig *ModuleConfig) (config *ProductConfig, err *TypedError) {
	config = new(ProductConfig)

	// apply type check
	version, ok := data["Version"].(string)
	if !ok {
		return nil, NewTypedError(ConfigLoadFailed,
			errors.New("invalid type for product Config item `Version`"))
	}
	config.Version = version

	//
	confMap, ok := data["Config"].(map[string]interface{})
	if !ok {
		return nil, NewTypedError(ConfigLoadFailed,
			errors.New("invalid type for product Config item `Config`"))
	}
	config.Config = make(productConfig)

	// build Config item
	for name, conf := range confMap {
		converted, ok := conf.(map[string]interface{})
		if !ok {
			return nil, NewTypedError(ConfigLoadFailed,
				fmt.Errorf("invalid type for product Config(%s) item `Config`", name))
		}

		// build Config for each product
		item, err := buildProductConfigItem(converted, modConfig)
		if err != nil {
			return nil, NewTypedError(BuildConfigItemFailed,
				fmt.Errorf("building for product: %s.\n%s", name, err.Error()))
		}
		config.Config[name] = *item
	}

	return config, nil
}

// build single product Config item
func buildProductConfigItem(config map[string]interface{}, modConfig *ModuleConfig) (item *ProductConfigItem, err *TypedError) {
	item = new(ProductConfigItem)
	cond, ok := config["Cond"]
	if !ok {
		return nil, NewTypedError(ConfigItemRequired,
			errors.New("missing required Config item `Cond`"))
	}

	condStr, ok := cond.(string)
	if !ok {
		return nil, NewTypedError(InvalidConfigItem,
			errors.New("invalid type of item `Cond`"))
	}

	// building condition
	condBuilt, rawErr := condition.Build(condStr)
	if rawErr != nil {
		return nil, NewTypedError(BuildCondFailed, rawErr)
	}
	item.Cond = condBuilt

	// get anonymous field JWTConfig from module Config
	jwtConfig := reflect.ValueOf(modConfig.Basic).FieldByName("Config")

	// cast Config item as reflect.Value
	refItem := reflect.Indirect(reflect.ValueOf(item))

	// merge default Config
	err = merge(refItem, jwtConfig, config)
	if err != nil {
		return nil, err
	}
	if item.SecretPath != modConfig.Basic.SecretPath {
		root, _ := filepath.Split(modConfig.Basic.ProductConfigPath)

		// ensure secret path is absolute path
		item.SecretPath = util.ConfPathProc(item.SecretPath, root)

		// read secret
		item.Secret, rawErr = readSecret(item.SecretPath)
		if rawErr != nil {
			return nil, NewTypedError(BadSecretConfig, rawErr)
		}
	}

	return item, nil
}

// merge Config with default Config
func merge(conf reflect.Value, defConf reflect.Value, keySet map[string]interface{}) (err *TypedError) {
	// key set is used to distinct whether the falsely value from Config -
	// is truly false or does not exists (default zero-value)

	// get item type of the default Config
	typeJwtConfig := defConf.Type()
	numField := defConf.NumField()
	for i := 0; i < numField; i++ {
		// get name and value from module JWTConfig
		name := typeJwtConfig.Field(i).Name
		value := defConf.FieldByName(name)

		// get value field for refItem by name
		refValue := conf.FieldByName(name)
		if v, ok := keySet[name]; ok {
			// cast v as type reflect.Value
			convertV := reflect.ValueOf(v)

			if convertV.Type() != refValue.Type() {
				// type check failed
				return NewTypedError(InvalidConfigItem,
					fmt.Errorf("invalid type of item `%s`", name))
			}
			// override Config item
			refValue.Set(convertV)
		} else if refValue != value {
			// merge with module Config
			// type check ignored here (always correct type given)
			refValue.Set(value)
		}
	}

	return nil
}
