// Copyright (c) 2024 The BFE Authors.
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

package mod_wasmplugin

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
	gcfg "gopkg.in/gcfg.v1"
)

type ConfModWasm struct {
	Basic struct {
		WasmPluginPath string // path of Wasm plugins
		DataPath string // path of config data
	}

	Log struct {
		OpenDebug bool
	}
}

// ConfLoad loads config from config file
func ConfLoad(filePath string, confRoot string) (*ConfModWasm, error) {
	var cfg ConfModWasm
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_redirect
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModWasm) Check(confRoot string) error {
	if cfg.Basic.WasmPluginPath == "" {
		log.Logger.Warn("ModWasm.WasmPluginPath not set, use default value")
		cfg.Basic.WasmPluginPath = "mod_wasm"
	}
	cfg.Basic.WasmPluginPath = bfe_util.ConfPathProc(cfg.Basic.WasmPluginPath, confRoot)

	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModWasm.DataPath not set, use default value")
		cfg.Basic.WasmPluginPath = "mod_wasm/wasm.data"
	}
	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	return nil
}
