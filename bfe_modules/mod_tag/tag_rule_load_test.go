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

package mod_tag

import (
	"strings"
	"testing"
)

func TestTagRuleFileLoadCase1(t *testing.T) {
	tagRuleConf, err := TagRuleFileLoad("testdata/mod_tag/tag_rule.data")
	if err != nil {
		t.Errorf("should have no error, but error is %v", err)
	}

	expectVersion := "20200218210000"
	if tagRuleConf.Version != expectVersion {
		t.Errorf("Version should be %s, but it's %s", expectVersion, tagRuleConf.Version)
	}

	if tagRuleConf.Config == nil {
		t.Errorf("Config should not be nil")
	}

	ruleList, ok := tagRuleConf.Config[expectProduct]
	if !ok {
		t.Errorf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Errorf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}

func TestTagRuleFileLoadCase2(t *testing.T) {
	_, err := TagRuleFileLoad("testdata/mod_tag/tag_rule.data1")
	if err == nil {
		t.Errorf("should have error")
	}

	if !strings.Contains(err.Error(), "TagName may be empty") {
		t.Errorf("error message is not expected: %v", err)
	}
}

func TestTagRuleFileLoadCase3(t *testing.T) {
	_, err := TagRuleFileLoad("testdata/mod_tag/tag_rule.data2")
	if err == nil {
		t.Errorf("should have error")
	}

	if !strings.Contains(err.Error(), "TagValue may be empty") {
		t.Errorf("error message is not expected: %v", err)
	}
}
