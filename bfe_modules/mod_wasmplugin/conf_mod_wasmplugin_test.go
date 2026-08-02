// Copyright (c) 2026 The BFE Authors.
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
	"os"
	"testing"
)

func TestConfLoad(t *testing.T) {
	cfg, err := ConfLoad("./testdata/mod_wasm/mod_wasm.conf", "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if cfg.Basic.WasmPluginPath != "mod_wasm/wasm_plugins" {
		t.Errorf("WasmPluginPath should be mod_wasm/wasm_plugins, not %s", cfg.Basic.WasmPluginPath)
	}

	if cfg.Basic.DataPath != "mod_wasm/wasm.data" {
		t.Errorf("DataPath should be mod_wasm/wasm.data, not %s", cfg.Basic.DataPath)
	}

	if !cfg.Log.OpenDebug {
		t.Errorf("OpenDebug should be true")
	}
}

func TestConfLoadDefaultPaths(t *testing.T) {
	f, err := os.CreateTemp("", "mod_wasmplugin_*.conf")
	if err != nil {
		t.Fatalf("CreateTemp() error: %v", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString("[log]\nOpenDebug = true\n"); err != nil {
		t.Fatalf("WriteString() error: %v", err)
	}
	f.Close()

	cfg, err := ConfLoad(f.Name(), "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if cfg.Basic.WasmPluginPath == "" {
		t.Errorf("WasmPluginPath should not be empty")
	}
}

func TestConfLoadFileNotExist(t *testing.T) {
	_, err := ConfLoad("./testdata/mod_wasm/not_exist.conf", "")
	if err == nil {
		t.Errorf("ConfLoad() should return error for missing file")
	}
}
