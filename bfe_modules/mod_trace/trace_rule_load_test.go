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

package mod_trace

import "testing"

const (
	expectProduct = "example_product"
)

func TestTraceRuleFileLoadCase1(t *testing.T) {
	traceRuleConf, err := TraceRuleFileLoad("testdata/mod_trace/trace_rule.data")
	if err != nil {
		t.Errorf("should have no error, but error is %v", err)
	}

	expectVersion := "20200316215500"
	if traceRuleConf.Version != expectVersion {
		t.Errorf("Version should be %s, but it's %s", expectVersion, traceRuleConf.Version)
	}

	if traceRuleConf.Config == nil {
		t.Errorf("Config should not be nil")
	}

	ruleList, ok := traceRuleConf.Config[expectProduct]
	if !ok {
		t.Errorf("config should have product: %s", expectProduct)
	}

	if len(ruleList) != 1 {
		t.Errorf("len(ruleList) should be 1, but it's %d", len(ruleList))
	}
}
