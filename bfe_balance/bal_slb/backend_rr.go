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

// backend with round-robin

package bal_slb

import (
	"time"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

type WeightSS struct {
	final         int       // final target weight after slow-start
	slowStartTime int       // time for backend increases the weight to the full value, in seconds
	startTime     time.Time // time of the first request
}

type BackendRR struct {
	weight      int                 // weight of this backend
	current     int                 // current weight
	backend     *backend.BfeBackend // point to BfeBackend
	inSlowStart bool                // indicate if in slow-start phase
	weightSS    WeightSS            // slow_start related parameters
}

func NewBackendRR() *BackendRR {
	backendRR := new(BackendRR)
	backendRR.backend = backend.NewBfeBackend()

	return backendRR
}

// Init initialize BackendRR with BackendConf
func (backRR *BackendRR) Init(subClusterName string, conf *cluster_table_conf.BackendConf) {
	// scale up 100 times from conf file
	backRR.weight = *conf.Weight * 100
	backRR.current = backRR.weight
	backRR.weightSS.final = backRR.weight

	back := backRR.backend
	back.Init(subClusterName, conf)
}

func (backRR *BackendRR) UpdateWeight(weight int) {
	backRR.weight = weight * 100

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

func (backRR *BackendRR) initSlowStart(ssTime int) {
	backRR.weightSS.slowStartTime = ssTime
	if backRR.weightSS.slowStartTime == 0 {
		backRR.inSlowStart = false
	} else {
		backRR.weightSS.startTime = time.Now()
		backRR.inSlowStart = true

		// set weight/current to 1, to avoid no traffic allowed at the beginning of start
		backRR.weight = 1
		backRR.current = 1
	}
}

func (backRR *BackendRR) updateSlowStart() {
	if backRR.inSlowStart {
		current := time.Duration(backRR.weightSS.final) * time.Since(backRR.weightSS.startTime)
		if backRR.weightSS.slowStartTime != 0 {
			current /= time.Duration(backRR.weightSS.slowStartTime) * time.Second
			backRR.weight = int(current)
		} else {
			backRR.weight = backRR.weightSS.final
		}
		if backRR.weight >= backRR.weightSS.final {
			backRR.weight = backRR.weightSS.final
			backRR.inSlowStart = false
		}
	}
}
