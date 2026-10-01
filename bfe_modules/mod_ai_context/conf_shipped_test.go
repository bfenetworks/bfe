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

package mod_ai_context

import (
	"testing"
)

// TestShippedSampleConf guards the sample config shipped under conf/: it must
// stay loadable by the real loaders (AGENTS.md: conf changes must keep the
// packaged default setup usable).
func TestShippedSampleConf(t *testing.T) {
	cfg, err := ConfLoad("../../conf/mod_ai_context/mod_ai_context.conf", "../../conf")
	if err != nil {
		t.Fatalf("shipped conf load err: %v", err)
	}
	if _, err := ContextRuleConfLoad(cfg.Basic.ProductRulePath); err != nil {
		t.Fatalf("shipped rule data load err: %v", err)
	}
}
