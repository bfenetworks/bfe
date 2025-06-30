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

func TestProductParamLoadAndCheck_1(t *testing.T) {
	productParamPath := "./testdata/product_param.data"

	conf, err := ProductParamLoadAndCheck(productParamPath)
	if err != nil {
		t.Errorf("ProductParamLoadAndCheck(): %v", err)
		return
	}

	p1, found := conf.Config["ProductA"]
	if !found {
		t.Errorf("ProductParamLoadAndCheck(): ProductA is not found")
		return
	}
	if p1.SendBody != false && p1.SendBodySize != 0 {
		t.Errorf("ProductParamLoadAndCheck(): ProductA param err: %v", p1)
		return
	}

	p2, found := conf.Config["ProductB"]
	if !found {
		t.Errorf("ProductParamLoadAndCheck(): ProductB is not found")
		return
	}
	if p2.SendBody != true && p2.SendBodySize != 4096 {
		t.Errorf("ProductParamLoadAndCheck(): ProductB param err: %v", p2)
		return
	}
}

func TestProductParamLoadAndCheck_2(t *testing.T) {
	productParamPath := "./testdata/product_param_1.data"

	_, err := ProductParamLoadAndCheck(productParamPath)
	if err == nil {
		t.Errorf("ProductParamLoadAndCheck() should return error")
		return
	}
}

func TestProductParamLoadAndCheck_3(t *testing.T) {
	productParamPath := "./testdata/product_param_2.data"

	cfg, err := ProductParamLoadAndCheck(productParamPath)
	if err != nil {
		t.Errorf("ProductParamLoadAndCheck(): %v", err)
		return
	}

	if cfg.Config["ProductA"].SendBody != false && cfg.Config["ProductA"].SendBodySize != 0 {
		t.Errorf("ProductA: %v", cfg.Config["ProductA"])
		return
	}
}

func TestProductParamLoadAndCheck_4(t *testing.T) {
	productParamPath := "./testdata/product_param_empty.data"

	_, err := ProductParamLoadAndCheck(productParamPath)
	if err != nil {
		t.Errorf("ProductParamLoadAndCheck() should not return error:%v", err)
		return
	}
}
