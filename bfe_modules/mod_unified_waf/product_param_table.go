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
	"sync"

	"github.com/bfenetworks/bfe/bfe_basic"
)

type ProductParamTable struct {
	lock      sync.RWMutex
	prodParam *ProductParams
	version   string
}

func NewProductParamTable() *ProductParamTable {
	t := new(ProductParamTable)
	t.prodParam = &ProductParams{}

	return t
}

func (t *ProductParamTable) Update(param ProductParams, ver string) {
	t.lock.Lock()
	t.prodParam = &param
	t.version = ver
	t.lock.Unlock()
}

func (t *ProductParamTable) GetRequestWafParam(req *bfe_basic.Request) *WafParam {
	t.lock.RLock()
	table := t.prodParam
	t.lock.RUnlock()

	productName := req.Route.Product
	if param, ok := (*table)[productName]; ok {
		return &param
	}

	return nil
}

func (t *ProductParamTable) Version() string {
	t.lock.RLock()
	version := t.version
	t.lock.RUnlock()

	return version
}
