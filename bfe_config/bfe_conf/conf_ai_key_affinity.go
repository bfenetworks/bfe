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

package bfe_conf

import (
	"fmt"

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
)

// ConfigAIKeyAffinity holds the Redis configuration used by AI key session
// affinity (session->key binding and key penalty state). The handle built
// from it is owned by bfe_server core (see initAIKeyAffinityRedis), so
// affinity availability depends only on this section, not on any module's
// Redis configuration. When Disabled is true (the default), no client is
// created and affinity silently does not apply (fail-open).
type ConfigAIKeyAffinity struct {
	// disable AI key session affinity or not
	Disabled bool

	// ServiceConf: bns name (or weighted bns list) of redis servers,
	// resolved via name_conf.data
	ServiceConf string

	// max idle connections in pool
	MaxIdle int

	// max active connections in pool,
	// when set 0, there is no connection num limit
	MaxActive int

	// config for connection (ms)
	ConnectTimeoutMs int
	ReadTimeoutMs    int
	WriteTimeoutMs   int

	// redis password, ignore if not set
	Password string
}

func (cfg *ConfigAIKeyAffinity) SetDefaultConf() {
	cfg.Disabled = true
	cfg.MaxIdle = 10
	cfg.MaxActive = 20
	cfg.ConnectTimeoutMs = 1000
	cfg.ReadTimeoutMs = 1000
	cfg.WriteTimeoutMs = 1000
}

func (cfg *ConfigAIKeyAffinity) Check(confRoot string) error {
	if cfg.Disabled {
		return nil
	}
	return ConfAIKeyAffinityCheck(cfg, confRoot)
}

func ConfAIKeyAffinityCheck(cfg *ConfigAIKeyAffinity, confRoot string) error {
	// check redis server conf
	if err := redis_client.CheckRedisConf(cfg.ServiceConf); err != nil {
		return err
	}

	// check connect timeout
	if cfg.ConnectTimeoutMs <= 0 {
		return fmt.Errorf("AIKeyAffinity.ConnectTimeoutMs[%d] should > 0", cfg.ConnectTimeoutMs)
	}

	// check read/write timeout
	if cfg.ReadTimeoutMs <= 0 || cfg.WriteTimeoutMs <= 0 {
		return fmt.Errorf("AIKeyAffinity.ReadTimeoutMs[%d]/WriteTimeoutMs[%d] should > 0",
			cfg.ReadTimeoutMs, cfg.WriteTimeoutMs)
	}

	// check MaxIdle
	if cfg.MaxIdle <= 0 {
		return fmt.Errorf("AIKeyAffinity.MaxIdle[%d] should > 0", cfg.MaxIdle)
	}

	// check MaxActive (0 means no connection num limit)
	if cfg.MaxActive < 0 {
		return fmt.Errorf("AIKeyAffinity.MaxActive[%d] should >= 0", cfg.MaxActive)
	}

	return nil
}
