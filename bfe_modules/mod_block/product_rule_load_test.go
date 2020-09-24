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

package mod_block

import (
	"testing"
)

func TestBlockConfLoad_1(t *testing.T) {
	config, err := ProductRuleConfLoad("./testdata/block_rules_1.conf")
	if err != nil {
		t.Errorf("get err from ProductRuleConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config["pn"]) != 1 {
		t.Errorf("len(config.Config['pn']) should be 1")
		return
	}
}

func TestBlockConfLoad_2(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/block_rules_2.conf")
	if err == nil {
		t.Error("err should not be nil")
		return
	}
}

func TestBlockConfLoad_3(t *testing.T) {
	_, err := ProductRuleConfLoad("./testdata/block_rules_3.conf")
	if err == nil {
		t.Error("err should not be nil")
		return
	}
}
