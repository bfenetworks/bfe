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

package mod_auth_jwt

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

import (
	"github.com/bfenetworks/go-lib/web-monitor/web_monitor"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_http"
	"github.com/bfenetworks/bfe/bfe_module"
)

const testKeyFile = `[
    {
        "k": "YmZland0Mg",
        "kty": "oct",
        "kid": "0001"
    },
    {
        "k": "YmZland0",
        "kty": "oct",
        "kid": "0002"
    }
]`

const validJWTToken = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiIsImtpZCI6IjAwMDEifQ." +
	"eyJuYW1lIjoiVW5pdHRlc3QiLCJzdWIiOiJVbml0dGVzdCIsImlzcyI6IkJGRSBHYXRld2F5In0." +
	"NZcVkR0hcJLFrmtFZGlrMUye5wFB2twCKDDRHwn4QQ4"

// prepareTempConf creates a temporary conf root that contains
// mod_auth_jwt/mod_auth_jwt.conf, mod_auth_jwt/auth_jwt_rule.data and
// mod_auth_jwt/key_file. The caller is responsible for cleaning up the
// returned directory.
//
// dataContent may contain the placeholder "{{KEY_FILE}}", which is replaced
// with the absolute path of the generated key file so that the rule loader can
// locate it regardless of the current working directory.
func prepareTempConf(t *testing.T, dataContent string) string {
	t.Helper()

	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	modDir := filepath.Join(dir, "mod_auth_jwt")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	keyPath := filepath.Join(modDir, "key_file")
	if err := ioutil.WriteFile(keyPath, []byte(testKeyFile), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	dataContent = strings.ReplaceAll(dataContent, "{{KEY_FILE}}", filepath.ToSlash(keyPath))

	if err := ioutil.WriteFile(filepath.Join(modDir, "auth_jwt_rule.data"), []byte(dataContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	confContent := `[basic]
DataPath = ./mod_auth_jwt/auth_jwt_rule.data

[log]
OpenDebug = true
`
	if err := ioutil.WriteFile(filepath.Join(modDir, "mod_auth_jwt.conf"), []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	return dir
}

func TestNewModuleAuthJWTAndName(t *testing.T) {
	m := NewModuleAuthJWT()
	if m == nil {
		t.Fatal("NewModuleAuthJWT() returns nil")
	}

	if m.Name() != ModAuthJWT {
		t.Errorf("Name() should be %s, got %s", ModAuthJWT, m.Name())
	}
}

func TestModuleAuthJWTInitSuccess(t *testing.T) {
	dataContent := `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "{{KEY_FILE}}",
                "Realm": "unittest"
            }
        ]
    },
    "Version": "unittest"
}`
	dir := prepareTempConf(t, dataContent)

	m := NewModuleAuthJWT()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestModuleAuthJWTInitConfNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	m := NewModuleAuthJWT()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when config file does not exist")
	}
}

func TestModuleAuthJWTInitDataLoadFail(t *testing.T) {
	dir := prepareTempConf(t, "invalid json data")

	m := NewModuleAuthJWT()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()

	if err := m.Init(cb, wh, dir); err == nil {
		t.Error("Init() should fail when rule data file is invalid")
	}
}

func TestModuleAuthJWTGetState(t *testing.T) {
	m := NewModuleAuthJWT()
	m.state.ReqAuthSuccess.Inc(1)

	b, err := m.getState(nil)
	if err != nil {
		t.Errorf("getState() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getState() returns empty response")
	}

	b, err = m.getStateDiff(nil)
	if err != nil {
		t.Errorf("getStateDiff() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("getStateDiff() returns empty response")
	}
}

func TestGetTokenAuthorizationTypeError(t *testing.T) {
	m := NewModuleAuthJWT()

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Header.Set("Authorization", "Basic dGVzdA==")

	_, err := m.getToken(req)
	if err == nil {
		t.Error("getToken() should fail for non-Bearer authorization type")
	}
	if !strings.Contains(err.Error(), "Authorization type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetTokenAuthorizationFormatError(t *testing.T) {
	m := NewModuleAuthJWT()

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Header.Set("Authorization", "BearerOnlyOnePart")

	_, err := m.getToken(req)
	if err == nil {
		t.Error("getToken() should fail for malformed Authorization header")
	}
	if !strings.Contains(err.Error(), "format error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthJWTHandlerCorrectWithBearerPrefix(t *testing.T) {
	dataContent := `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "{{KEY_FILE}}",
                "Realm": "unittest"
            }
        ]
    },
    "Version": "unittest"
}`
	dir := prepareTempConf(t, dataContent)

	m := NewModuleAuthJWT()
	cb := bfe_module.NewBfeCallbacks()
	wh := web_monitor.NewWebHandlers()
	if err := m.Init(cb, wh, dir); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	req := new(bfe_basic.Request)
	req.Session = new(bfe_basic.Session)
	req.Route.Product = "unittest"
	req.HttpRequest, _ = bfe_http.NewRequest("GET", "http://www.example.org", nil)
	req.HttpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", validJWTToken))

	ret, resp := m.authJWTHandler(req)
	if ret != bfe_module.BfeHandlerGoOn {
		t.Errorf("ret should be %d, not %d", bfe_module.BfeHandlerGoOn, ret)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func TestConfLoadDefaultDataPath(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	modDir := filepath.Join(dir, "mod_auth_jwt")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	confPath := filepath.Join(modDir, "mod_auth_jwt.conf")
	confContent := `[log]
OpenDebug = true
`
	if err := ioutil.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := ConfLoad(confPath, dir)
	if err != nil {
		t.Errorf("ConfLoad() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("ConfLoad() returns nil config")
	}
	if !strings.Contains(cfg.Basic.DataPath, "auth_jwt_rule.data") {
		t.Errorf("DataPath should contain auth_jwt_rule.data, got %s", cfg.Basic.DataPath)
	}
}

func TestConfLoadFileNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	confPath := filepath.Join(dir, "not_exist.conf")
	if _, err := ConfLoad(confPath, dir); err == nil {
		t.Error("ConfLoad() should fail when config file does not exist")
	}
}

func TestAuthJWTConfLoadVersionMissing(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "no_version.data")
	content := `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "./key_file",
                "Realm": "unittest"
            }
        ]
    }
}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := AuthJWTConfLoad(path); err == nil {
		t.Error("AuthJWTConfLoad() should fail when Version is missing")
	}
}

func TestAuthJWTConfLoadConfigMissing(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "no_config.data")
	content := `{
    "Version": "unittest"
}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := AuthJWTConfLoad(path); err == nil {
		t.Error("AuthJWTConfLoad() should fail when Config is missing")
	}
}

func TestAuthJWTConfLoadCondEmpty(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	keyPath := filepath.Join(dir, "key_file")
	if err := ioutil.WriteFile(keyPath, []byte(testKeyFile), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	path := filepath.Join(dir, "empty_cond.data")
	content := fmt.Sprintf(`{
    "Config": {
        "unittest": [
            {
                "Cond": "",
                "KeyFile": %q,
                "Realm": "unittest"
            }
        ]
    },
    "Version": "unittest"
}`, keyPath)
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := AuthJWTConfLoad(path); err == nil {
		t.Error("AuthJWTConfLoad() should fail when Cond is empty")
	}
}

func TestAuthJWTConfLoadKeyFileEmpty(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "empty_keyfile.data")
	content := `{
    "Config": {
        "unittest": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "",
                "Realm": "unittest"
            }
        ]
    },
    "Version": "unittest"
}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := AuthJWTConfLoad(path); err == nil {
		t.Error("AuthJWTConfLoad() should fail when KeyFile is empty")
	}
}

func TestAuthJWTConfLoadProductRuleNil(t *testing.T) {
	dir, err := ioutil.TempDir("", "mod_auth_jwt_test")
	if err != nil {
		t.Fatalf("TempDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "nil_rule.data")
	content := `{
    "Config": {
        "unittest": null
    },
    "Version": "unittest"
}`
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := AuthJWTConfLoad(path); err == nil {
		t.Error("AuthJWTConfLoad() should fail when product rule list is nil")
	}
}

func TestReadKeyFileNotExist(t *testing.T) {
	_, err := readKeyFile("/path/that/does/not/exist/key_file")
	if err == nil {
		t.Error("readKeyFile() should fail when file does not exist")
	}
}
