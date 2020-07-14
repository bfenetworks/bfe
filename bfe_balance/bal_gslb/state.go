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

package bal_gslb

// SubClusterState is state of sub-cluster.
type SubClusterState struct {
	BackendNum int // number of backends
}

// GslbState is state of cluster.
type GslbState struct {
	SubClusters map[string]*SubClusterState // state of sub-cluster
	BackendNum  int                         // number of cluster backend
}

func State(bal *BalanceGslb) *GslbState {
	gslbState := new(GslbState)
	gslbState.SubClusters = make(map[string]*SubClusterState)

	bal.lock.Lock()

	for _, sub := range bal.subClusters {
		subState := &SubClusterState{
			BackendNum: sub.Len(),
		}

		gslbState.SubClusters[sub.Name] = subState
		gslbState.BackendNum += subState.BackendNum
	}

	bal.lock.Unlock()

	return gslbState
}
