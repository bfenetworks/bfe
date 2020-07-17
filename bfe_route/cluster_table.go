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

// table for maintain backend cluster

package bfe_route

import "fmt"

import (
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_route/bfe_cluster"
)

// ClusterMap holds mappings from clusterName to cluster.
type ClusterMap map[string]*bfe_cluster.BfeCluster

type ClusterTable struct {
	clusterTable ClusterMap
	versions     ClusterVersion
}

type ClusterVersion struct {
	ClusterConfVer string // version of cluster-conf
}

func newClusterTable() *ClusterTable {
	ct := new(ClusterTable)
	return ct
}

func (t *ClusterTable) Init(clusterConfFilename string) error {
	// init cluster basic
	t.clusterTable = make(ClusterMap)
	clusterConf, err := cluster_conf.ClusterConfLoad(clusterConfFilename)
	if err != nil {
		return err
	}

	t.BasicInit(clusterConf)

	log.Logger.Info("init cluster table success")
	return nil
}

func (t *ClusterTable) BasicInit(clusterConfs cluster_conf.BfeClusterConf) {
	t.clusterTable = make(ClusterMap)

	for clusterName, clusterConf := range *clusterConfs.Config {
		// create new cluster
		cluster := bfe_cluster.NewBfeCluster(clusterName)

		// initialize
		cluster.BasicInit(clusterConf)
		// add cluster to clusterTable
		t.clusterTable[clusterName] = cluster
	}

	t.versions.ClusterConfVer = *clusterConfs.Version
}

func (t *ClusterTable) Lookup(clusterName string) (*bfe_cluster.BfeCluster, error) {
	// lookup in cluster table
	cluster, ok := t.clusterTable[clusterName]
	if !ok {
		return cluster, fmt.Errorf("no cluster found for %s", clusterName)
	}

	return cluster, nil
}

func (t *ClusterTable) GetVersions() ClusterVersion {
	return t.versions
}

func (t *ClusterTable) ClusterMap() ClusterMap {
	return t.clusterTable
}
