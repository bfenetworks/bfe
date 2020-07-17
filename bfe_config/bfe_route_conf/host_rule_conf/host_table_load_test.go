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

package host_rule_conf

import (
	"testing"
)

func TestHostTableLoad_1(t *testing.T) {
	// load host table from file
	config, err := HostRuleConfLoad("./testdata/host_table_1.conf")

	if err != nil {
		t.Errorf("get err from HostTableLoad():%s", err.Error())
		return
	}

	t.Logf("TestHostTableLoad_1():len(config)=%d\n", len(config.HostMap))
	t.Logf("TestProductRuleLoad_1():config=%s\n", config)

	if config.HostMap["a1.example.com"] != "A" {
		t.Error("config.HostMap['a1.example.com'] should be 'A'")
	}

	if config.Version != "1234" {
		t.Error("config.Version should be '1234'")
	}
}

func TestHostTableLoad_2(t *testing.T) {
	// load host table from file
	_, err := HostRuleConfLoad("./testdata/host_table_2.conf")

	if err == nil {
		t.Error("it should be error in HostTableLoad()")
	} else {
		t.Logf("err in HostTableLoad():%s\n", err.Error())
	}
}

func TestHostTableLoad_3(t *testing.T) {
	if _, err := HostRuleConfLoad("./testdata/host_table_3.conf"); err == nil {
		t.Error("it should be error in HostTableLoad()")
	} else {
		t.Logf("err in HostTableLoad(): %s\n", err.Error())
	}
}
