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

package mod_session_sticky

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
	gcfg "gopkg.in/gcfg.v1"
)

const (
	defaultCacheSize = 10000
)

type ConfModSessionSticky struct {
	Basic struct {
		DataPath  string // path of config data (session sticky)
		CacheSize int    // sticky cache size
	}

	Log struct {
		OpenDebug bool
	}
}

// ConfLoad loades config from config file
func ConfLoad(filePath string, confRoot string) (*ConfModSessionSticky, error) {
	var err error
	var cfg ConfModSessionSticky

	// read config from file
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

func (cfg *ConfModSessionSticky) Check(confRoot string) error {
	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModSessionSticky.DataPath not set, use default value")
		cfg.Basic.DataPath = "mod_session_sticky/session_sticky.data"
	}

	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	if cfg.Basic.CacheSize < defaultCacheSize {
		cfg.Basic.CacheSize = defaultCacheSize
	}
	return nil
}
