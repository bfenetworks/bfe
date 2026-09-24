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

package gemini

import (
	"testing"
)

func TestExtractModelFromPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"generateContent", "/v1beta/models/gemini-2.5-flash:generateContent", "gemini-2.5-flash"},
		{"streamGenerateContent", "/v1beta/models/gemini-2.5-flash:streamGenerateContent", "gemini-2.5-flash"},
		{"countTokens", "/v1beta/models/gemini-2.5-flash:countTokens", "gemini-2.5-flash"},
		{"bare model path", "/v1beta/models/gemini-2.5-flash", "gemini-2.5-flash"},
		{"trailing slash", "/v1beta/models/gemini-2.5-flash/", "gemini-2.5-flash"},
		{"empty after prefix", "/v1beta/models/", ""},
		{"action only after prefix", "/v1beta/models/:generateContent", ""},
		{"prefix without trailing slash", "/v1beta/models", ""},
		{"empty path", "", ""},
		{"openai chat path", "/v1/chat/completions", ""},
		{"anthropic messages path", "/v1/messages", ""},
		{"similar but shorter prefix", "/v1beta/model/gemini-2.5-flash:generateContent", ""},
	}

	for _, c := range cases {
		if got := ExtractModelFromPath(c.path); got != c.want {
			t.Errorf("%s: ExtractModelFromPath(%q) = %q, want %q", c.name, c.path, got, c.want)
		}
	}
}
