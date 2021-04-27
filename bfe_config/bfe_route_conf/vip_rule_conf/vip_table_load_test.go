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

package vip_rule_conf

import (
	"fmt"
	"net"
	"testing"
)

func TestVipTableLoad_1(t *testing.T) {
	config, err := VipRuleConfLoad("./testdata/vip_table_1.data")
	if err != nil {
		t.Error(fmt.Sprintf("get err from VipRuleConfLoad():%s", err.Error()))
		return
	}

	if config.VipMap["10.10.10.1"] != "pb" {
		t.Error("config.VipMap['10.10.10.1'] should be 'pb'")
	}

	if config.Version != "1234" {
		t.Error("config.Version should be '1234'")
	}
}

func TestVipTableLoad_2(t *testing.T) {
	_, err := VipRuleConfLoad("./testdata/vip_table_2.data")
	if err == nil {
		t.Error("it should be error in VipRuleConfLoad()")
	}
}

func TestVipTableLoad_3(t *testing.T) {
	config, err := VipRuleConfLoad("./testdata/vip_table_3.data")
	if err != nil {
		t.Errorf("get err from VipRuleConfLoad():%s", err.Error())
		return
	}

	vip := net.ParseIP("2001:0:1111:A:B0::9000:200")
	if config.VipMap[vip.String()] != "pb" {
		t.Errorf("config.VipMap['2001:0:1111:A:B0::9000:200'] should be 'pb', not %s",
			config.VipMap[vip.String()])
	}
}
