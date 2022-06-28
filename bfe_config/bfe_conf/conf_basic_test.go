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

import (
	gcfg "gopkg.in/gcfg.v1"
)

func confBasicLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	cfg.Server.SetDefaultConf()
	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	// check basic conf
	err = ConfBasicCheck(&cfg.Server, confRoot)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func Test_conf_basic_case1(t *testing.T) {
	config, err := confBasicLoad("./testdata/conf_basic/bfe_1.conf", "/home/bfe/conf")
	if err != nil {
		t.Errorf("err in BfeConfigLoad():%s", err.Error())
		return
	}

	if config.Server.HttpPort != 80 {
		t.Error("config.HttpPort should be 80")
	}

	if config.Server.MonitorEnabled && config.Server.MonitorPort != 8080 {
		t.Error("config.MonitorPort should be 8080")
	}

	if config.Server.MaxCpus != 5 {
		t.Error("config.MaxCpus should be 5")
	}

	if config.Server.ClientReadTimeout != 24 {
		t.Error("config.ClientReadTimeout should be 24")
	}

	if config.Server.HostRuleConf != "/home/bfe/conf/host_rule123.conf" {
		t.Error("config.HostRuleConf should be '/home/bfe/conf/host_rule123.conf'")
	}
}

func Test_conf_basic_case2(t *testing.T) {
	// service port is too small
	_, err := confBasicLoad("./testdata/conf_basic/bfe_2.conf", "")
	if err == nil {
		t.Error("BfeConfigLoad() should return nil")
	} else {
		println(err.Error())
	}
}

func Test_conf_basic_case3(t *testing.T) {
	// monitor port is too small
	_, err := confBasicLoad("./testdata/conf_basic/bfe_3.conf", "")
	if err == nil {
		t.Error("BfeConfigLoad() should return nil")
	}
}

func Test_conf_basic_case4(t *testing.T) {
	// maxCpus is zero
	_, err := confBasicLoad("./testdata/conf_basic/bfe_4.conf", "")
	if err != nil {
		t.Errorf("BfeConfigLoad() should return nil, but is:%s", err.Error())
	}
}

func Test_conf_basic_check(t *testing.T) {
	checks := []struct {
		conf *ConfigBasic
		err  string
	}{
		{&ConfigBasic{HttpPort: 80, HttpsPort: 443, MonitorPort: -1, MonitorEnabled: true},"MonitorPort[-1] should be in [1, 65535]"},
		{&ConfigBasic{HttpPort: 80, HttpsPort: 443, MonitorPort: 8080, MonitorEnabled: false, MaxCpus: -1}, "MaxCpus[-1] is too small"},
		{&ConfigBasic{HttpPort: 80, HttpsPort: 443, MonitorPort: 8080, MonitorEnabled: true, MaxCpus: 10, TlsHandshakeTimeout: 30,
			GracefulShutdownTimeout: 30}, "ClientReadTimeout[0] should > 0"},
		{&ConfigBasic{HttpPort: 80, HttpsPort: 443, MonitorPort: 8080, MonitorEnabled: true, MaxCpus: 10, TlsHandshakeTimeout: 30,
			GracefulShutdownTimeout: 30, ClientReadTimeout: 10, ClientWriteTimeout: 10, MonitorInterval: 33},
			"MonitorInterval[33] can not divide 60"},
	}

	for _, c := range checks {
		if e := basicConfCheck(c.conf).Error(); e != c.err {
			t.Errorf("error not matched, [%s] [%s]", e, c.err)
		}
	}

}

func Test_conf_data_check(t *testing.T) {
	var confBasic ConfigBasic
	dataFileConfCheck(&confBasic, "/workroot")
	if confBasic.HostRuleConf != "/workroot/server_data_conf/host_rule.data" {
		t.Errorf("hostRuleconf set error %s", confBasic.HostRuleConf)
	}
}
