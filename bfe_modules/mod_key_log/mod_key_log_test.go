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

package mod_key_log

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_module"
)

func TestNewModuleKeyLog(t *testing.T) {
	m := NewModuleKeyLog()
	if m == nil {
		t.Fatal("NewModuleKeyLog() returns nil")
	}
	if m.Name() != "mod_key_log" {
		t.Errorf("module name error, got: %s", m.Name())
	}
}

func TestModuleKeyLogInit(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_key_log_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	modConfDir := filepath.Join(dir, "mod_key_log")
	if err := os.MkdirAll(modConfDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	dataPath, _ := filepath.Abs("./testdata/mod_key_log/key_log.json")
	confPath := filepath.Join(modConfDir, "mod_key_log.conf")
	content := `[Log]
LogFile = key.log

[basic]
DataPath = ` + filepath.ToSlash(dataPath) + `
`
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	m := NewModuleKeyLog()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	err = m.Init(cb, wh, dir)
	if err != nil {
		t.Errorf("Init() error: %v", err)
	}

	if m.TlsKeyLogWriter() == nil {
		t.Error("TlsKeyLogWriter() should not be nil after Init()")
	}
}

func TestTlsKeyLogWriterBeforeInit(t *testing.T) {
	m := NewModuleKeyLog()
	if m.TlsKeyLogWriter() != nil {
		t.Error("TlsKeyLogWriter() should be nil before Init()")
	}
}

func TestLoadConfData(t *testing.T) {
	m := NewModuleKeyLog()
	m.dataConfigPath = "./testdata/mod_key_log/key_log.json"

	version, err := m.loadConfData(nil)
	if err != nil {
		t.Errorf("loadConfData() error: %v", err)
	}
	if version == "" {
		t.Error("version should not be empty")
	}
}

func TestKeyLogTable(t *testing.T) {
	table := NewKeyLogTable()
	conf, err := keyLogConfLoad("./testdata/mod_key_log/key_log.json")
	if err != nil {
		t.Fatalf("keyLogConfLoad() error: %v", err)
	}
	table.Update(conf)

	rules, ok := table.Search("example_product")
	if !ok {
		t.Error("should find rules for example_product")
	}
	if rules == nil || len(*rules) == 0 {
		t.Error("rules should not be empty")
	}

	_, ok = table.Search("not_exist")
	if ok {
		t.Error("should not find rules for not_exist")
	}
}
