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

package bfe_wasmplugin

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"testing"
)

const testPluginDir = "../conf/wasm_plugin/headers"

// writeTestPlugin writes wasm bytes with matching conf and md5 files
// into dir, using the layout expected by NewWasmPlugin.
func writeTestPlugin(t *testing.T, dir string, name string, wasmBytes []byte) {
	t.Helper()

	md5Bytes := md5.Sum(wasmBytes)
	files := map[string][]byte{
		name + ".wasm": wasmBytes,
		name + ".conf": []byte("{}"),
		name + ".md5":  []byte(hex.EncodeToString(md5Bytes[:])),
	}
	for fileName, data := range files {
		if err := os.WriteFile(path.Join(dir, fileName), data, 0644); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}
}

func TestNewWasmPlugin(t *testing.T) {
	plugin, err := NewWasmPlugin(WasmPluginConfig{
		PluginName:  "headers",
		Path:        testPluginDir,
		InstanceNum: 1,
	})
	if err != nil {
		t.Fatalf("NewWasmPlugin() error: %v", err)
	}
	defer plugin.Clear()

	if plugin.InstanceNum() != 1 {
		t.Errorf("InstanceNum() = %d, want 1", plugin.InstanceNum())
	}
}

func TestNewWasmPluginMalformedWasm(t *testing.T) {
	validBytes, err := os.ReadFile(path.Join(testPluginDir, "headers.wasm"))
	if err != nil {
		t.Fatalf("read wasm file: %v", err)
	}

	// overwrite part of the type section with invalid value types
	corruptedBytes := append([]byte(nil), validBytes...)
	copy(corruptedBytes[200:], []byte{0xff, 0xff, 0xff, 0xff})

	cases := []struct {
		name      string
		wasmBytes []byte
	}{
		{"corrupted", corruptedBytes},
		{"not_wasm", []byte("not a wasm binary")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestPlugin(t, dir, c.name, c.wasmBytes)

			plugin, err := NewWasmPlugin(WasmPluginConfig{
				PluginName:  c.name,
				Path:        dir,
				InstanceNum: 1,
			})
			if !errors.Is(err, ErrModuleCreate) {
				t.Errorf("NewWasmPlugin() error = %v, want %v", err, ErrModuleCreate)
			}
			if plugin != nil {
				t.Errorf("NewWasmPlugin() should return nil plugin on malformed wasm")
			}
		})
	}
}
