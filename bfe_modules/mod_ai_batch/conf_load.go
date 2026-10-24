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
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	gcfg "gopkg.in/gcfg.v1"
)

// ConfModAiBatch is the startup-fixed configuration of mod_ai_batch. All
// tunable knobs live in the hot-reloadable mod_ai_batch.data instead: in
// this codebase .conf files are loaded once at Init (reloadHandlers only
// cover the .data file), and these batch knobs are expected to be tuned
// while the batch feature is being rolled out.
type ConfModAiBatch struct {
	Basic struct {
		ProductRulePath string // path for mod_ai_batch.data
	}

	// redis conf for BATCH_* state keys (same cluster as QUOTA_* and the
	// AIKeyAffinity bindings). Empty Bns disables batch state/affinity
	// (fail-open: passthrough continues, counters record the misses).
	Redis struct {
		Bns            string // bns name for redis proxy
		ConnectTimeout int    // connect timeout (ms)
		ReadTimeout    int    // read timeout (ms)
		WriteTimeout   int    // write timeout (ms)
		MaxIdle        int    // max idle connections in pool
		MaxActive      int    // max active connections in pool, 0 = unlimited
		Password       string // redis password, ignore if not set
	}

	Log struct {
		OpenDebug bool // whether open debug
	}
}

/* load config from config file */
func ConfLoad(filePath string, confRoot string) (*ConfModAiBatch, error) {
	var cfg ConfModAiBatch
	var err error

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModAiBatch) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		return fmt.Errorf("ProductRulePath is empty")
	}
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)
	if cfg.Redis.Bns != "" {
		return redis_client.CheckRedisConf(cfg.Redis.Bns)
	}
	return nil
}
