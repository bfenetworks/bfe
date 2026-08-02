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
	"testing"
)

func prepareMimeFile(t *testing.T, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_mime_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "mime_type.data")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return filepath.ToSlash(path)
}

func TestMimeTypeConfLoadFileNotExist(t *testing.T) {
	if _, err := MimeTypeConfLoad("./testdata/mod_static/mime_type.data.not_exist"); err == nil {
		t.Errorf("MimeTypeConfLoad() should return error when file does not exist")
	}
}

func TestMimeTypeConfLoadInvalidJSON(t *testing.T) {
	path := prepareMimeFile(t, "not a json")
	if _, err := MimeTypeConfLoad(path); err == nil {
		t.Errorf("MimeTypeConfLoad() should return error for invalid JSON")
	}
}

func TestMimeTypeConfLoadMissingVersion(t *testing.T) {
	path := prepareMimeFile(t, `{"Config":{".html":"text/html"}}`)
	if _, err := MimeTypeConfLoad(path); err == nil {
		t.Errorf("MimeTypeConfLoad() should return error when Version is missing")
	}
}

func TestMimeTypeConfConvert(t *testing.T) {
	conf := &MimeTypeConf{
		Version: "unittest",
		Config: MimeType{
			".HTML": "text/html",
			".DOC":  "application/msword",
		},
	}
	MimeTypeConfConvert(conf)

	if _, ok := conf.Config[".html"]; !ok {
		t.Errorf("MimeTypeConfConvert() should lower-case extension .HTML")
	}
	if _, ok := conf.Config[".doc"]; !ok {
		t.Errorf("MimeTypeConfConvert() should lower-case extension .DOC")
	}
	if _, ok := conf.Config[".HTML"]; ok {
		t.Errorf("MimeTypeConfConvert() should remove original upper-case key")
	}
}

func TestMimeTypeTableUpdateAndSearch(t *testing.T) {
	table := NewMimeTypeTable()
	if table == nil {
		t.Fatal("NewMimeTypeTable() returns nil")
	}

	table.Update(MimeTypeConf{
		Version: "v1",
		Config: MimeType{
			".html": "text/html",
		},
	})

	value, ok := table.Search(".html")
	if !ok || value != "text/html" {
		t.Errorf("Search(.html) should return text/html, got %s, ok=%v", value, ok)
	}

	_, ok = table.Search(".notexist")
	if ok {
		t.Errorf("Search(.notexist) should return false")
	}
}
