// Copyright (c) 2021 The BFE Authors.
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

package mod_tcp_keepalive

import (
	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
	gcfg "gopkg.in/gcfg.v1"
)

type ConfModTcpKeepAlive struct {
	Basic struct {
		DataPath string // path of product keepalive rule data
	}

	Log struct {
		OpenDebug bool // whether open debug
	}
}

func ConfLoad(filePath string, confRoot string) (*ConfModTcpKeepAlive, error) {
	var cfg ConfModTcpKeepAlive
	var err error

	err = gcfg.ReadFileInto(&cfg, filePath)
	if err != nil {
		return &cfg, err
	}

	// check conf of mod_tcp_keepalive
	err = cfg.Check(confRoot)
	if err != nil {
		return &cfg, err
	}

	return &cfg, nil
}

func (cfg *ConfModTcpKeepAlive) Check(confRoot string) error {
	return ConfModTcpKeepAliveCheck(cfg, confRoot)
}

func ConfModTcpKeepAliveCheck(cfg *ConfModTcpKeepAlive, confRoot string) error {
	if cfg.Basic.DataPath == "" {
		log.Logger.Warn("ModTcpKeepAlive.DataPath not set, use default value")
		cfg.Basic.DataPath = "mod_tcp_keepalive/tcp_keepalive.data"
	}
	cfg.Basic.DataPath = bfe_util.ConfPathProc(cfg.Basic.DataPath, confRoot)

	return nil
}
