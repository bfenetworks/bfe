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

package mod_ai_rate_limit

import (
	"testing"
)

func TestMatchModelEmptyPolicyModels(t *testing.T) {
	if !matchModel(nil, "gpt-4") {
		t.Error("empty policyModels should match any model")
	}
	if !matchModel([]string{}, "gpt-4") {
		t.Error("empty policyModels should match any model")
	}
}

func TestMatchModelEmptyClientModel(t *testing.T) {
	if matchModel([]string{"gpt-4"}, "") {
		t.Error("empty client model should not match")
	}
}

func TestMatchModelWildcard(t *testing.T) {
	if !matchModel([]string{"*"}, "gpt-4") {
		t.Error("wildcard should match any model")
	}
	if !matchModel([]string{"gpt-4", "*"}, "gpt-4o") {
		t.Error("wildcard should match any model when present in list")
	}
}

func TestMatchModelExactMatch(t *testing.T) {
	if !matchModel([]string{"gpt-4"}, "gpt-4") {
		t.Error("exact model name should match")
	}
	if !matchModel([]string{"gpt-4", "gpt-4o"}, "gpt-4o") {
		t.Error("model should match one in the list")
	}
}

func TestMatchModelNoMatch(t *testing.T) {
	if matchModel([]string{"gpt-4"}, "gpt-4o") {
		t.Error("different model should not match")
	}
	if matchModel([]string{"gpt-4", "gpt-4o"}, "gpt-3.5") {
		t.Error("model not in list should not match")
	}
}

func TestMatchModelWildcardFirst(t *testing.T) {
	if !matchModel([]string{"*", "gpt-4"}, "gpt-4o") {
		t.Error("wildcard at first position should match")
	}
}

func TestMatchModelSingleModelList(t *testing.T) {
	if !matchModel([]string{"claude-3"}, "claude-3") {
		t.Error("single model list should match same model")
	}
	if matchModel([]string{"claude-3"}, "claude-3.5") {
		t.Error("single model list should not match different model")
	}
}
