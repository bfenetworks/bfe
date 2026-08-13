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
)

// BFEConfigBuilder builds a temporary BFE configuration directory from a template.
type BFEConfigBuilder struct {
	// TemplateDir contains static BFE data files (bfe.conf, cluster_conf, mod_ai_route, etc.).
	TemplateDir string
	// TargetConfDir is the directory where the final BFE config will be written.
	TargetConfDir string
	// Backends maps cluster names to mock backends.
	Backends map[string]*MockBackend
	// TotalBodyBufferSize overrides the totalBodyBufferSize value in bfe.conf.
	// A value of 0 keeps the template value.
	TotalBodyBufferSize int64
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

	return nil
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
		config[name] = clusterBasicConf()
	}

	path := filepath.Join(b.TargetConfDir, "cluster_conf", "cluster_conf.data")
	return writeJSONFile(path, clusterConf)
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
