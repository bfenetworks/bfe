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

package bfe_server

import (
	"testing"

	"github.com/bfenetworks/bfe/bfe_config/bfe_conf"
)

func TestInitAIKeyAffinityRedisDisabled(t *testing.T) {
	srv := &BfeServer{}
	srv.Config.AIKeyAffinity = bfe_conf.ConfigAIKeyAffinity{
		Disabled: true,
	}

	srv.initAIKeyAffinityRedis()

	if srv.AIKeyAffinityRedis != nil {
		t.Fatal("expected nil redis client when AIKeyAffinity is disabled")
	}
}

func TestInitAIKeyAffinityRedisEnabled(t *testing.T) {
	srv := &BfeServer{}
	srv.Config.AIKeyAffinity = bfe_conf.ConfigAIKeyAffinity{
		Disabled:         false,
		ServiceConf:      "redis_bns",
		MaxIdle:          10,
		MaxActive:        20,
		ConnectTimeoutMs: 1000,
		ReadTimeoutMs:    1000,
		WriteTimeoutMs:   1000,
	}

	srv.initAIKeyAffinityRedis()

	if srv.AIKeyAffinityRedis == nil {
		t.Fatal("expected non-nil redis client when AIKeyAffinity is enabled")
	}
}

func TestInitAIKeyAffinityRedisUnresolvableNotBlocking(t *testing.T) {
	// An unresolvable bns name must not block startup: the client is created
	// anyway and connections are established lazily on first use.
	srv := &BfeServer{}
	srv.Config.AIKeyAffinity = bfe_conf.ConfigAIKeyAffinity{
		Disabled:         false,
		ServiceConf:      "unknown-redis-bns",
		MaxIdle:          10,
		MaxActive:        20,
		ConnectTimeoutMs: 1000,
		ReadTimeoutMs:    1000,
		WriteTimeoutMs:   1000,
	}

	srv.initAIKeyAffinityRedis()

	if srv.AIKeyAffinityRedis == nil {
		t.Fatal("expected non-nil redis client even when bns name is unresolvable")
	}
}
