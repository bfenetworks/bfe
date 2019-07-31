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

package bfe_conf

import (
	"testing"
)

func TestBfeConfigLoad_case1(t *testing.T) {
	config, err := BfeConfigLoad("./testdata/conf_all/bfe_1.conf", "/home/bfe/conf")
	if err != nil {
		t.Errorf("BfeConfigLoad() return error[%s]", err.Error())
		return
	}

	if config.Server.HttpPort != 80 {
		t.Error("config.HttpPort should be 80")
	}

	if config.Server.MonitorPort != 8080 {
		t.Error("config.MonitorPort should be 8080")
	}

	if config.Server.MaxCpus != 2 {
		t.Error("config.MaxCpus should be 2")
	}

	if config.Server.HostRuleConf != "/home/bfe/conf/route_conf/host_rule.data" {
		t.Error("err in HostRuleConf")
	}

	if config.Server.Modules == nil {
		t.Error("Modules should not be nil")
		return
	}

	if len(config.Server.Modules) != 2 {
		t.Error("len(Modules) should be 2")
	}

	if config.Server.ClusterTableConf != "/home/bfe/conf/cluster_conf/cluster_table.data" {
		t.Error("err in ClusterTableConf")
	}

	if config.Server.GslbConf != "/home/bfe/conf/cluster_conf/gslb.data" {
		t.Error("err in GslbConf")
	}

	if config.Server.ClusterConf != "/home/bfe/conf/cluster_conf/cluster_conf.data" {
		t.Error("err in ClusterConf")
	}
}
