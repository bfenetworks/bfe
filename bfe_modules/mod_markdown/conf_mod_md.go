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

package mod_markdown

import (
	"github.com/baidu/go-lib/log"
	"gopkg.in/gcfg.v1"
)
import (
	"github.com/bfenetworks/bfe/bfe_util"
)

const DefaultRulePath = "mod_markdown/markdown_rule.data"

type ConfModMarkdown struct {
	Basic struct {
		// md product rule data path
		ProductRulePath string
	}
	Log struct {
		// open debug log
		OpenDebug bool
	}
}

func (cfg *ConfModMarkdown) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ConfModMarkdown.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = DefaultRulePath
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	return nil
}

func ConfLoad(filePath string, confRoot string) (*ConfModMarkdown, error) {
	var cfg ConfModMarkdown
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
