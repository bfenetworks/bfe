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

package mod_ai_route

import (
	"testing"
)

func TestAiRouteDataLoadValid(t *testing.T) {
	fileName := "testdata/mod_ai_route/ai_route.data"
	data, err := AiRouteDataLoad(fileName)
	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}

	if data.Version != "20260720150000" {
		t.Errorf("Version mismatch: expected 20260720150000, got %s", data.Version)
	}

	if len(data.RouteRules) != 3 {
		t.Errorf("RouteRules count: expected 3, got %d", len(data.RouteRules))
	}

	if len(data.ApikeyRouteTableBindings) != 3 {
		t.Errorf("Bindings count: expected 3, got %d", len(data.ApikeyRouteTableBindings))
	}

	apikeyTable, ok := data.RouteRules["apikey_ak_user_a"]
	if !ok {
		t.Fatal("route rule apikey_ak_user_a not found")
	}
	if apikeyTable.Type != RouteTypeApikey {
		t.Errorf("apikey_ak_user_a type: expected apikey, got %s", apikeyTable.Type)
	}
	if len(apikeyTable.Rules) != 2 {
		t.Errorf("apikey_ak_user_a rules count: expected 2, got %d", len(apikeyTable.Rules))
	}
}

func TestAiRouteDataLoadFileNotFound(t *testing.T) {
	if _, err := AiRouteDataLoad("testdata/mod_ai_route/non_existent.data"); err == nil {
		t.Error("expected error for non-existent file")
	}
}
