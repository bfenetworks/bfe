// Copyright (c) 2025 The BFE Authors.
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

package mod_ai_token_auth

import (
	"fmt"

	"github.com/baidu/go-lib/log"
	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	gcfg "gopkg.in/gcfg.v1"
)

type ConfModAITokenAuth struct {
	Basic struct {
		ProductRulePath string
	}

	// redis conf
	Redis struct {
		Bns            string // bns name for redis proxy
		ConnectTimeout int    // connect timeout (ms)
		ReadTimeout    int    // read timeout (ms)
		WriteTimeout   int    // write timeout(ms)

		// max idle connections in pool
		MaxIdle int

		// redis password，ignore if not set
		Password string

		// max active connections in pool,
		// when set 0, there is no connection num limit
		MaxActive int
	}

	Log struct {
		OpenDebug bool
	}
}

func (cfg *ConfModAITokenAuth) Check(confRoot string) error {
	if cfg.Basic.ProductRulePath == "" {
		log.Logger.Warn("ModAITokenAuth.ProductRulePath not set, use default value")
		cfg.Basic.ProductRulePath = "mod_ai_toekn_auth/token_rule.data"
	}

	cfg.Basic.ProductRulePath = bfe_util.ConfPathProc(cfg.Basic.ProductRulePath, confRoot)

	// check redis server conf
	if err := redis_client.CheckRedisConf(cfg.Redis.Bns); err != nil {
		return err
	}

	// check connectTimeOut
	if cfg.Redis.ConnectTimeout <= 0 {
		return fmt.Errorf("Redis.ConnectTimeout must > 0")
	}

	// check Read/Write Timeout
	if cfg.Redis.ReadTimeout <= 0 || cfg.Redis.WriteTimeout <= 0 {
		return fmt.Errorf("Redis.ReadTimeout/WriteTimeout must > 0")
	}

	return nil
}

func ConfLoad(filePath string, confRoot string) (*ConfModAITokenAuth, error) {
	var cfg ConfModAITokenAuth
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
