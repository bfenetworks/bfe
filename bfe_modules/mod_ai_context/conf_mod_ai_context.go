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

package mod_ai_context

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util"
	gcfg "gopkg.in/gcfg.v1"
)

// The module INI deliberately keeps only bootstrap-style static config (rule
// file path, log switch). All business tuning lives in the rule.data Defaults
// block so it can be hot reloaded.
type ConfModAiContext struct {
	Basic struct {
		ProductRulePath string // path for product rule
	}

	Log struct {
		OpenDebug bool // whether open debug
	}
}

/* load config from config file */
func ConfLoad(filePath string, confRoot string) (*ConfModAiContext, error) {
	var cfg ConfModAiContext
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModAiContext) Check(confRoot string) error {
	return ConfModAiContextCheck(cfg, confRoot)
}

func ConfModAiContextCheck(cfg *ConfModAiContext, confRoot string) error {
	// check ProductRulePath
	if cfg.Basic.ProductRulePath == "" {
		return fmt.Errorf("ConfModAiContextCheck ProductRulePath is nil")
	}

	// build conf path
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	return nil
}
