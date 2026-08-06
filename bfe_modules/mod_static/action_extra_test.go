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

func TestActionFileCheckErrorCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("./testdata", "mod_static_action_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validDir := filepath.ToSlash(tempDir)
	if err := os.WriteFile(filepath.Join(tempDir, "index.html"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	browse := ActionBrowse
	index := "INDEX"

	tests := []struct {
		name    string
		conf    *ActionFile
		wantErr bool
	}{
		{
			name: "nil Cmd",
			conf: &ActionFile{
				Cmd:    nil,
				Params: []string{validDir, ""},
			},
			wantErr: true,
		},
		{
			name: "invalid Cmd",
			conf: &ActionFile{
				Cmd:    &index,
				Params: []string{validDir, "index.html"},
			},
			wantErr: true,
		},
		{
			name: "BROWSE with wrong number of Params",
			conf: &ActionFile{
				Cmd:    &browse,
				Params: []string{validDir},
			},
			wantErr: true,
		},
		{
			name: "BROWSE with nonexistent directory",
			conf: &ActionFile{
				Cmd:    &browse,
				Params: []string{validDir + "_not_exist", "index.html"},
			},
			wantErr: true,
		},
		{
			name: "BROWSE with nonexistent default file",
			conf: &ActionFile{
				Cmd:    &browse,
				Params: []string{validDir, "missing.html"},
			},
			wantErr: true,
		},
		{
			name: "BROWSE with empty default file",
			conf: &ActionFile{
				Cmd:    &browse,
				Params: []string{validDir, ""},
			},
			wantErr: false,
		},
		{
			name: "BROWSE with valid default file",
			conf: &ActionFile{
				Cmd:    &browse,
				Params: []string{validDir, "index.html"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ActionFileCheck(tt.conf)
			if tt.wantErr && err == nil {
				t.Errorf("ActionFileCheck() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ActionFileCheck() should not return error, got %v", err)
			}
		})
	}
}

func TestActionConvert(t *testing.T) {
	cmd := ActionBrowse
	params := []string{"./testdata/mod_static", "index.html"}
	actionFile := ActionFile{
		Cmd:    &cmd,
		Params: params,
	}

	action := actionConvert(actionFile)
	if action.Cmd != ActionBrowse {
		t.Errorf("Cmd should be %s, got %s", ActionBrowse, action.Cmd)
	}
	if len(action.Params) != len(params) || action.Params[0] != params[0] {
		t.Errorf("Params mismatch, got %v", action.Params)
	}
}

func prepareActionRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("./testdata", "mod_static_action_")
	if err != nil {
		t.Fatalf("MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.ToSlash(dir)
}

func TestActionFileCheckDefaultFilePathErrorMessage(t *testing.T) {
	root := prepareActionRoot(t)
	browse := ActionBrowse

	err := ActionFileCheck(&ActionFile{
		Cmd:    &browse,
		Params: []string{root, "missing.html"},
	})
	if err == nil {
		t.Fatalf("ActionFileCheck() should return error")
	}
	if !strings.Contains(err.Error(), "Default File") {
		t.Errorf("error message should mention Default File, got %v", err)
	}
}
