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
	"testing"
)

import (
	"github.com/baidu/bfe/bfe_util"
)

func TestNewModuleConfig_1(t *testing.T) {
	config, err := NewModuleConfig(bfe_util.ConfPathProc("mod_auth_jwt.conf", confRoot))
	if err != nil {
		t.Error(err)
	}

	t.Log(config)
}

func TestNewModuleConfig_2(t *testing.T) {
	_, err := NewModuleConfig(bfe_util.ConfPathProc("module_config_empty.data", confRoot))
	if err == nil {
		t.Errorf("loading module config without required parameter should be failed")
	}

	t.Log(err)
}

func TestNewModuleConfig_3(t *testing.T) {
	_, err := NewModuleConfig(bfe_util.ConfPathProc("module_config_invalid.data", confRoot))
	if err == nil {
		t.Errorf("loading module config with invalid parameter value should be failed")
	}

	t.Log(err)
}
