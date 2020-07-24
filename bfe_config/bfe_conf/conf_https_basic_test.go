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

func confHttpsBasicLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	// check basic conf
	err = cfg.HttpsBasic.Check(confRoot)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func TestConfHttpsBasicLoad(t *testing.T) {
	conf, err := confHttpsBasicLoad("testdata/conf_https_basic/bfe_1.conf", "./")
	if err != nil {
		t.Errorf("load config err: %s", err)
		return
	}
	httpsConf := conf.HttpsBasic

	// check cert conf
	certConfExpect := "tls_conf/server_cert_conf.data"
	if httpsConf.ServerCertConf != certConfExpect {
		t.Errorf("wrong server cert conf (expect %s, actual %s)",
			certConfExpect, httpsConf.ServerCertConf)
	}

	// check cipher suite
	if len(httpsConf.CipherSuites) != 9 {
		t.Errorf("wrong number of ciphersuites (expect 9, actual %d)",
			len(httpsConf.CipherSuites))
	}

	// check curve Preferences
	if len(httpsConf.CurvePreferences) != 1 {
		t.Errorf("wrong number of curvePreferences (expect 1, actual %d)",
			len(httpsConf.CurvePreferences))
	}
}

func TestConfHttpsBasicLoad2(t *testing.T) {
	confFiles := []string{
		"testdata/conf_https_basic/bfe_2.conf",
		"testdata/conf_https_basic/bfe_3.conf",
	}

	for _, file := range confFiles {
		_, err := confHttpsBasicLoad(file, "./")
		if err == nil {
			t.Errorf("should found err while loading config %s", file)
		}
	}
}
