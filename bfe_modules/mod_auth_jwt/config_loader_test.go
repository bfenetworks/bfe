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
	"testing"
)

func TestLoadModuleConfigValid(t *testing.T) {
	config, err := LoadModuleConfig("./testdata/mod_auth_jwt/mod_auth_jwt.conf")
	if err != nil {
		t.Error("Unexpected error happened while loading a valid module Config.\n" + err.Error())
		return
	}
	t.Logf("%+v", config)
}

func TestLoadModuleConfigMissing(t *testing.T) {
	_, err := LoadModuleConfig("./testdata/mod_auth_jwt/module_config_empty.data")
	if err == nil {
		t.Error("Unexpected loaded without error with an invalid module Config")
		return
	}
	if err.code != ConfigItemRequired {
		t.Error("Unexpected error happened while loading an invalid(missing) module Config.\n" + err.Error())
	}
}

func TestLoadModuleConfigInvalid(t *testing.T) {
	_, err := LoadModuleConfig("./testdata/mod_auth_jwt/module_config_invalid.data")
	if err == nil {
		t.Error("Unexpected loaded without error with an invalid module Config")
		return
	}
	if err.code != ConfigItemInvalid {
		t.Error("Unexpected error happened while loading an invalid module Config.\n" + err.Error())
	}
}

func TestLoadProductConfigValid(t *testing.T) {
	modConfig, _ := LoadModuleConfig("./testdata/mod_auth_jwt/mod_auth_jwt.conf")
	config, err := LoadProductConfig(modConfig)
	if err != nil {
		t.Error("UnExpected error occurred when loading a valid product Config\n" + err.Error())
		return
	}
	testConfig := config.Config["test"]
	if testConfig.ValidateClaimNbf || testConfig.ValidateClaimIss != "issuer" {
		t.Error("Product Config item override failed")
	}
	t.Logf("%+v", modConfig)
	t.Logf("%+v", config)
}

func TestLoadProductConfigInvalid(t *testing.T) {
	modConfig := new(ModuleConfig)
	modConfig.Basic.ProductConfigPath = "testdata/mod_auth_jwt/product_config_invalid_type.data"
	_, err := LoadProductConfig(modConfig)
	if err == nil {
		t.Error("Unexpected load successfully with invalid data")
		return
	}
	if err.code != BuildConfigItemFailed {
		t.Error("Other error occurred when loading Config with invalid item type.\n" + err.Error())
	}
}
