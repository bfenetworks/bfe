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

import (
	"fmt"
)

import (
	"github.com/bfenetworks/bfe/bfe_balance/backend"
	"github.com/bfenetworks/bfe/bfe_balance/bal_slb"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
)

// type of sub cluster
const (
	TypeGslbNormal    = 0 // normal sub cluster
	TypeGslbBlackhole = 1 // gslb black hole
)

type SubCluster struct {
	Name     string             // name of sub cluster
	sType    int                // TypeGslbNormal, or TypeGslbBlackhole
	backends *bal_slb.BalanceRR // backend with round-robin
	weight   int                // weight between sub-clusters
}

func newSubCluster(name string) *SubCluster {
	sub := new(SubCluster)

	// set name
	sub.Name = name

	// set type
	if name == "GSLB_BLACKHOLE" {
		sub.sType = TypeGslbBlackhole
	} else {
		sub.sType = TypeGslbNormal
	}

	// create backends
	sub.backends = bal_slb.NewBalanceRR(name)

	return sub
}

// init initializes sub-cluster with backend list.
func (sub *SubCluster) init(backends cluster_table_conf.SubClusterBackend) {
	sub.backends.Init(backends)
}

// update updates sub-cluster with backend list.
func (sub *SubCluster) update(backends cluster_table_conf.SubClusterBackend) {
	sub.backends.Update(backends)
}

// release releases sub-cluster.
func (sub *SubCluster) release() {
	sub.backends.Release()
}

// Len return length of sub-cluster.
func (sub *SubCluster) Len() int {
	return sub.backends.Len()
}

func (sub *SubCluster) balance(algor int, key []byte) (*backend.BfeBackend, error) {
	if sub.backends.Len() == 0 {
		return nil, fmt.Errorf("no backend in sub cluster [%s]", sub.Name)
	}

	// balance from sub-cluster
	return sub.backends.Balance(algor, key)
}

func (sub *SubCluster) setSlowStart(slowStartTime int) {
	sub.backends.SetSlowStart(slowStartTime)
}

// SubClusterList is a list of sub-cluster.
type SubClusterList []*SubCluster

type SubClusterListSorter struct {
	l SubClusterList
}

func (s SubClusterListSorter) Len() int {
	return len(s.l)
}

func (s SubClusterListSorter) Swap(i, j int) {
	s.l[i], s.l[j] = s.l[j], s.l[i]
}

func (s SubClusterListSorter) Less(i, j int) bool {
	return s.l[i].Name < s.l[j].Name
}
