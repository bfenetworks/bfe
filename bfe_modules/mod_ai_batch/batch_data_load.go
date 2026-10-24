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

package mod_ai_batch

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"

	"github.com/bfenetworks/go-lib/log"
)

// OwnerCheckMiss policies for downloads whose BATCH_FILE binding is missing
// (expired TTL, Redis write failure at upload, etc.).
const (
	OwnerCheckMissAllowLog = "allow_log" // pass through + counter (default)
	OwnerCheckMissDeny     = "deny"      // reject with 404
)

// BatchDataConf is the hot-reloadable configuration (mod_ai_batch.data).
// Effective per-request file limits are min(bound rate-limit policies'
// batch_limits, these global hard ceilings); see mod_ai_rate_limit.
type BatchDataConf struct {
	Version string `json:"Version"`

	// MaxFileBytes/MaxFileLines are the global hard ceilings, enforced on
	// both upload and download. 0 or negative disables the ceiling (not
	// recommended for production).
	MaxFileBytes int64 `json:"MaxFileBytes"`
	MaxFileLines int64 `json:"MaxFileLines"`

	// Reserve estimation defaults used when the price row carries no
	// limits.max_input_tokens/max_output_tokens (see mod_ai_token_auth).
	ReserveInputTokens  int64 `json:"ReserveInputTokens"`
	ReserveOutputTokens int64 `json:"ReserveOutputTokens"`
	// ReservePerLineMicros is the RMB reserve per input line in 1e-6 yuan
	// (micros). The model-specific usage is priced exactly at settle time
	// from the result file; this per-line reference only sizes the reserve.
	ReservePerLineMicros int64 `json:"ReservePerLineMicros"`

	// MaxLineParseBytes bounds the per-line parse buffer when scanning a
	// batch result file for usage (guards against abnormal lines).
	MaxLineParseBytes int64 `json:"MaxLineParseBytes"`

	// OwnerCheckMissPolicy: allow_log (default) | deny.
	OwnerCheckMissPolicy string `json:"OwnerCheckMissPolicy"`
}

func defaultBatchDataConf() *BatchDataConf {
	return &BatchDataConf{
		Version:              "1.0",
		MaxFileBytes:         1 << 30, // 1GB
		MaxFileLines:         100000,
		ReserveInputTokens:   8192,
		ReserveOutputTokens:  4096,
		ReservePerLineMicros: 200000, // 0.002 yuan per line
		MaxLineParseBytes:    1 << 20,
		OwnerCheckMissPolicy: OwnerCheckMissAllowLog,
	}
}

func (c *BatchDataConf) check() error {
	if c.MaxLineParseBytes <= 0 {
		return fmt.Errorf("MaxLineParseBytes must be > 0")
	}
	switch c.OwnerCheckMissPolicy {
	case "", OwnerCheckMissAllowLog, OwnerCheckMissDeny:
	default:
		return fmt.Errorf("OwnerCheckMissPolicy must be %q or %q", OwnerCheckMissAllowLog, OwnerCheckMissDeny)
	}
	if c.OwnerCheckMissPolicy == "" {
		c.OwnerCheckMissPolicy = OwnerCheckMissAllowLog
	}
	return nil
}

// loadBatchData (re)loads mod_ai_batch.data; it is registered in
// reloadHandlers so conf-agent's standard prober/file_store/trigger flow
// picks changes up without a restart.
func (m *ModuleAiBatch) loadBatchData(query url.Values) (string, error) {
	path := m.conf.Basic.ProductRulePath

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s err: %v", path, err)
	}

	cfg := defaultBatchDataConf()
	if err := json.Unmarshal(content, cfg); err != nil {
		return "", fmt.Errorf("decode %s err: %v", path, err)
	}
	if err := cfg.check(); err != nil {
		return "", fmt.Errorf("check %s err: %v", path, err)
	}

	m.dataLock.Lock()
	m.data = cfg
	m.dataLock.Unlock()

	if openDebug {
		log.Logger.Debug("mod_ai_batch: loadBatchData ok, version=%s", cfg.Version)
	}
	return fmt.Sprintf("%s=%s", path, cfg.Version), nil
}
