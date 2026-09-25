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

package mod_traffic_mirror

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestRewriteMirrorBodyModel(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rewrites := []*MirrorBodyRewriteConf{
		{Path: "model", Value: "deepseek-v3"},
	}

	out, err := rewriteMirrorBody(body, rewrites)
	if err != nil {
		t.Fatalf("rewriteMirrorBody failed: %v", err)
	}

	if v := gjson.GetBytes(out, "model").String(); v != "deepseek-v3" {
		t.Errorf("model = %s, want deepseek-v3", v)
	}
	// original body must not be modified
	if v := gjson.GetBytes(body, "model").String(); v != "gpt-4o" {
		t.Errorf("original body modified: model = %s", v)
	}
	// other fields stay intact
	if v := gjson.GetBytes(out, "stream").Bool(); !v {
		t.Error("stream flag should stay true")
	}
}

func TestRewriteMirrorBodyFieldMissing(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	rewrites := []*MirrorBodyRewriteConf{
		{Path: "model", Value: "deepseek-v3"},
	}

	if _, err := rewriteMirrorBody(body, rewrites); err == nil {
		t.Error("rewrite should fail when field not found")
	}
}

func TestRewriteMirrorBodyInvalidJson(t *testing.T) {
	body := []byte(`not-a-json`)
	rewrites := []*MirrorBodyRewriteConf{
		{Path: "model", Value: "deepseek-v3"},
	}

	if _, err := rewriteMirrorBody(body, rewrites); err == nil {
		t.Error("rewrite should fail for invalid json")
	}
}
