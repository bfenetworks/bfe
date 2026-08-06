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

package mod_errors

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestConfLoad(t *testing.T) {
	config, err := ConfLoad("./testdata/mod_errors/mod_errors.conf", "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if config.Basic.DataPath != "mod_errors/errors_rule.data" {
		t.Errorf("DataPath should be mod_errors/errors_rule.data, not %s", config.Basic.DataPath)
	}
}

func TestConfLoadDefaultDataPath(t *testing.T) {
	f, err := ioutil.TempFile("", "mod_errors_*.conf")
	if err != nil {
		t.Fatalf("TempFile() error: %v", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString("[log]\nOpenDebug = true\n"); err != nil {
		t.Fatalf("WriteString() error: %v", err)
	}
	f.Close()

	config, err := ConfLoad(f.Name(), "")
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	if config.Basic.DataPath != "mod_errors/mod_errors.data" {
		t.Errorf("DataPath should be mod_errors/mod_errors.data, not %s", config.Basic.DataPath)
	}
}

func TestConfLoadFileNotExist(t *testing.T) {
	_, err := ConfLoad("./testdata/mod_errors/not_exist.conf", "")
	if err == nil {
		t.Errorf("ConfLoad() should return error for missing file")
	}
}
