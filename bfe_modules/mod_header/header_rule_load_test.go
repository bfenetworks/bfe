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

package mod_header

import (
	"testing"
)

// abnormal type
const (
	abnormalTypeNilCondition = iota
	abnormalTypeNilActions
	abnormalTypeEmptyActions // actions is empty slice but not nil
	abnormalTypeNilLast
)

func makeHeaderRuleFile() HeaderRuleFile {
	cond := "condition"
	last := false
	cmd := "REQ_HEADER_ADD"
	action := ActionFile{
		Cmd:    &cmd,
		Params: []string{"header1", "value1"},
	}

	return HeaderRuleFile{
		Cond:    &cond,
		Actions: &ActionFileList{action},
		Last:    &last,
	}
}

func makeAbnormalHeaderRuleFile(abnormalType int) HeaderRuleFile {
	headerRule := makeHeaderRuleFile()
	switch abnormalType {
	case abnormalTypeNilCondition:
		headerRule.Cond = nil
	case abnormalTypeNilActions:
		headerRule.Actions = nil
	case abnormalTypeEmptyActions:
		actions := make(ActionFileList, 0)
		headerRule.Actions = &actions
	case abnormalTypeNilLast:
		headerRule.Last = nil
	default:
		return HeaderRuleFile{}
	}
	return headerRule
}

func TestHeaderConfLoad(t *testing.T) {
	_, err := HeaderConfLoad("./testdata/mod_header/header_rule.data")
	if err != nil {
		t.Errorf("HeaderConfLoad() failed for %v", err)
	}

	//Negative case: not exist conf file
	_, err = HeaderConfLoad("./testdata/not_exist.conf")
	if err == nil {
		t.Error("HeaderConfLoad() failed for not exist conf file")
	}
}

// normal case
func TestHeaderRuleCheckCase1(t *testing.T) {
	headerRule := makeHeaderRuleFile()

	// invoke HeaderRuleCheck()
	err := HeaderRuleCheck(headerRule)

	// verify
	if err != nil {
		t.Errorf("HeaderRuleCheck(): %s, with Cond=%+v, Action=%+v, Last=%+v",
			err, headerRule.Cond, headerRule.Actions, headerRule.Last)
	}
}

// abnormal cases
func TestHeaderRuleCheckCase2(t *testing.T) {
	testCases := []int{
		abnormalTypeNilCondition,
		abnormalTypeNilActions,
		abnormalTypeEmptyActions,
		abnormalTypeNilLast,
	}

	for _, abnormalType := range testCases {
		headerRule := makeAbnormalHeaderRuleFile(abnormalType)

		// invoke HeaderRuleCheck()
		err := HeaderRuleCheck(headerRule)

		// verify
		if err == nil {
			t.Errorf("HeaderRuleCheck() should fail, with Cond=%+v, Action=%+v, Last=%+v",
				headerRule.Cond, headerRule.Actions, headerRule.Last)
		}
	}
}
