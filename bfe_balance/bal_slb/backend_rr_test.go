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

package bal_slb

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

func TestBackendRRInit_case1(t *testing.T) {
	var conf cluster_table_conf.BackendConf
	conf.Name = new(string)
	*conf.Name = "example05.instance"
	conf.Addr = new(string)
	*conf.Addr = "10.26.35.33"
	conf.Port = new(int)
	*conf.Port = 8000
	conf.Weight = new(int)
	*conf.Weight = 10

	backendRR := NewBackendRR()
	backendRR.Init("example.cluster", &conf)

	if backendRR.weight != 10 * 100 {
		t.Error("backend.weight should be 10 * 100")
	}

	if backendRR.current != 10 * 100 {
		t.Error("backend.current should be 10 * 100")
	}

	backend := backendRR.backend
	if backend.Port != 8000 {
		t.Error("backend.port should be 8000")
	}

	if !backend.Avail() {
		t.Error("backend.available should be true")
	}
}
