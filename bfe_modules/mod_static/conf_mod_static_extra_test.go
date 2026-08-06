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

package mod_static

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepareConfRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_conf_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.ToSlash(dir)
}

func TestConfLoadFileNotExist(t *testing.T) {
	root := prepareConfRoot(t)
	confPath := filepath.Join(root, "mod_static", "mod_static.conf")

	if _, err := ConfLoad(confPath, root); err == nil {
		t.Errorf("ConfLoad() should return error when config file does not exist")
	}
}

func TestConfLoadInvalidConf(t *testing.T) {
	root := prepareConfRoot(t)
	modDir := filepath.Join(root, "mod_static")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	confPath := filepath.Join(modDir, "mod_static.conf")
	if err := os.WriteFile(confPath, []byte("this is not a valid gcfg file"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := ConfLoad(confPath, root); err == nil {
		t.Errorf("ConfLoad() should return error for invalid config content")
	}
}

func TestConfLoadDefaultPaths(t *testing.T) {
	root := prepareConfRoot(t)
	modDir := filepath.Join(root, "mod_static")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	confPath := filepath.Join(modDir, "mod_static.conf")
	confContent := `[log]
OpenDebug = true
`
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := ConfLoad(confPath, root)
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad() returns nil config")
	}

	expectedDataPath := filepath.ToSlash(filepath.Join(root, "mod_static", "static_rule.data"))
	if cfg.Basic.DataPath != expectedDataPath {
		t.Errorf("DataPath should be %s, got %s", expectedDataPath, cfg.Basic.DataPath)
	}

	expectedMimeTypePath := filepath.ToSlash(filepath.Join(root, "mod_static", "mime_type.data"))
	if cfg.Basic.MimeTypePath != expectedMimeTypePath {
		t.Errorf("MimeTypePath should be %s, got %s", expectedMimeTypePath, cfg.Basic.MimeTypePath)
	}

	if cfg.Basic.EnableCompress {
		t.Errorf("EnableCompress should be false by default")
	}
}

func TestConfLoadRelativePath(t *testing.T) {
	root := prepareConfRoot(t)
	modDir := filepath.Join(root, "mod_static")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	confPath := filepath.Join(modDir, "mod_static.conf")
	confContent := `[basic]
DataPath = ./mod_static/static_rule.data
MimeTypePath = ./mod_static/mime_type.data
EnableCompress = true
`
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := ConfLoad(confPath, root)
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if !strings.Contains(cfg.Basic.DataPath, "static_rule.data") {
		t.Errorf("DataPath should contain static_rule.data, got %s", cfg.Basic.DataPath)
	}
	if !cfg.Basic.EnableCompress {
		t.Errorf("EnableCompress should be true")
	}
}
