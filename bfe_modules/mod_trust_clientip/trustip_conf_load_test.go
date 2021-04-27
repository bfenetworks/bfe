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

package mod_trust_clientip

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_util/net_util"
)

func TestTrustIPConfLoad_1(t *testing.T) {
	config, err := TrustIPConfLoad("./testdata/trust_ip_1.conf")
	if err != nil {
		t.Errorf("get err from TrustIPConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config["cdn"]) != 2 {
		t.Errorf("len(config.Config['cdn']) should be 2")
		return
	}

	addr := net_util.ParseIPv4("119.75.215.0")
	if !(*config.Config["cdn"])[0].Begin.Equal(addr) {
		t.Errorf("config.Config['cdn'][0].Begin should be '119.75.215.0'")
		return
	}
}

func TestTrustIPConfLoad_2(t *testing.T) {
	_, err := TrustIPConfLoad("./testdata/trust_ip_2.conf")
	if err == nil {
		t.Error("TrustIPConfLoad() should return error")
		return
	}
}

func TestTrustIPConfLoad_3(t *testing.T) {
	_, err := TrustIPConfLoad("./testdata/trust_ip_3.conf")
	if err != nil {
		t.Error("TrustIPConfLoad() should return nil", err)
		return
	}
}
