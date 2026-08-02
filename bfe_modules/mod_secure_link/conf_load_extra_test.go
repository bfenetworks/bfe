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

package mod_secure_link

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func writeTempConfFile(t *testing.T, dataPath string, openDebug bool) string {
	t.Helper()

	// Use a temporary directory with forward slashes so that relative config
	// paths remain portable across platforms.
	dir := strings.ReplaceAll(t.TempDir(), "\\", "/")
	modDir := dir + "/mod_secure_link"
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	var content string
	if dataPath != "" {
		content = "[basic]\nDataPath = " + dataPath + "\n\n[log]\nOpenDebug = " + strconv.FormatBool(openDebug) + "\n"
	} else {
		content = "[log]\nOpenDebug = " + strconv.FormatBool(openDebug) + "\n"
	}

	confPath := modDir + "/mod_secure_link.conf"
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return confPath
}

func TestConfLoadDefaultDataPath(t *testing.T) {
	confPath := writeTempConfFile(t, "", false)
	cfg, err := ConfLoad(confPath, "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	want := "mod_secure_link/secure_link.data"
	if cfg.Basic.DataPath != want {
		t.Errorf("DataPath want %q, got %q", want, cfg.Basic.DataPath)
	}
}

func TestConfLoadOpenDebug(t *testing.T) {
	confPath := writeTempConfFile(t, "mod_secure_link/secure_link.data", true)
	cfg, err := ConfLoad(confPath, "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}
	if !cfg.Log.OpenDebug {
		t.Errorf("Log.OpenDebug want true, got false")
	}
}

func TestConfLoadFileNotExist(t *testing.T) {
	_, err := ConfLoad("testdata/mod_secure_link/not_exist.conf", "")
	if err == nil {
		t.Errorf("ConfLoad() want error for missing file, got nil")
	}
}
