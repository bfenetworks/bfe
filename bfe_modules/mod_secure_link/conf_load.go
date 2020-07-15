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

package mod_secure_link

import (
	"github.com/baidu/go-lib/log"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModSecureLink struct {
	Basic struct {
		DataPath string // path of config data (mod_secure_link)
	}

	Log struct {
		OpenDebug bool
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModSecureLink, error) {
	cfg := &ConfModSecureLink{}
	err := gcfg.ReadFileInto(cfg, filePath)
	if err != nil {
		return nil, err
	}

	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModSecureLink.DataPath not set, use default value")
		cfg.Basic.DataPath = "mod_secure_link/secure_link.data"
	}
	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	return cfg, nil
}
