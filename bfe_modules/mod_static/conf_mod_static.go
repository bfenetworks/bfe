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

package mod_static

import (
	"github.com/baidu/go-lib/log"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModStatic struct {
	Basic struct {
		DataPath       string
		MimeTypePath   string
		EnableCompress bool
	}

	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModStatic, error) {
	var err error
	var cfg ConfModStatic

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return nil, err
	}

	err = cfg.Check(confRoot)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *ConfModStatic) Check(confRoot string) error {
	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModStatic.DataPath not set, use default value")
		cfg.Basic.DataPath = "mod_static/static_rule.data"
	}
	if cfg.Basic.MimeTypePath == "" {
		log.Logger.Warn("ModStatic.MimeTypePath not set, use default value")
		cfg.Basic.MimeTypePath = "mod_static/mime_type.data"
	}

	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)
	cfg.Basic.MimeTypePath = bfe_util.ConfPathProc(cfg.Basic.MimeTypePath, confRoot)

	return nil
}
