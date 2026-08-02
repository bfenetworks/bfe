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

package mod_tcp_keepalive

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepareTempDataFile(t *testing.T, content string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_tcp_keepalive_data_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "tcp_keepalive.data")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return path
}

func TestFormatIPValid(t *testing.T) {
	cases := []struct {
		in     string
		expect string
	}{
		{"10.1.1.1", "10.1.1.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"2001:0db8:02de:0000:0000:0000:0000:0e13", "2001:db8:2de::e13"},
	}

	for _, c := range cases {
		got, err := formatIP(c.in)
		if err != nil {
			t.Errorf("formatIP(%q) error: %v", c.in, err)
			continue
		}
		if got != c.expect {
			t.Errorf("formatIP(%q) = %q, want %q", c.in, got, c.expect)
		}
	}
}

func TestFormatIPInvalid(t *testing.T) {
	cases := []string{
		"10.1",
		"2001::25de::cade",
		"not-an-ip",
		"",
	}

	for _, c := range cases {
		if _, err := formatIP(c); err == nil {
			t.Errorf("formatIP(%q) should return error", c)
		}
	}
}

func TestConvertConfSuccess(t *testing.T) {
	conf := ProductRuleConf{
		Version: "v1",
		Config: map[string]ProductRulesFile{
			"product1": {
				{
					VipConf:        []string{"10.1.1.1", "10.1.1.2"},
					KeepAliveParam: KeepAliveParam{KeepIdle: 70},
				},
			},
		},
	}

	data, err := ConvertConf(conf)
	if err != nil {
		t.Fatalf("ConvertConf() error: %v", err)
	}

	if data.Version != "v1" {
		t.Errorf("Version = %q, want %q", data.Version, "v1")
	}

	rules, ok := data.Config["product1"]
	if !ok {
		t.Fatalf("product1 not found")
	}
	if len(rules) != 2 {
		t.Errorf("len(rules) = %d, want 2", len(rules))
	}

	if rules["10.1.1.1"].KeepIdle != 70 {
		t.Errorf("KeepIdle = %d, want 70", rules["10.1.1.1"].KeepIdle)
	}
}

func TestConvertConfDuplicateIP(t *testing.T) {
	conf := ProductRuleConf{
		Version: "v1",
		Config: map[string]ProductRulesFile{
			"product1": {
				{VipConf: []string{"10.1.1.1"}, KeepAliveParam: KeepAliveParam{KeepIdle: 10}},
				{VipConf: []string{"10.1.1.1"}, KeepAliveParam: KeepAliveParam{KeepIdle: 20}},
			},
		},
	}

	if _, err := ConvertConf(conf); err == nil {
		t.Error("ConvertConf() should fail for duplicated ip")
	} else if !strings.Contains(err.Error(), "duplicated ip") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertConfInvalidIP(t *testing.T) {
	conf := ProductRuleConf{
		Version: "v1",
		Config: map[string]ProductRulesFile{
			"product1": {
				{VipConf: []string{"10.1"}, KeepAliveParam: KeepAliveParam{KeepIdle: 10}},
			},
		},
	}

	if _, err := ConvertConf(conf); err == nil {
		t.Error("ConvertConf() should fail for invalid ip")
	}
}

func TestRulesCheckSuccess(t *testing.T) {
	rules := KeepAliveRules{
		"10.1.1.1":  {KeepIdle: 10, KeepIntvl: 5, KeepCnt: 3},
		"127.0.0.1": {KeepIdle: 0, KeepIntvl: 0, KeepCnt: 0},
	}

	if err := RulesCheck(rules); err != nil {
		t.Errorf("RulesCheck() error: %v", err)
	}
}

func TestRulesCheckInvalidIP(t *testing.T) {
	rules := KeepAliveRules{
		"not-an-ip": {},
	}

	if err := RulesCheck(rules); err == nil {
		t.Error("RulesCheck() should fail for invalid ip")
	} else if !strings.Contains(err.Error(), "invalid ip") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRulesCheckNegativeParam(t *testing.T) {
	cases := []KeepAliveParam{
		{KeepIdle: -1},
		{KeepIntvl: -1},
		{KeepCnt: -1},
	}

	for i, p := range cases {
		rules := KeepAliveRules{
			"10.1.1.1": p,
		}
		if err := RulesCheck(rules); err == nil {
			t.Errorf("case %d: RulesCheck() should fail for negative param", i)
		}
	}
}

func TestProductRulesCheckEmptyProduct(t *testing.T) {
	conf := ProductRules{
		"": {"10.1.1.1": {}},
	}

	if err := ProductRulesCheck(conf); err == nil {
		t.Error("ProductRulesCheck() should fail for empty product name")
	}
}

func TestProductRulesCheckNilRules(t *testing.T) {
	conf := ProductRules{
		"product1": nil,
	}

	if err := ProductRulesCheck(conf); err == nil {
		t.Error("ProductRulesCheck() should fail for nil rules")
	}
}

func TestProductRulesCheckNestedError(t *testing.T) {
	conf := ProductRules{
		"product1": {"not-an-ip": {}},
	}

	if err := ProductRulesCheck(conf); err == nil {
		t.Error("ProductRulesCheck() should fail for invalid nested rule")
	} else if !strings.Contains(err.Error(), "product1") {
		t.Errorf("error should contain product name: %v", err)
	}
}

func TestProductRuleDataCheckMissingVersion(t *testing.T) {
	conf := ProductRuleData{Config: ProductRules{"product1": {"10.1.1.1": {}}}}

	if err := ProductRuleDataCheck(conf); err == nil {
		t.Error("ProductRuleDataCheck() should fail when Version is missing")
	}
}

func TestProductRuleDataCheckMissingConfig(t *testing.T) {
	conf := ProductRuleData{Version: "v1"}

	if err := ProductRuleDataCheck(conf); err == nil {
		t.Error("ProductRuleDataCheck() should fail when Config is missing")
	}
}

func TestKeepAliveDataLoadFileNotExist(t *testing.T) {
	if _, err := KeepAliveDataLoad("./testdata/not_exist.data"); err == nil {
		t.Error("KeepAliveDataLoad() should fail when file does not exist")
	}
}

func TestKeepAliveDataLoadInvalidJSON(t *testing.T) {
	path := prepareTempDataFile(t, "not json")

	if _, err := KeepAliveDataLoad(path); err == nil {
		t.Error("KeepAliveDataLoad() should fail for invalid JSON")
	}
}

func TestKeepAliveDataLoadInvalidIP(t *testing.T) {
	content := `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1"],
                "KeepAliveParam": {"KeepIdle": 10}
            }
        ]
    },
    "Version": "v1"
}`
	path := prepareTempDataFile(t, content)

	if _, err := KeepAliveDataLoad(path); err == nil {
		t.Error("KeepAliveDataLoad() should fail for invalid ip")
	}
}

func TestKeepAliveDataLoadDuplicateIP(t *testing.T) {
	content := `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1"],
                "KeepAliveParam": {"KeepIdle": 10}
            },
            {
                "VipConf": ["10.01.1.1"],
                "KeepAliveParam": {"KeepIdle": 20}
            }
        ]
    },
    "Version": "v1"
}`
	path := prepareTempDataFile(t, content)

	if _, err := KeepAliveDataLoad(path); err == nil {
		t.Error("KeepAliveDataLoad() should fail for duplicated ip")
	}
}

func TestKeepAliveDataLoadMissingVersion(t *testing.T) {
	content := `{
    "Config": {
        "product1": [{"VipConf": ["10.1.1.1"], "KeepAliveParam": {}}]
    }
}`
	path := prepareTempDataFile(t, content)

	if _, err := KeepAliveDataLoad(path); err == nil {
		t.Error("KeepAliveDataLoad() should fail when Version is missing")
	}
}

func TestKeepAliveDataLoadNegativeParam(t *testing.T) {
	content := `{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1"],
                "KeepAliveParam": {"KeepIdle": -1}
            }
        ]
    },
    "Version": "v1"
}`
	path := prepareTempDataFile(t, content)

	if _, err := KeepAliveDataLoad(path); err == nil {
		t.Error("KeepAliveDataLoad() should fail for negative keepalive param")
	}
}

func TestConfModTcpKeepAliveCheckDefaultPath(t *testing.T) {
	cfg := &ConfModTcpKeepAlive{}
	if err := ConfModTcpKeepAliveCheck(cfg, "./conf"); err != nil {
		t.Fatalf("ConfModTcpKeepAliveCheck() error: %v", err)
	}

	if !strings.Contains(cfg.Basic.DataPath, "mod_tcp_keepalive/tcp_keepalive.data") {
		t.Errorf("unexpected DataPath: %s", cfg.Basic.DataPath)
	}
}

func TestConfModTcpKeepAliveCheckRelativePath(t *testing.T) {
	cfg := &ConfModTcpKeepAlive{}
	cfg.Basic.DataPath = "mydata/tcp_keepalive.data"

	if err := ConfModTcpKeepAliveCheck(cfg, "./conf"); err != nil {
		t.Fatalf("ConfModTcpKeepAliveCheck() error: %v", err)
	}

	if !strings.Contains(cfg.Basic.DataPath, "mydata/tcp_keepalive.data") {
		t.Errorf("unexpected DataPath: %s", cfg.Basic.DataPath)
	}
}

func TestConfLoadFileNotExist(t *testing.T) {
	if _, err := ConfLoad("./testdata/not_exist.conf", ""); err == nil {
		t.Error("ConfLoad() should fail when config file does not exist")
	}
}

func TestConfLoadInvalidSyntax(t *testing.T) {
	path := prepareTempDataFile(t, "[basic\nDataPath = value")

	if _, err := ConfLoad(path, ""); err == nil {
		t.Error("ConfLoad() should fail for invalid config syntax")
	}
}

func TestConfLoadDefaultDataPath(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_tcp_keepalive_conf_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	confPath := filepath.Join(dir, "mod_tcp_keepalive.conf")
	content := "[log]\nOpenDebug = true\n"
	if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := ConfLoad(confPath, dir)
	if err != nil {
		t.Fatalf("ConfLoad() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad() returns nil config")
	}

	if !strings.Contains(cfg.Basic.DataPath, "mod_tcp_keepalive/tcp_keepalive.data") {
		t.Errorf("unexpected DataPath: %s", cfg.Basic.DataPath)
	}

	if !cfg.Log.OpenDebug {
		t.Error("Log.OpenDebug should be true")
	}
}
