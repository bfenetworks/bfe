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

package mod_prison

import (
	"github.com/baidu/go-lib/log"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModPrison struct {
	Basic struct {
		ProductRulePath string // path for product rule
	}

	Log struct {
		OpenDebug bool // whether open debug
	}
}

// ConfLoad load config from file
func ConfLoad(filePath string, confRoot string) (*ConfModPrison, error) {
	var cfg ConfModPrison
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_prison
	err = ConfModPrisonCheck(&cfg, confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

// ConfModPrisonCheck check conf of mod_prison
func ConfModPrisonCheck(cfg *ConfModPrison, confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ModPrison.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = "mod_prison/prison.data"
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)
	return nil
}
