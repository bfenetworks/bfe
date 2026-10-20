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
	addr   string
	now    time.Time
	t      *testing.T
}

// NewRedisServer starts a new embedded redis server.
func NewRedisServer(t *testing.T) *RedisServer {
	s := &RedisServer{t: t, now: time.Now().UTC()}
	var err error
	s.server, err = miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}
	s.addr = s.server.Addr()
	return s
}

// Addr returns the redis server address in "host:port" format.
func (s *RedisServer) Addr() string {
	return s.addr
}

// Close stops the redis server.
func (s *RedisServer) Close() {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

// Fault stops the server so that subsequent client calls are rejected
// (connection refused), simulating a redis outage.
func (s *RedisServer) Fault() {
	s.Close()
}

// Recover starts a fresh empty server on the original address, restoring
// connectivity. It is used to assert fail-open behavior after an outage.
func (s *RedisServer) Recover() {
	if s.server != nil {
		return
	}
	srv := miniredis.NewMiniRedis()
	if err := srv.StartAddr(s.addr); err != nil {
		s.t.Fatalf("recover miniredis on %s failed: %v", s.addr, err)
	}
	s.server = srv
	s.now = time.Now().UTC()
}

// Set stores a raw string value for the given key.
func (s *RedisServer) Set(key string, value string) {
	s.server.Set(key, value)
}

// Get returns the string value of a key ("" when missing).
func (s *RedisServer) Get(key string) string {
	v, err := s.server.Get(key)
	if err != nil {
		return ""
	}
	return v
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

// HGetAll returns the field/value map of a hash key (nil when the key is
// missing or not a hash). Used by scenarios that assert on BATCH_* records.
func (s *RedisServer) HGetAll(key string) map[string]string {
	fields, err := s.server.HKeys(key)
	if err != nil || len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		out[field] = s.server.HGet(key, field)
	}
	return out
}

// Keys returns all keys currently present in redis.
func (s *RedisServer) Keys() []string {
	return s.server.Keys()
}

// FastForward advances the redis server clock, affecting key expiry.
func (s *RedisServer) FastForward(d time.Duration) {
	s.server.FastForward(d)
}

// AdvanceTime advances both the key-expiry clock and the virtual server clock
// (the one returned by the Redis TIME command). Time-based limiters that build
// sliding windows from TIME need the latter; FastForward alone only shifts
// key TTLs. The jump is measured from the later of the tracked virtual time
// and the current wall clock, so a jump is never diluted by elapsed test time.
func (s *RedisServer) AdvanceTime(d time.Duration) {
	s.server.FastForward(d)
	base := s.now
	if real := time.Now().UTC(); real.After(base) {
		base = real
	}
	s.now = base.Add(d)
	s.server.SetTime(s.now)
}

// ZMembers returns the members of a sorted set (nil when missing).
func (s *RedisServer) ZMembers(key string) []string {
	members, err := s.server.ZMembers(key)
	if err != nil {
		return nil
	}
	return members
}
