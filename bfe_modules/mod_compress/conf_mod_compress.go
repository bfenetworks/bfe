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

package mod_compress

import (
	"github.com/baidu/go-lib/log"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModCompress struct {
	Basic struct {
		ProductRulePath string
	}

	Log struct {
		OpenDebug bool
	}
}

func (cfg *ConfModCompress) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ModCompress.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = "mod_compress/compress_rule.data"
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	return nil
}

func ConfLoad(filePath string, confRoot string) (*ConfModCompress, error) {
	var cfg ConfModCompress
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
