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

package bfe_server

import (
	"errors"
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
)

// TestShouldMarkAIStreamCompleted pins the issue #1415 completion-mark
// matrix: only a fully delivered (nil copy error) 200 SSE response of a
// non-file/batch AI request marks the response completed. Client aborts,
// mid-stream upstream drops, non-SSE shapes and non-2xx responses must never
// reach the mark, and a nil aiMeta (non-AI request) is rejected outright.
func TestShouldMarkAIStreamCompleted(t *testing.T) {
	sseRes := func() *bfe_http.Response {
		return &bfe_http.Response{StatusCode: bfe_http.StatusOK, IsSse: true}
	}
	aiMeta := func() *bfe_basic.AiBasicInfo {
		return &bfe_basic.AiBasicInfo{AuthStyle: bfe_basic.AuthStyleOpenAI, Mode: bfe_basic.ModeChat}
	}

	tests := []struct {
		name    string
		res     *bfe_http.Response
		aiMeta  *bfe_basic.AiBasicInfo
		copyErr error
		want    bool
	}{
		{
			name: "clean EOF SSE 200 chat marks completed",
			res:  sseRes(), aiMeta: aiMeta(), copyErr: nil, want: true,
		},
		{
			name: "client abort (ErrClientWrite) never marks",
			res:  sseRes(), aiMeta: aiMeta(), copyErr: bfe_basic.ErrClientWrite, want: false,
		},
		{
			name: "mid-stream upstream drop (generic read error) never marks",
			res:  sseRes(), aiMeta: aiMeta(), copyErr: errors.New("unexpected EOF"), want: false,
		},
		{
			name:   "non-200 SSE response never marks",
			res:    &bfe_http.Response{StatusCode: bfe_http.StatusInternalServerError, IsSse: true},
			aiMeta: aiMeta(), copyErr: nil, want: false,
		},
		{
			name:   "non-SSE JSON response not covered (non-stream path owns it)",
			res:    &bfe_http.Response{StatusCode: bfe_http.StatusOK, IsSse: false},
			aiMeta: aiMeta(), copyErr: nil, want: false,
		},
		{
			name: "file mode SSE excluded from estimation billing",
			res:  sseRes(), aiMeta: &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeFile}, copyErr: nil, want: false,
		},
		{
			name: "batch mode SSE excluded from estimation billing",
			res:  sseRes(), aiMeta: &bfe_basic.AiBasicInfo{Mode: bfe_basic.ModeBatch}, copyErr: nil, want: false,
		},
		{
			name: "nil aiMeta (non-AI request) rejected",
			res:  sseRes(), aiMeta: nil, copyErr: nil, want: false,
		},
		{
			name: "nil response rejected",
			res:  nil, aiMeta: aiMeta(), copyErr: nil, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldMarkAIStreamCompleted(tt.res, tt.aiMeta, tt.copyErr); got != tt.want {
				t.Errorf("shouldMarkAIStreamCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}
