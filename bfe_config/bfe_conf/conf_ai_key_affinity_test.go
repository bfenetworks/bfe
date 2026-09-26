// Copyright (c) 2026 The BFE Authors.
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

func confAIKeyAffinityLoad(filePath string, confRoot string) (BfeConfig, error) {
	var cfg BfeConfig
	var err error

	SetDefaultConf(&cfg)

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return cfg, err
	}

	// check ai key affinity conf
	err = cfg.AIKeyAffinity.Check(confRoot)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func TestConfAIKeyAffinitySetDefaultConf(t *testing.T) {
	var cfg ConfigAIKeyAffinity
	cfg.SetDefaultConf()

	if !cfg.Disabled {
		t.Errorf("wrong Disabled, expect true")
	}
	if cfg.MaxIdle != 10 {
		t.Errorf("wrong MaxIdle, expect 10, actual %d", cfg.MaxIdle)
	}
	if cfg.MaxActive != 20 {
		t.Errorf("wrong MaxActive, expect 20, actual %d", cfg.MaxActive)
	}
	if cfg.ConnectTimeoutMs != 1000 {
		t.Errorf("wrong ConnectTimeoutMs, expect 1000, actual %d", cfg.ConnectTimeoutMs)
	}
	if cfg.ReadTimeoutMs != 1000 {
		t.Errorf("wrong ReadTimeoutMs, expect 1000, actual %d", cfg.ReadTimeoutMs)
	}
	if cfg.WriteTimeoutMs != 1000 {
		t.Errorf("wrong WriteTimeoutMs, expect 1000, actual %d", cfg.WriteTimeoutMs)
	}
}

func TestConfAIKeyAffinityLoad(t *testing.T) {
	conf, err := confAIKeyAffinityLoad("testdata/conf_ai_key_affinity/bfe_1.conf", "./")
	if err != nil {
		t.Errorf("load config err: %s", err)
		return
	}
	affinityConf := conf.AIKeyAffinity

	if affinityConf.Disabled {
		t.Errorf("wrong Disabled, expect false")
	}

	serviceConfExpect := "redis_bns"
	if affinityConf.ServiceConf != serviceConfExpect {
		t.Errorf("wrong ServiceConf, expect %s, actual %s", serviceConfExpect, affinityConf.ServiceConf)
	}

	if affinityConf.ConnectTimeoutMs != 1000 {
		t.Errorf("wrong connect timeout")
	}

	if affinityConf.ReadTimeoutMs != 1000 {
		t.Errorf("wrong read timeout")
	}

	if affinityConf.WriteTimeoutMs != 1000 {
		t.Errorf("wrong write timeout")
	}

	if affinityConf.MaxIdle != 10 {
		t.Errorf("wrong max idle")
	}

	if affinityConf.MaxActive != 20 {
		t.Errorf("wrong max active")
	}

	if affinityConf.Password != "redis_pass" {
		t.Errorf("wrong password")
	}
}

func TestConfAIKeyAffinityLoad2(t *testing.T) {
	confFile := "testdata/conf_ai_key_affinity/bfe_2.conf"
	_, err := confAIKeyAffinityLoad(confFile, "./")
	if err == nil {
		t.Errorf("should found err while loading config %s", confFile)
	}
}

func TestConfAIKeyAffinityLoadDisabledByDefault(t *testing.T) {
	// without [AIKeyAffinity] section, affinity stays disabled (fail-open)
	conf, err := confAIKeyAffinityLoad("testdata/conf_ai_key_affinity/bfe_3.conf", "./")
	if err != nil {
		t.Errorf("load config err: %s", err)
		return
	}
	if !conf.AIKeyAffinity.Disabled {
		t.Errorf("wrong Disabled, expect true when section not configured")
	}
}

func TestConfAIKeyAffinityCheck(t *testing.T) {
	valid := ConfigAIKeyAffinity{
		Disabled:         false,
		ServiceConf:      "redis_bns",
		MaxIdle:          10,
		MaxActive:        20,
		ConnectTimeoutMs: 1000,
		ReadTimeoutMs:    1000,
		WriteTimeoutMs:   1000,
	}

	if err := valid.Check("./"); err != nil {
		t.Errorf("check valid conf err: %s", err)
	}

	// disabled conf skips validation
	disabled := ConfigAIKeyAffinity{Disabled: true}
	if err := disabled.Check("./"); err != nil {
		t.Errorf("check disabled conf err: %s", err)
	}

	cases := []struct {
		name   string
		mutate func(*ConfigAIKeyAffinity)
	}{
		{"empty ServiceConf", func(c *ConfigAIKeyAffinity) { c.ServiceConf = "" }},
		{"bad ServiceConf", func(c *ConfigAIKeyAffinity) { c.ServiceConf = "a,weight:x" }},
		{"zero ConnectTimeoutMs", func(c *ConfigAIKeyAffinity) { c.ConnectTimeoutMs = 0 }},
		{"zero ReadTimeoutMs", func(c *ConfigAIKeyAffinity) { c.ReadTimeoutMs = 0 }},
		{"zero WriteTimeoutMs", func(c *ConfigAIKeyAffinity) { c.WriteTimeoutMs = 0 }},
		{"zero MaxIdle", func(c *ConfigAIKeyAffinity) { c.MaxIdle = 0 }},
		{"negative MaxActive", func(c *ConfigAIKeyAffinity) { c.MaxActive = -1 }},
	}
	for _, tc := range cases {
		cfg := valid
		tc.mutate(&cfg)
		if err := cfg.Check("./"); err == nil {
			t.Errorf("%s: should found err", tc.name)
		}
	}

	// MaxActive = 0 means no connection num limit
	noLimit := valid
	noLimit.MaxActive = 0
	if err := noLimit.Check("./"); err != nil {
		t.Errorf("check MaxActive=0 conf err: %s", err)
	}
}
