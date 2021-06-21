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

package bfe_conf

import (
	"fmt"
	"strings"
)

type ConfigSessionCache struct {
	// disable tls session cache or not
	SessionCacheDisabled bool

	// address for redis servers
	Servers string

	// prefix for cache key
	KeyPrefix string

	// config for connection (ms)
	ConnectTimeout int
	ReadTimeout    int
	WriteTimeout   int

	// max idle connections in pool
	MaxIdle int

	// expire time for tls session state (s)
	SessionExpire int
}

func (cfg *ConfigSessionCache) SetDefaultConf() {
	cfg.SessionCacheDisabled = true
	cfg.KeyPrefix = "bfe"
	cfg.ConnectTimeout = 50
	cfg.WriteTimeout = 50
	cfg.MaxIdle = 20
	cfg.SessionExpire = 3600
}

func (cfg *ConfigSessionCache) Check(confRoot string) error {
	if cfg.SessionCacheDisabled {
		return nil
	}
	return ConfSessionCacheCheck(cfg, confRoot)
}

func ConfSessionCacheCheck(cfg *ConfigSessionCache, confRoot string) error {
	// check servers
	names := strings.Split(cfg.Servers, ",")
	if len(cfg.Servers) == 0 || len(names) < 1 {
		return fmt.Errorf("Servers[%s] invalid server names", cfg.Servers)
	}

	// check ReadTimeout
	if cfg.ReadTimeout <= 0 {
		return fmt.Errorf("ReadTimeout[%d] should > 0", cfg.ReadTimeout)
	}

	// check WriteTimeout
	if cfg.WriteTimeout <= 0 {
		return fmt.Errorf("WriteTimeout[%d] should > 0", cfg.WriteTimeout)
	}

	// check MaxIdle
	if cfg.MaxIdle <= 0 {
		return fmt.Errorf("MaxIdle[%d] should > 0", cfg.MaxIdle)
	}

	// check SessionExpire
	if cfg.SessionExpire <= 0 {
		return fmt.Errorf("SessionExpire[%d] should > 0", cfg.SessionExpire)
	}

	return nil
}
