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
	"github.com/baidu/go-lib/log"
	"gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/access_log"
)

const (
	DefaultRulePath = "mod_waf/waf_rule.data" // default product rule path
)

type ConfModWaf struct {
	Basic struct {
		ProductRulePath string // path of waf rule data
	}
	Log access_log.LogConfig
}

func (cfg *ConfModWaf) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ConfModWaf.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = DefaultRulePath
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	return cfg.Log.Check(confRoot)
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
