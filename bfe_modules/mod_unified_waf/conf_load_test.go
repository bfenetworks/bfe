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

// config path is not empty, correct
func TestConfLoad_1(t *testing.T) {
	confPath := "./testdata/mod_unified_waf.conf"
	ModWafDataPath := "./testdata/mod_unified_waf.data"
	productParamPath := "./testdata/product_param.data"
	albWafInstancesPath := "./testdata/alb_waf_instances.data"
	WafProductName := "BFEMockWaf"

	conf, err := ConfLoad(confPath, "")
	if err != nil {
		t.Errorf("ConfLoad(): %v", err)
		return
	}

	if conf.Basic.WafProductName != WafProductName {
		t.Errorf("WafProductName should be %s not %s", WafProductName, conf.Basic.WafProductName)
	}

	if conf.ConfigPath.ModWafDataPath != ModWafDataPath {
		t.Errorf("ModWafDataPath should be %s not %s", ModWafDataPath, conf.ConfigPath.ModWafDataPath)
	}
	if conf.ConfigPath.ProductParamPath != productParamPath {
		t.Errorf("ProductParamPath should be %s not %s", productParamPath, conf.ConfigPath.ProductParamPath)
	}
	if conf.ConfigPath.AlbWafInstancesPath != albWafInstancesPath {
		t.Errorf("AlbWafInstancesPath should be %s not %s", albWafInstancesPath, conf.ConfigPath.AlbWafInstancesPath)
	}
}
