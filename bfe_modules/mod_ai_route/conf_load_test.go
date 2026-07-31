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

func TestConfLoadNormalCase(t *testing.T) {
	fileName := "testdata/mod_ai_route/mod_ai_route.conf"
	conf, err := ConfLoad(fileName, "")
	if err != nil {
		t.Fatalf("load conf should success, got err: %s", err)
	}

	if conf.Basic.RouteRulePath != "../data/mod_ai_route/ai_route.data" {
		t.Errorf("RouteRulePath mismatch: expected ../data/mod_ai_route/ai_route.data, got %s", conf.Basic.RouteRulePath)
	}

	if conf.Log.OpenDebug != true {
		t.Errorf("OpenDebug mismatch: expected true, got %v", conf.Log.OpenDebug)
	}
}

func TestConfLoadEmptyRouteRulePath(t *testing.T) {
	fileName := "testdata/mod_ai_route/mod_ai_route.conf.empty"
	if _, err := ConfLoad(fileName, ""); err == nil {
		t.Error("RouteRulePath is empty, load conf should fail")
	}
}

func TestConfLoadFileNotFound(t *testing.T) {
	if _, err := ConfLoad("testdata/mod_ai_route/non_existent.conf", ""); err == nil {
		t.Error("load non-existent conf should fail")
	}
}
