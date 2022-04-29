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
	"io/ioutil"
	"net"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

func loadJson(path string, v interface{}) error {
	x, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(x, &v)
}

func TestInit(t *testing.T) {
	var c cluster_table_conf.ClusterBackend
	var gb cluster_conf.GslbBasicConf
	var g gslb_conf.GslbClusterConf
	var err error

	loadJson("testdata/cluster1", &c)
	loadJson("testdata/gb1", &gb)
	loadJson("testdata/g1", &g)
	t.Logf("%v %v %v\n", c, gb, g)

	bal := NewBalanceGslb("cluster_demo")
	if err := bal.Init(g); err != nil {
		t.Errorf("init error %s", err)
	}
	t.Logf("%+v\n", bal)
	if bal.totalWeight != 100 || !bal.single || bal.subClusters[bal.avail].Name != "light.example.wt" || bal.retryMax != 3 || bal.crossRetry != 1 {
		t.Errorf("init error")
	}

	if len(bal.subClusters) != 3 {
		t.Errorf("cluster len error")
	}

	t.Logf("%+v", bal.subClusters[0])
	t.Logf("%+v", bal.subClusters[1])
	t.Logf("%+v", bal.subClusters[2])

	var c1 cluster_table_conf.ClusterBackend
	var gb1 cluster_conf.GslbBasicConf
	var g1 gslb_conf.GslbClusterConf
	loadJson("testdata/cluster2", &c1)
	loadJson("testdata/gb2", &gb1)
	loadJson("testdata/g2", &g1)

	err = cluster_conf.GslbBasicConfCheck(&gb1)
	if err != nil {
		t.Errorf("GslbBasicConfCheck err %s", err)
	}
	t.Logf("%v %v %v\n", c1, gb1, g1)
	if err := bal.Reload(g1); err != nil {
		t.Errorf("reload error %s", err)
	}

	bal.SetGslbBasic(gb1)

	t.Logf("%+v\n", bal)
	t.Logf("%+v", bal.subClusters[0])
	t.Logf("%+v", bal.subClusters[1])
	t.Logf("%+v", bal.subClusters[2])

	if bal.totalWeight != 90 || bal.single || bal.retryMax != 4 || bal.crossRetry != 2 {
		t.Errorf("init error")
	}
}

func prepareBalanceGslb(backendConf, gslbBasicConf, gslbClusterConf, clusterName string) *BalanceGslb {
	var c cluster_table_conf.ClusterBackend
	var gb cluster_conf.GslbBasicConf
	var g gslb_conf.GslbClusterConf

	loadJson(backendConf, &c)
	loadJson(gslbBasicConf, &gb)
	loadJson(gslbClusterConf, &g)

	bal := NewBalanceGslb(clusterName)
	bal.Init(g)
	bal.BackendReload(c)
	bal.SetGslbBasic(gb)

	return bal
}

func prepareRequest() *bfe_basic.Request {
	req := new(bfe_basic.Request)
	req.HttpRequest = new(bfe_http.Request)
	req.RemoteAddr = &net.TCPAddr{
		IP:   net.ParseIP("1.1.1.1"),
		Port: 80,
		Zone: "testZone",
	}
	req.ClientAddr = req.RemoteAddr
	return req
}

func SetReqHeader(req *bfe_basic.Request, key string) {
	if cookieKey, ok := cluster_conf.GetCookieKey(key); ok {
		req.CookieMap = make(map[string]*bfe_http.Cookie)
		cookie := &bfe_http.Cookie{Name: cookieKey, Value: "val"}
		req.CookieMap[cookie.Name] = cookie
	} else {
		req.HttpRequest.Header = make(bfe_http.Header)
		req.HttpRequest.Header.Set(key, "val")
	}
}

func TestSlowStart(t *testing.T) {
	t.Logf("bal_gslb_test: TestSlowStart")
	var c cluster_table_conf.ClusterBackend
	var gb cluster_conf.GslbBasicConf
	var g gslb_conf.GslbClusterConf
	var err error

	loadJson("testdata/cluster1", &c)
	loadJson("testdata/gb", &gb)
	loadJson("testdata/g1", &g)
	t.Logf("%v %v %v\n", c, gb, g)

	bal := NewBalanceGslb("cluster_dumi")
	if err := bal.Init(g); err != nil {
		t.Errorf("init error %s", err)
	}
	t.Logf("%+v\n", bal)
	if bal.totalWeight != 100 || !bal.single || bal.subClusters[bal.avail].Name != "light.example.wt" || bal.retryMax != 3 || bal.crossRetry != 1 {
		t.Errorf("init error")
	}

	if len(bal.subClusters) != 3 {
		t.Errorf("cluster len error")
	}

	t.Logf("%+v", bal.subClusters[0])
	t.Logf("%+v", bal.subClusters[1])
	t.Logf("%+v", bal.subClusters[2])

	var c1 cluster_table_conf.ClusterBackend
	var gb1 cluster_conf.GslbBasicConf
	var g1 gslb_conf.GslbClusterConf
	loadJson("testdata/cluster2", &c1)
	loadJson("testdata/gb2", &gb1)
	loadJson("testdata/g2", &g1)

	err = cluster_conf.GslbBasicConfCheck(&gb1)
	if err != nil {
		t.Errorf("GslbBasicConfCheck err %s", err)
	}
	t.Logf("%v %v %v\n", c1, gb1, g1)
	if err := bal.Reload(g1); err != nil {
		t.Errorf("reload error %s", err)
	}

	bal.SetGslbBasic(gb1)

	var backendConf cluster_conf.BackendBasic
	err = cluster_conf.BackendBasicCheck(&backendConf)
	if err != nil {
		t.Errorf("BackendBasicCheck err %s", err)
	}
	var ssTime = 30
	backendConf.SlowStartTime = &ssTime
	bal.SetSlowStart(backendConf)

	t.Logf("%+v\n", bal)
	t.Logf("%+v", bal.subClusters[0])
	t.Logf("%+v", bal.subClusters[1])
	t.Logf("%+v", bal.subClusters[2])
}
