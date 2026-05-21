// Copyright (c) 2025 The BFE Authors.
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

package mod_ai_token_auth

import "testing"

func TestConfModAITokenAuthCheckDefaultProductRulePath(t *testing.T) {
	cfg := &ConfModAITokenAuth{}
	cfg.Redis.Bns = "BFE.poc-redis-wx"
	cfg.Redis.ConnectTimeout = 20
	cfg.Redis.ReadTimeout = 20
	cfg.Redis.WriteTimeout = 20

	if err := cfg.Check("conf"); err != nil {
		t.Fatalf("ConfModAITokenAuth.Check() error = %v", err)
	}

	want := "conf/mod_ai_token_auth/token_rule.data"
	if cfg.Basic.ProductRulePath != want {
		t.Fatalf("ProductRulePath = %s, want %s", cfg.Basic.ProductRulePath, want)
	}
}
