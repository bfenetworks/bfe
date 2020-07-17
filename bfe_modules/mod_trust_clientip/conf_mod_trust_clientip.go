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

package mod_trust_clientip

import (
	"github.com/baidu/go-lib/log"
	gcfg "gopkg.in/gcfg.v1"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

type ConfModTrustClientIP struct {
	Basic struct {
		DataPath string // path of config data (trust-ip)
	}

	Log struct {
		OpenDebug bool
	}
}

// ConfLoad loads config from config file.
func ConfLoad(filePath string, confRoot string) (*ConfModTrustClientIP, error) {
	var cfg ConfModTrustClientIP
	var err error

	// read config from file
	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_trust_clientip
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModTrustClientIP) Check(confRoot string) error {
	return ConfModTrustClientIPCheck(cfg, confRoot)
}

func ConfModTrustClientIPCheck(cfg *ConfModTrustClientIP, confRoot string) error {
	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModTrustClientIP.DataPath not set, use default value")
		cfg.Basic.DataPath = "mod_trust_clientip/trust_client_ip.data"
	}

	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)
	return nil
}
