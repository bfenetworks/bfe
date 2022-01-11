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

func TestKeepAliveTable(t *testing.T) {
	table := NewKeepAliveTable()
	path := "./testdata/tcp_keepalive.data"
	data, err := KeepAliveDataLoad(path)
	if err != nil {
		t.Errorf("KeepAliveDataLoad(%s) error: %v", path, err)
		return
	}

	table.Update(data)
	if len(table.productRules) != 2 {
		t.Errorf("table.Update error: rules length should be 2 but %d", len(table.productRules))
		return
	}

	_, ok := table.Search("product1")
	if !ok {
		t.Errorf("table.Search error: product1 should exist")
		return
	}
}
