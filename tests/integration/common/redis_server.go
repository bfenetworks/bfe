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

package common

import (
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// RedisServer wraps miniredis for integration tests.
type RedisServer struct {
	server *miniredis.Miniredis
	t      *testing.T
}

// NewRedisServer starts a new embedded redis server.
func NewRedisServer(t *testing.T) *RedisServer {
	s := &RedisServer{t: t}
	var err error
	s.server, err = miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	return s
}

// Addr returns the redis server address in "host:port" format.
func (s *RedisServer) Addr() string {
	return s.server.Addr()
}

// Close stops the redis server.
func (s *RedisServer) Close() {
	s.server.Close()
}

// SetQuota sets an integer quota value for the given key.
func (s *RedisServer) SetQuota(key string, value int64) {
	s.server.Set(key, fmt.Sprintf("%d", value))
}

// GetQuota returns the current integer quota value for the given key.
func (s *RedisServer) GetQuota(key string) int64 {
	v, err := s.server.Get(key)
	if err != nil {
		s.t.Fatalf("get quota %s failed: %v", key, err)
	}
	var value int64
	_, err = fmt.Sscanf(v, "%d", &value)
	if err != nil {
		s.t.Fatalf("parse quota %s value %s failed: %v", key, v, err)
	}
	return value
}

// Exists reports whether the given key is present in redis.
func (s *RedisServer) Exists(key string) bool {
	return s.server.Exists(key)
}

// Keys returns all keys currently present in redis.
func (s *RedisServer) Keys() []string {
	return s.server.Keys()
}

// FastForward advances the redis server clock, affecting key expiry.
func (s *RedisServer) FastForward(d time.Duration) {
	s.server.FastForward(d)
}
