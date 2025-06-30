// Copyright (c) 2025 The BFE Authors.
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

package mod_unified_waf

import (
	"testing"
)

func TestWafDataParamLoadAndCheck_1(t *testing.T) {
	ModWafDataPath := "./testdata/mod_unified_waf.data"

	conf, err := WafDataParamLoadAndCheck(ModWafDataPath)
	if err != nil {
		t.Errorf("WafDataParamLoadAndCheck(): %v", err)
		return
	}

	if conf.Config.WafClient.ConnectTimeout != 30 {
		t.Errorf("WafClient.ConnectTimeout != 30")
		return
	}
	if conf.Config.WafDetect.RetryMax != 2 {
		t.Errorf("WafDetect.RetryMax != 2")
		return
	}
	if conf.Config.WafDetect.ReqTimeout != 50 {
		t.Errorf("WafDetect.ReqTimeout != 50")
		return
	}
}

func TestWafDataParamLoadAndCheck_2(t *testing.T) {
	ModWafDataPath := "./testdata/mod_unified_waf_2.data"

	conf, err := WafDataParamLoadAndCheck(ModWafDataPath)
	if err != nil {
		t.Errorf("WafDataParamLoadAndCheck(): %v", err)
		return
	}

	if conf.Config.WafClient.ConnectTimeout != 30 {
		t.Errorf("WafClient.ConnectTimeout != 30")
		return
	}
	if conf.Config.WafDetect.RetryMax != 2 {
		t.Errorf("WafDetect.RetryMax != 2")
		return
	}
	if conf.Config.WafDetect.ReqTimeout != 50 {
		t.Errorf("WafDetect.ReqTimeout != 50")
		return
	}
	reqt := conf.Config.GetReqTimeout(100)
	if reqt != 50 {
		t.Errorf("conf.Config.GetReqTimeout(100) != 50, actual:%d", reqt)
		return
	}
}
