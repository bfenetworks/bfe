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

package mod_compress

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestConfLoadFileNotExist(t *testing.T) {
	_, err := ConfLoad("./testdata/mod_compress/not_exist.conf", "")
	if err == nil {
		t.Fatalf("ConfLoad() should return error for missing file")
	}
}

func TestConfLoadDefaultProductRulePath(t *testing.T) {
	f, err := ioutil.TempFile("", "mod_compress_*.conf")
	if err != nil {
		t.Fatalf("TempFile() error: %v", err)
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

	if cfg.Basic.ProductRulePath != "mod_compress/compress_rule.data" {
		t.Errorf("ProductRulePath should be mod_compress/compress_rule.data, not %s",
			cfg.Basic.ProductRulePath)
	}
}

func TestConfLoadConfRootPath(t *testing.T) {
	f, err := ioutil.TempFile("", "mod_compress_*.conf")
	if err != nil {
		t.Fatalf("TempFile() error: %v", err)
	}
	defer os.Remove(f.Name())

	content := "[basic]\nProductRulePath = compress_rule.data\n[log]\nOpenDebug = true\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString() error: %v", err)
	}
	f.Close()

	confRoot := "myconf"
	cfg, err := ConfLoad(f.Name(), confRoot)
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}

	expected := path.Join(confRoot, "compress_rule.data")
	if cfg.Basic.ProductRulePath != expected {
		t.Errorf("ProductRulePath should be %s, not %s", expected, cfg.Basic.ProductRulePath)
	}
}

func TestConfModCompressCheckDefaultPath(t *testing.T) {
	cfg := &ConfModCompress{}
	if err := cfg.Check(""); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if cfg.Basic.ProductRulePath != "mod_compress/compress_rule.data" {
		t.Errorf("ProductRulePath should be mod_compress/compress_rule.data, not %s",
			cfg.Basic.ProductRulePath)
	}
}
