// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"testing"
)

func TestKeepAliveDataLoad_1(t *testing.T) {
	data, err := KeepAliveDataLoad("./testdata/tcp_keepalive.data")
	if err != nil {
		t.Errorf("get err from ProductRuleConfLoad(): %s", err.Error())
		return
	}

	if len(data.Config) != 2 {
		t.Errorf("len(config.Config) should be 2, but is %d", len(data.Config))
		return
	}

	if len(data.Config["product1"]) != 3 {
		t.Errorf("len(data.Config[product1]) should be 3, but is %d", len(data.Config["product1"]))
		return
	}
}

// invalid format of data
func TestKeepAliveDataLoad_2(t *testing.T) {
	_, err := KeepAliveDataLoad("./testdata/tcp_keepalive_2.data")
	if err == nil {
		t.Error("err should not be nil")
		return
	}
}

// invalid format of ip
func TestKeepAliveDataLoad_3(t *testing.T) {
	_, err := KeepAliveDataLoad("./testdata/tcp_keepalive_3.data")
	if err == nil {
		t.Errorf("err should not be nil: %v", err)
		return
	}
}

func TestFormatIP(t *testing.T) {
	ret1, _ := formatIP("2001:0db8:02de:0000:0000:0000:0000:0e13")
	ret2, _ := formatIP("2001:db8:2de:000:000:000:000:e13")
	ret3, _ := formatIP("2001:db8:2de:0:0:0:0:e13")
	expect1 := "2001:db8:2de::e13"

	if ret1 != expect1 || ret2 != expect1 || ret3 != expect1 {
		t.Errorf("ret should equal to %s", expect1)
		return
	}

	ret5, _ := formatIP("2001:db8:2de:0:0:0:0:e13")
	ret6, _ := formatIP("2001:db8:2de::e13")
	expect2 := "2001:db8:2de::e13"
	if ret5 != expect2 || ret6 != expect2 {
		t.Errorf("ret should equal to %s", expect2)
		return
	}

	_, err := formatIP("2001::25de::cade")
	if err == nil {
		t.Error("err should not be nil, 2001::25de::cade is invalid ip")
		return
	}

	_, err = formatIP("127.1")
	if err == nil {
		t.Error("err should not be nil, 127.1 is invalid ip")
		return
	}
}
