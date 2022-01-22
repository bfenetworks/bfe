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

package mod_degrade

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
	"gopkg.in/gcfg.v1"
)

type ConfModDegrade struct {
	Basic struct {
		ProductRulePath string // path of product degrade rule data
	}
}

func (cfg *ConfModDegrade) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ModDegrade.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = "mod_degrade/degrade.data"
	}
	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)
	return nil
}

func ConfLoad(filePath string, confRoot string) (*ConfModDegrade, error) {
	var(
		cfg ConfModDegrade
		err error
	)

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_degrade
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}
