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

package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
)

// TokenRuleData holds the content of mod_ai_token_auth/token_rule.data.
type TokenRuleData struct {
	Version    string
	QuotaPlans map[string][]QuotaPlan
	Tokens     map[string]map[string]TokenFile
	Config     map[string][]TokenRule
}

// QuotaPlan is the JSON representation of a quota plan.
type QuotaPlan struct {
	Id          string
	Unlimited   bool
	PassNoQuota bool
	RedisKey    string
	CreateTime  int64
	ExpiredTime int64
	Quota       int64
	ResetMode   int
	Unit        string
}

// TokenFile is the JSON representation of a token file.
type TokenFile struct {
	Key            string                `json:"key"`
	Enabled        int                   `json:"enabled"`
	Status         int                   `json:"status"`
	Name           string                `json:"name"`
	UpdateTime     int64                 `json:"update_time"`
	ExpiredTime    int64                 `json:"expired_time"`
	UnlimitedQuota bool                  `json:"unlimited_quota"`
	Models         *string               `json:"allow_models"`
	BlockModels    *string               `json:"block_models"`
	Subnet         *string               `json:"subnet"`
	Tags           []bfe_basic.ApikeyTag `json:"tags"`
	QuotaPlans     []string              `json:"quota_plans"`
}

// TokenRule is the JSON representation of a token rule.
type TokenRule struct {
	Cond   string
	Action ActionFile
}

// ActionFile is the JSON representation of an action.
type ActionFile struct {
	Cmd string
}

// BFEConfigBuilder builds a temporary BFE configuration directory from a template.
type BFEConfigBuilder struct {
	// TemplateDir contains static BFE data files (bfe.conf, cluster_conf, mod_ai_route, etc.).
	TemplateDir string
	// TargetConfDir is the directory where the final BFE config will be written.
	TargetConfDir string
	// Backends maps cluster names to mock backends.
	Backends map[string]*MockBackend
	// AIConfs optionally injects AIConf into cluster_conf.data for specific clusters.
	AIConfs map[string]*cluster_conf.AIConf
	// TotalBodyBufferSize overrides the totalBodyBufferSize value in bfe.conf.
	// A value of 0 keeps the template value.
	TotalBodyBufferSize int64
	// RedisAddr is the address of the redis server used by mod_ai_token_auth.
	// If empty, mod_ai_token_auth.conf is not rewritten.
	RedisAddr string
	// TokenRuleData optionally generates mod_ai_token_auth/token_rule.data.
	TokenRuleData *TokenRuleData
}

// Build prepares the BFE configuration directory.
func (b *BFEConfigBuilder) Build() error {
	if err := os.MkdirAll(b.TargetConfDir, 0755); err != nil {
		return fmt.Errorf("create target conf dir failed: %w", err)
	}
	if err := copyDirContents(b.TemplateDir, b.TargetConfDir); err != nil {
		return fmt.Errorf("copy template dir failed: %w", err)
	}

	if err := b.normalizeAIRouteData(); err != nil {
		return fmt.Errorf("normalize ai_route.data failed: %w", err)
	}

	if err := b.generateClusterTable(); err != nil {
		return fmt.Errorf("generate cluster_table.data failed: %w", err)
	}

	if _, err := os.Stat(filepath.Join(b.TargetConfDir, "cluster_conf", "cluster_conf.data")); os.IsNotExist(err) {
		if err := b.generateClusterConfData(); err != nil {
			return fmt.Errorf("generate cluster_conf.data failed: %w", err)
		}
	}

	if err := RewriteBFEPorts(filepath.Join(b.TargetConfDir, "bfe.conf"), 0, 0, 0); err != nil {
		return fmt.Errorf("rewrite bfe ports failed: %w", err)
	}

	if b.TotalBodyBufferSize > 0 {
		if err := RewriteBFETotalBodyBufferSize(filepath.Join(b.TargetConfDir, "bfe.conf"), b.TotalBodyBufferSize); err != nil {
			return fmt.Errorf("rewrite totalBodyBufferSize failed: %w", err)
		}
	}

	if b.RedisAddr != "" {
		if err := b.setupRedisBns(); err != nil {
			return fmt.Errorf("setup redis bns failed: %w", err)
		}
	}

	if b.TokenRuleData != nil {
		if err := b.writeTokenRuleData(); err != nil {
			return fmt.Errorf("write token_rule.data failed: %w", err)
		}
	}

	return nil
}

const redisBnsName = "redis_bns"

func (b *BFEConfigBuilder) setupRedisBns() error {
	host, port, err := splitHostPort(b.RedisAddr)
	if err != nil {
		return fmt.Errorf("parse redis addr %s failed: %w", b.RedisAddr, err)
	}

	// rewrite mod_ai_token_auth.conf to use the fixed bns name
	if err := b.rewriteModAITokenAuthBns(); err != nil {
		return fmt.Errorf("rewrite mod_ai_token_auth bns failed: %w", err)
	}

	// generate name_conf.data mapping bns name to redis addr
	nameConf := map[string]interface{}{
		"Version": "1.0",
		"Config": map[string][]map[string]interface{}{
			redisBnsName: {
				{"Host": host, "Port": port, "Weight": 100},
			},
		},
	}
	path := filepath.Join(b.TargetConfDir, "server_data_conf", "name_conf.data")
	if err := writeJSONFile(path, nameConf); err != nil {
		return fmt.Errorf("write name_conf.data failed: %w", err)
	}

	// rewrite bfe.conf to load name_conf
	return b.rewriteBFEConfNameConf()
}

func (b *BFEConfigBuilder) rewriteModAITokenAuthBns() error {
	path := filepath.Join(b.TargetConfDir, "mod_ai_token_auth", "mod_ai_token_auth.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Bns") {
			lines[i] = "Bns = \"" + redisBnsName + "\""
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func (b *BFEConfigBuilder) rewriteBFEConfNameConf() error {
	path := filepath.Join(b.TargetConfDir, "bfe.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "NameConf") {
			lines[i] = "NameConf = server_data_conf/name_conf.data"
			found = true
		}
	}
	if !found {
		// insert after vipRuleConf if not found
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "vipRuleConf") {
				lines = append(lines[:i+1], append([]string{"NameConf = server_data_conf/name_conf.data"}, lines[i+1:]...)...)
				break
			}
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func splitHostPort(addr string) (string, int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid addr %s", addr)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %s", parts[1])
	}
	return parts[0], port, nil
}

func (b *BFEConfigBuilder) writeTokenRuleData() error {
	path := filepath.Join(b.TargetConfDir, "mod_ai_token_auth", "token_rule.data")
	return writeJSONFile(path, b.TokenRuleData)
}

func (b *BFEConfigBuilder) normalizeAIRouteData() error {
	path := filepath.Join(b.TargetConfDir, "mod_ai_route", "ai_route.data")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var aiRoute map[string]interface{}
	if err := json.Unmarshal(data, &aiRoute); err != nil {
		return err
	}
	normalizeAIRouteData(aiRoute)

	out, err := json.MarshalIndent(aiRoute, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func (b *BFEConfigBuilder) generateClusterTable() error {
	clusterTable := map[string]interface{}{
		"Version": "20260720150000",
		"Config":  map[string]interface{}{},
	}
	config := clusterTable["Config"].(map[string]interface{})

	for name, backend := range b.Backends {
		host, port := backend.HostPort()
		config[name] = map[string]interface{}{
			"sub_" + clusterSubName(name): []map[string]interface{}{
				{
					"Name":   name + "-backend-0",
					"Addr":   host,
					"Port":   port,
					"Weight": 100,
				},
			},
		}
	}

	data, err := json.MarshalIndent(clusterTable, "", "    ")
	if err != nil {
		return err
	}

	path := filepath.Join(b.TargetConfDir, "cluster_conf", "cluster_table.data")
	return os.WriteFile(path, data, 0644)
}

func (b *BFEConfigBuilder) generateClusterConfData() error {
	clusterConf := map[string]interface{}{
		"Version": "20260720150000",
		"Config":  map[string]interface{}{},
	}
	config := clusterConf["Config"].(map[string]interface{})

	for name := range b.Backends {
		conf := clusterBasicConf()
		if aiConf, ok := b.AIConfs[name]; ok && aiConf != nil {
			aiConfMap, err := aiConfToMap(aiConf)
			if err != nil {
				return fmt.Errorf("marshal AIConf for cluster %s failed: %w", name, err)
			}
			conf["AIConf"] = aiConfMap
		}
		config[name] = conf
	}

	path := filepath.Join(b.TargetConfDir, "cluster_conf", "cluster_conf.data")
	return writeJSONFile(path, clusterConf)
}

func aiConfToMap(aiConf *cluster_conf.AIConf) (map[string]interface{}, error) {
	bytes, err := json.Marshal(aiConf)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func clusterBasicConf() map[string]interface{} {
	return map[string]interface{}{
		"BackendConf": map[string]interface{}{
			"TimeoutConnSrv":        2000,
			"TimeoutResponseHeader": 50000,
			"MaxIdleConnsPerHost":   0,
			"RetryLevel":            0,
		},
		"CheckConf": map[string]interface{}{
			"Schem":         "http",
			"Uri":           "/healthcheck",
			"Host":          "example.org",
			"StatusCode":    200,
			"FailNum":       10,
			"CheckInterval": 1000,
		},
		"GslbBasic": map[string]interface{}{
			"CrossRetry": 0,
			"RetryMax":   2,
			"HashConf": map[string]interface{}{
				"HashStrategy":  0,
				"HashHeader":    "Cookie:UID",
				"SessionSticky": false,
			},
		},
		"ClusterBasic": map[string]interface{}{
			"TimeoutReadClient":      30000,
			"TimeoutWriteClient":     60000,
			"TimeoutReadClientAgain": 30000,
			"ReqWriteBufferSize":     512,
			"ReqFlushInterval":       0,
			"ResFlushInterval":       -1,
			"CancelOnClientClose":    false,
			"DisableHealthCheck":     true,
		},
	}
}

// RewriteBFEPorts rewrites the httpPort, httpsPort and monitorPort lines in bfe.conf
// and ensures HTTP/HTTPS/monitor listeners are bound to the loopback interface only.
func RewriteBFEPorts(path string, httpPort, httpsPort, monitorPort int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	hasHTTPAddr := false
	hasHTTPSAddr := false
	hasMonitorAddr := false
	serverSectionIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[server]" {
			serverSectionIdx = i
		}
		if strings.HasPrefix(trimmed, "httpAddr") {
			hasHTTPAddr = true
		}
		if strings.HasPrefix(trimmed, "httpsAddr") {
			hasHTTPSAddr = true
		}
		if strings.HasPrefix(trimmed, "monitorAddr") {
			hasMonitorAddr = true
		}
	}
	if serverSectionIdx >= 0 {
		insertAfter := serverSectionIdx
		if !hasHTTPAddr {
			lines = append(lines[:insertAfter+1], append([]string{`httpAddr = "127.0.0.1"`}, lines[insertAfter+1:]...)...)
			insertAfter++
		}
		if !hasHTTPSAddr {
			lines = append(lines[:insertAfter+1], append([]string{`httpsAddr = "127.0.0.1"`}, lines[insertAfter+1:]...)...)
			insertAfter++
		}
		if !hasMonitorAddr {
			lines = append(lines[:insertAfter+1], append([]string{`monitorAddr = "127.0.0.1"`}, lines[insertAfter+1:]...)...)
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if httpPort > 0 && strings.HasPrefix(trimmed, "httpPort") {
			lines[i] = "httpPort = " + strconv.Itoa(httpPort)
		}
		if httpsPort > 0 && strings.HasPrefix(trimmed, "httpsPort") {
			lines[i] = "httpsPort = " + strconv.Itoa(httpsPort)
		}
		if monitorPort >= 0 && strings.HasPrefix(trimmed, "monitorPort") {
			lines[i] = "monitorPort = " + strconv.Itoa(monitorPort)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// RewriteBFETotalBodyBufferSize rewrites the totalBodyBufferSize line in bfe.conf.
func RewriteBFETotalBodyBufferSize(path string, size int64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "totalBodyBufferSize") {
			lines[i] = "totalBodyBufferSize = " + strconv.FormatInt(size, 10)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("totalBodyBufferSize not found in %s", path)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func normalizeAIRouteData(data map[string]interface{}) {
	if data == nil {
		return
	}
	rr, ok := data["route_rules"].(map[string]interface{})
	if !ok {
		return
	}
	for _, table := range rr {
		t, ok := table.(map[string]interface{})
		if !ok {
			continue
		}
		rules, ok := t["rules"].([]interface{})
		if !ok {
			continue
		}
		for _, rule := range rules {
			r, ok := rule.(map[string]interface{})
			if !ok {
				continue
			}
			if r["fallbacks"] == nil {
				r["fallbacks"] = []interface{}{}
			}
		}
	}
}

func writeJSONFile(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
}
