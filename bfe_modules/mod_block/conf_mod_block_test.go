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
	"fmt"
	"testing"
)

func Test_conf_mod_block_case1(t *testing.T) {
	config, err := ConfLoad("./testdata/conf_mod_block/bfe_1.conf", "")
	if err != nil {
		msg := fmt.Sprintf("confModBlockLoad():err=%s", err.Error())
		t.Error(msg)
		return
	}

	if config.Basic.ProductRulePath != "/home/bfe/conf/rule.data" {
		t.Error("ProductRulePath should be /home/bfe/conf/rule.data")
	}
	if config.Basic.IPBlocklistPath != "/home/bfe/conf/ip.data" {
		t.Error("IPBlocklistPath should be /home/bfe/conf/ip.data")
	}
}

/* load config from config file    */
func Test_conf_mod_block_case2(t *testing.T) {
	// illegal value
	config, _ := ConfLoad("./testdata/conf_mod_block/bfe_2.conf", "")

	// use default value
	if config.Basic.ProductRulePath != "mod_block/block_rules.data" {
		t.Error("ProductRulePath should be mod_block/block_rules.data")
	}
	if config.Basic.IPBlocklistPath != "mod_block/ip_blocklist.data" {
		t.Error("IPBlocklistPath should be mod_block/ip_blocklist.data")
	}
}
