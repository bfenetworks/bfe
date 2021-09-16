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

func confSessionCacheLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	// check basic conf
	err = cfg.SessionCache.Check(confRoot)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func TestConfSessionCacheLoad(t *testing.T) {
	conf, err := confSessionCacheLoad("testdata/conf_session_cache/bfe_1.conf", "./")
	if err != nil {
		t.Errorf("load config err: %s", err)
		return
	}
	scacheConf := conf.SessionCache

	if scacheConf.SessionCacheDisabled {
		t.Errorf("wrong SessionCacheDisabled, expect false")
	}

	serverExpect := "10.1.2.3:9000"
	if scacheConf.Servers != serverExpect {
		t.Errorf("wrong servers, expect %s, actual %s", serverExpect, scacheConf.Servers)
	}

	if scacheConf.ConnectTimeout != 10 {
		t.Errorf("wrong connect timeout")
	}

	if scacheConf.ReadTimeout != 10 {
		t.Errorf("wrong read timeout")
	}

	if scacheConf.WriteTimeout != 10 {
		t.Errorf("wrong write timeout")
	}

	if scacheConf.MaxIdle != 10 {
		t.Errorf("wrong max idle")
	}

	if scacheConf.SessionExpire != 600000 {
		t.Errorf("wrong sesson expire")
	}
}

func TestConfSessionCacheLoad2(t *testing.T) {
	confFile := "testdata/conf_session_cache/bfe_2.conf"
	_, err := confSessionCacheLoad(confFile, "./")
	if err == nil {
		t.Errorf("should found err while loading config %s", confFile)
	}
}
