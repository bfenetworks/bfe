// Copyright (c) 2019 Baidu, Inc.
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

// backend with round robin    

package bal_slb

import (
	"github.com/baidu/bfe/bfe_balance/backend"
	"github.com/baidu/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

type BackendRR struct {
	weight  int                 // weight of this backend
	current int                 // current weight
	backend *backend.BfeBackend // point to BfeBackend
}

func NewBackendRR() *BackendRR {
	backendRR := new(BackendRR)
	backendRR.backend = backend.NewBfeBackend()

	return backendRR
}

// Init initialize BackendRR with BackendConf
func (backRR *BackendRR) Init(subClusterName string, conf *cluster_table_conf.BackendConf) {
	backRR.weight = *conf.Weight
	backRR.current = *conf.Weight

	back := backRR.backend
	back.Init(subClusterName, conf)
}

func (backRR *BackendRR) UpdateWeight(weight int) {
	backRR.weight = weight

	// if weight > 0, don't touch backRR.current
	if weight <= 0 {
		backRR.current = 0
	}
}

func (backRR *BackendRR) Release() {
	backRR.backend.Release()
}

func (backRR *BackendRR) MatchAddrPort(addr string, port int) bool {
	back := backRR.backend
	return back.Addr == addr && back.Port == port
}
