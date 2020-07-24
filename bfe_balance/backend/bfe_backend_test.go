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

package backend

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

func TestBfeBackendInit_case1(t *testing.T) {
	var conf cluster_table_conf.BackendConf
	conf.Name = new(string)
	*conf.Name = "tc-example05.tc"

	conf.Addr = new(string)
	*conf.Addr = "10.26.35.33"

	conf.Port = new(int)
	*conf.Port = 8000

	conf.Weight = new(int)
	*conf.Weight = 10

	backend := NewBfeBackend()
	backend.Init("tc-example.tc", &conf)

	if backend.Port != 8000 {
		t.Error("backend.Port should be 8000")
		return
	}

	if !backend.avail {
		t.Error("backend.Available should be true")
		return
	}

	backend.SetAvail(false)

	if backend.Avail() != false {
		t.Error("backend should not be avail")
	}
}
