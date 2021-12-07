// Copyright (c) 2019 The BFE Authors.
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

package mod_auth_request

import (
	"strings"
	"testing"
)

func TestAuthRequestRuleFileLoadCase1(t *testing.T) {
	conf, err := AuthRequestRuleFileLoad("testdata/mod_auth_request/auth_request_rule.data")
	if err != nil {
		t.Fatalf("should have no error, but error is %v", err)
	}

	expectVersion := "auth_request_rule_version"
	if conf.Version != expectVersion {
		t.Fatalf("Version should be %s, but it's %s", expectVersion, conf.Version)
	}

	if conf.Config == nil {
		t.Fatalf("Config should not be nil")
	}

	ruleList, ok := conf.Config[expectProduct]
	if !ok {
		t.Fatalf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Fatalf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}

func TestAuthRequestRuleFileLoadCase2(t *testing.T) {
	_, err := AuthRequestRuleFileLoad("testdata/mod_auth_request/auth_request_rule_no_version.data")
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "no Version") {
		t.Fatalf("error message is not expected: %v", err)
	}
}

func TestAuthRequestRuleFileLoadCase3(t *testing.T) {
	_, err := AuthRequestRuleFileLoad("testdata/mod_auth_request/auth_request_rule_no_config.data")
	if err == nil {
		t.Fatalf("should have error")
	}

	if !strings.Contains(err.Error(), "no Config") {
		t.Fatalf("error message is not expected: %v", err)
	}
}
