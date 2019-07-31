// Copyright (c) 2019 Baidu, Inc.
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

package mod_block

import (
        gcfg "gopkg.in/gcfg.v1"
	"github.com/baidu/go-lib/log"
)

import (
	"github.com/baidu/bfe/bfe_util"
)

type ConfModBlock struct {
	Basic struct {
		ProductRulePath string // path of product block rule data
		IPBlacklistPath string // path of ip blacklist data
	}

	Log struct {
		OpenDebug bool //  whether open debug
	}
}

// ConfLoad loades config from config file
func ConfLoad(filePath string, confRoot string) (*ConfModBlock, error) {
	var cfg ConfModBlock
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_block
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModBlock) Check(confRoot string) error {
	return ConfModBlockCheck(cfg, confRoot)
}

func ConfModBlockCheck(cfg *ConfModBlock, confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ModBlock.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = "mod_block/block_rules.data"
	}
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	if cfg.Basic.IPBlacklistPath == "" {
		log.Logger.Warn("ModBlock.IPBlacklistPath not set, use default value")
		cfg.Basic.IPBlacklistPath = "mod_block/ip_blacklist.data"
	}
	cfg.Basic.IPBlacklistPath = bfe_util.ConfPathProc(cfg.Basic.IPBlacklistPath, confRoot)

	return nil
}
