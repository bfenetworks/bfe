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

package bfe_conf

import (
	"testing"
)

func TestBfeConfigLoadNormal(t *testing.T) {
	config, err := BfeConfigLoad("./testdata/conf_all/bfe.conf", "/home/bfe/conf")
	if err != nil {
		t.Errorf("BfeConfigLoad() return error[%v]", err)
		return
	}

	if config.Server.HttpPort != 80 {
		t.Errorf("config.HttpPort should be 80")
	}

	if config.Server.MonitorEnabled && config.Server.MonitorPort != 8080 {
		t.Errorf("config.MonitorPort should be 8080")
	}

	if config.Server.MaxCpus != 2 {
		t.Errorf("config.MaxCpus should be 2")
	}

	if config.Server.HostRuleConf != "/home/bfe/conf/route_conf/host_rule.data" {
		t.Errorf("err in HostRuleConf")
	}

	if config.Server.Modules == nil {
		t.Errorf("Modules should not be nil")
		return
	}

	if len(config.Server.Modules) != 2 {
		t.Errorf("len(Modules) should be 2")
	}

	if config.Server.ClusterTableConf != "/home/bfe/conf/cluster_conf/cluster_table.data" {
		t.Errorf("err in ClusterTableConf")
	}

	if config.Server.GslbConf != "/home/bfe/conf/cluster_conf/gslb.data" {
		t.Errorf("err in GslbConf")
	}

	if config.Server.ClusterConf != "/home/bfe/conf/cluster_conf/cluster_conf.data" {
		t.Errorf("err in ClusterConf")
	}

	if len(config.HttpsBasic.CipherSuites) != 3 {
		t.Errorf("CipherSuites length should be 3")
	}

	if len(config.HttpsBasic.CurvePreferences) != 1 && config.HttpsBasic.CurvePreferences[0] != "CurveP521" {
		t.Errorf("CurvePreferences should be CurveP521")
	}

	if config.SessionCache.SessionCacheDisabled {
		t.Errorf("SessionCache should not be disabled")
	}

	if config.SessionTicket.SessionTicketsDisabled {
		t.Errorf("SessionTicket should not be disabled")
	}
}

func TestBfeConfigLoadUsingDefault(t *testing.T) {
	config, err := BfeConfigLoad("./testdata/conf_all/bfe_default.conf", "/home/bfe/conf")
	if err != nil {
		t.Errorf("BfeConfigLoad() return error[%v]", err)
		return
	}

	if config.Server.HttpPort != 8080 {
		t.Errorf("config.HttpPort should be 8080")
	}

	if config.Server.MonitorEnabled && config.Server.MonitorPort != 8421 {
		t.Errorf("config.MonitorPort should be 8421")
	}

	if config.Server.MaxCpus != 0 {
		t.Errorf("config.MaxCpus should be %d, not %d", 0, config.Server.MaxCpus)
	}

	if config.Server.HostRuleConf != "/home/bfe/conf/server_data_conf/host_rule.data" {
		t.Errorf("err in HostRuleConf")
	}

	if len(config.Server.Modules) != 0 {
		t.Errorf("len(Modules) should be 0")
	}

	if config.Server.ClusterTableConf != "/home/bfe/conf/cluster_conf/cluster_table.data" {
		t.Errorf("err in ClusterTableConf")
	}

	if config.Server.GslbConf != "/home/bfe/conf/cluster_conf/gslb.data" {
		t.Errorf("err in GslbConf")
	}

	if config.Server.ClusterConf != "/home/bfe/conf/server_data_conf/cluster_conf.data" {
		t.Errorf("err in ClusterConf")
	}

	if len(config.HttpsBasic.CipherSuites) != 9 {
		t.Errorf("CipherSuites length should be 9")
	}

	if len(config.HttpsBasic.CurvePreferences) != 1 && config.HttpsBasic.CurvePreferences[0] != "CurveP256" {
		t.Errorf("CurvePreferences should be CurveP256")
	}

	if !config.SessionCache.SessionCacheDisabled {
		t.Errorf("SessionCache should be disabled")
	}

	if !config.SessionTicket.SessionTicketsDisabled {
		t.Errorf("SessionTicket should be disabled")
	}
}
