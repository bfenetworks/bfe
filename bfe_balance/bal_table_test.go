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

package bfe_balance

import (
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
)

func TestBalTableConfLoad(t *testing.T) {
	balTable := NewBalTable(nil)

	gslbFile := ""
	clusterTableFile := "testdata/bal_table/case1/cluster_table.data"
	if _, _, err := balTable.BalTableConfLoad(gslbFile, clusterTableFile); err == nil {
		t.Errorf("BalTableConfLoad case 0 should return err")
		return
	}

	gslbFile = "testdata/bal_table/case1/gslb.data"
	clusterTableFile = ""
	if _, _, err := balTable.BalTableConfLoad(gslbFile, clusterTableFile); err == nil {
		t.Errorf("BalTableConfLoad case 0 should return err")
		return
	}

	gslbFile = "testdata/bal_table/case1/gslb.data"
	clusterTableFile = "testdata/bal_table/case1/cluster_table.data"
	if _, _, err := balTable.BalTableConfLoad(gslbFile, clusterTableFile); err != nil {
		t.Errorf("err in BalTableConfLoad():%s", err.Error())
		return
	}
}

func TestGslbInit(t *testing.T) {
	balTable := NewBalTable(nil)
	gslbFile := "testdata/bal_table/case1/gslb.data"

	clusters := new(gslb_conf.GslbClustersConf)
	*clusters = make(map[string]gslb_conf.GslbClusterConf, 1)
	gslbClusterConf := gslb_conf.GslbClusterConf{
		"a": 0,
		"b": 0,
	}
	(*clusters)["cluster_a"] = gslbClusterConf
	hostName := "gslb-sch00.a01"
	ts := "201504131601"
	gslbConf := gslb_conf.GslbConf{
		Clusters: clusters,
		Hostname: &hostName,
		Ts:       &ts,
	}

	err := balTable.gslbInit(gslbConf)
	if err == nil {
		t.Errorf("GslbInit: case 0 should return err")
		return
	}

	gslbConf, err = gslb_conf.GslbConfLoad(gslbFile)
	if err != nil {
		t.Errorf("GslbInit: load case 1 test file err [%s]", err)
		return
	}

	if err = balTable.gslbInit(gslbConf); err != nil {
		t.Errorf("GslbInit: case 1 should return nil.")
		return
	}

	name := "a"
	addr := "1"
	port := 1
	weight := 0
	subClusterBackend := []*cluster_table_conf.BackendConf{
		{
			Name:   &name,
			Addr:   &addr,
			Port:   &port,
			Weight: &weight,
		},
	}

	clusterBackend := cluster_table_conf.ClusterBackend{
		"a": subClusterBackend,
	}
	allClusterBackend := cluster_table_conf.AllClusterBackend{
		"cluster_a": clusterBackend,
	}
	version := "12"
	backendConf := cluster_table_conf.ClusterTableConf{
		Config:  &allClusterBackend,
		Version: &version,
	}

	if err = balTable.backendInit(backendConf); err == nil {
		t.Error("BackendInit: case 0 should return err")
		return
	}

	backendFile := "testdata/bal_table/case1/cluster_table.data"
	backendConf, err = cluster_table_conf.ClusterTableLoad(backendFile)

	if err != nil {
		t.Errorf("BackendInit: load backend test1 file err [%s]", err)
		return
	}

	if err = balTable.backendInit(backendConf); err != nil {
		t.Errorf("BackendInit: case 1 should return nil. [%s]", err)
		return
	}
}

func TestInit(t *testing.T) {
	balTable := NewBalTable(nil)
	gslbFile := ""
	backendFile := "testdata/bal_table/case1/cluster_table.data"

	if err := balTable.Init(gslbFile, backendFile); err == nil {
		t.Error("Init: case 0 should return err")
		return
	}

	gslbFile = "testdata/bal_table/case1/gslb.data"
	backendFile = ""
	if err := balTable.Init(gslbFile, backendFile); err == nil {
		t.Error("Init: case 1 should return err")
		return
	}

	gslbFile = "testdata/bal_table/case1/gslb.data"
	backendFile = "testdata/bal_table/case1/cluster_table.data"

	if err := balTable.Init(gslbFile, backendFile); err != nil {
		t.Errorf("Init: case 2 should return nil. err [%s]", err)
		return
	}
}

func TestBalTableReload(t *testing.T) {
	gslbFile := "testdata/bal_table/case1/gslb.data"
	backendFile := "testdata/bal_table/case1/cluster_table.data"
	reloadGslbFile := "testdata/bal_table/case2/gslb.data"
	reloadBackendFile := "testdata/bal_table/case2/cluster_table.data"

	balTable := NewBalTable(nil)
	if err := balTable.Init(gslbFile, backendFile); err != nil {
		t.Errorf("BalTableReload: balTable.Init err %s", err)
		return
	}

	gslbConf, backendConf, err1 := balTable.BalTableConfLoad(reloadGslbFile, reloadBackendFile)
	if err1 != nil {
		t.Errorf("BalTableReload: case 0 BalTableConfLoad err [%s]", err1)
		return
	}

	if err := balTable.BalTableReload(gslbConf, backendConf); err != nil {
		t.Errorf("BalTableReload: case 0 should return nil. err [%s]", err)
		return
	}

	if balTable.versions.ClusterTableConfVer != "1234" {
		t.Errorf("BalTableReload: versions.ClusterTableConfVer should be '1234'")
		return
	}
	if balTable.versions.GslbConfTimeStamp != "1234" {
		t.Errorf("BalTableReload: versions.GslbConfTimeStamp should be '1234'")
		return
	}
	if balTable.versions.GslbConfSrc != "gslb-sch00.a01" {
		t.Errorf("BalTableReload: versions.GslbConfSrc should be 'gslb-sch00.a01', now[%s]",
			balTable.versions.GslbConfSrc)
		return
	}

	bt := balTable.balTable
	if len(bt) != 5 {
		t.Errorf("BalTableReload: len(balTable) should be 5! but return [%d]", len(bt))
		return
	}
	if _, ok := bt["cluster_c5"]; !ok {
		t.Error("BalTableReload: cluster_c5 should be in balTable!")
		return
	}

	balState := balTable.GetState()
	if balState.BackendNum != 6 {
		t.Errorf("BalTableReload: balState.BackendNum should be 6. not [%d]", balState.BackendNum)
		return
	}
}
