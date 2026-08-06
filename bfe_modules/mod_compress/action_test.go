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
	"compress/gzip"
	"strings"
	"testing"
)

import (
	"github.com/andybalholm/brotli"
)

func TestActionFileCheck(t *testing.T) {
	cases := []struct {
		name    string
		action  ActionFile
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil cmd",
			action:  ActionFile{Cmd: nil, Quality: intPtr(5), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "no Cmd",
		},
		{
			name:    "invalid cmd",
			action:  ActionFile{Cmd: strPtr("UNKNOWN"), Quality: intPtr(5), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "invalid cmd",
		},
		{
			name:    "gzip quality too low",
			action:  ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(gzip.HuffmanOnly - 1), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "Quality should be",
		},
		{
			name:    "gzip quality too high",
			action:  ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(gzip.BestCompression + 1), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "Quality should be",
		},
		{
			name:    "gzip valid",
			action:  ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(gzip.BestCompression), FlushSize: intPtr(128)},
			wantErr: false,
		},
		{
			name:    "brotli quality too low",
			action:  ActionFile{Cmd: strPtr(ActionBrotli), Quality: intPtr(brotli.BestSpeed - 1), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "Quality should be",
		},
		{
			name:    "brotli quality too high",
			action:  ActionFile{Cmd: strPtr(ActionBrotli), Quality: intPtr(brotli.BestCompression + 1), FlushSize: intPtr(128)},
			wantErr: true,
			errMsg:  "Quality should be",
		},
		{
			name:    "brotli valid",
			action:  ActionFile{Cmd: strPtr(ActionBrotli), Quality: intPtr(brotli.BestCompression), FlushSize: intPtr(128)},
			wantErr: false,
		},
		{
			name:    "flush size too small",
			action:  ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(5), FlushSize: intPtr(63)},
			wantErr: true,
			errMsg:  "FlushSize should be",
		},
		{
			name:    "flush size too large",
			action:  ActionFile{Cmd: strPtr(ActionGzip), Quality: intPtr(5), FlushSize: intPtr(4097)},
			wantErr: true,
			errMsg:  "FlushSize should be",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ActionFileCheck(&tc.action)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestActionConvert(t *testing.T) {
	cmd := ActionBrotli
	quality := 4
	flush := 256

	af := ActionFile{Cmd: &cmd, Quality: &quality, FlushSize: &flush}
	a := actionConvert(af)

	if a.Cmd != cmd || a.Quality != quality || a.FlushSize != flush {
		t.Errorf("unexpected action: %+v", a)
	}
}
