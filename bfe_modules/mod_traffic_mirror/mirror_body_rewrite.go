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
	"bytes"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// rewriteMirrorBody applies GJSON-path body field rewrites (FR-6) on the
// request body copy. Phase 1 only supports the top-level "model" field,
// validated at rule load time. The input body is never modified: a new
// buffer is returned.
func rewriteMirrorBody(body []byte, rewrites []*MirrorBodyRewriteConf) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("request body is not valid json")
	}

	out := body
	for _, rw := range rewrites {
		if rw == nil {
			continue
		}
		if !gjson.GetBytes(out, rw.Path).Exists() {
			return nil, fmt.Errorf("body field %s not found", rw.Path)
		}
		updated, err := sjson.SetBytes(out, rw.Path, rw.Value)
		if err != nil {
			return nil, fmt.Errorf("rewrite %s failed: %v", rw.Path, err)
		}
		out = updated
	}

	if bytes.Equal(out, body) {
		return nil, fmt.Errorf("body unchanged after rewrite")
	}
	return out, nil
}
