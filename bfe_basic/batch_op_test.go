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

package bfe_basic

import "testing"

func TestClassifyBatchOp(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{"POST", "/v1/files", BatchOpUpload},
		{"POST", "/files", BatchOpUpload},
		{"GET", "/v1/files/file-abc", BatchOpGet},
		{"GET", "/v1/files/file-abc/content", BatchOpDownload},
		{"GET", "/files/file-abc/content", BatchOpDownload},
		{"DELETE", "/v1/files/file-abc", ""}, // not intercepted
		{"POST", "/v1/batches", BatchOpCreate},
		{"GET", "/v1/batches", BatchOpList},
		{"GET", "/v1/batches/batch-abc", BatchOpGet},
		{"POST", "/v1/batches/batch-abc/cancel", BatchOpCancel},
		{"POST", "/batches/batch-abc/cancel", BatchOpCancel},
		// provider-native base_url prefixes reduce like DetectModeFromPath
		{"POST", "/compatible-mode/v1/files", BatchOpUpload},
		{"GET", "/compatible-mode/v1/batches/batch-abc", BatchOpGet},
		{"POST", "/compatible-mode/v1/batches/batch-abc/cancel", BatchOpCancel},
		// non-batch paths
		{"POST", "/v1/chat/completions", ""},
		{"GET", "/v1/models", ""},
	}
	for _, c := range cases {
		if got := ClassifyBatchOp(c.method, c.path); got != c.want {
			t.Errorf("ClassifyBatchOp(%q, %q) = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

func TestBatchPathIds(t *testing.T) {
	if got := BatchPathFileId("/v1/files/file-abc/content"); got != "file-abc" {
		t.Errorf("BatchPathFileId = %q, want file-abc", got)
	}
	if got := BatchPathBatchId("/v1/batches/batch-abc/cancel"); got != "batch-abc" {
		t.Errorf("BatchPathBatchId = %q, want batch-abc", got)
	}
	if got := BatchPathFileId("/v1/batches/batch-abc"); got != "" {
		t.Errorf("BatchPathFileId = %q, want empty", got)
	}
}
