// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"fmt"
)

import (
	"github.com/baidu/go-lib/log"
	"github.com/baidu/go-lib/log/log4go"
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

const (
	DefaultRulePath = "mod_waf/waf_rule.data" // default product rule path
)

type ConfModWaf struct {
	Basic struct {
		ProductRulePath string // path of waf rule data
	}
	Log struct {
		LogPrefix   string // log file prefix
		LogDir      string // log file dir
		RotateWhen  string // rotate time
		BackupCount int    // log file backup number
	}
}

func (cfg *ConfModWaf) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ConfModWaf.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = DefaultRulePath
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	if cfg.Log.LogPrefix == "" {
		return fmt.Errorf("ConfModWaf.LogPrefix is empty")
	}

	if cfg.Log.LogDir == "" {
		return fmt.Errorf("ConfModWaf.LogDir is empty")
	}
	cfg.Log.LogDir = bfe_util.ConfPathProc(cfg.Log.LogDir, confRoot)

	if !log4go.WhenIsValid(cfg.Log.RotateWhen) {
		return fmt.Errorf("ConfModWaf.RotateWhen invalid: %s", cfg.Log.RotateWhen)
	}

	if cfg.Log.BackupCount <= 0 {
		return fmt.Errorf("ConfModWaf.BackupCount should > 0: %d", cfg.Log.BackupCount)
	}

	return nil
}

func ConfLoad(filePath string, confRoot string) (*ConfModWaf, error) {
	var cfg ConfModWaf
	err := gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}
	err = cfg.Check(confRoot)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
