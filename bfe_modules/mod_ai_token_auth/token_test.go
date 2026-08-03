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

package mod_ai_token_auth

import (
	"errors"
	"testing"

	"github.com/bfenetworks/bfe/bfe_util/redis_client"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

type fakeRedisScript struct {
	keys []string
	args []interface{}
	run  func(key string, args ...interface{}) (interface{}, error)
}

func (f *fakeRedisScript) Run(key string, args ...interface{}) (interface{}, error) {
	return f.run(key, args...)
}

type fakeRedisClient struct {
	data    map[string]int64
	scripts map[string]redis_client.RedisScript
}

func newFakeRedisClient() *fakeRedisClient {
	return &fakeRedisClient{
		data:    make(map[string]int64),
		scripts: make(map[string]redis_client.RedisScript),
	}
}

func (f *fakeRedisClient) Setex(key string, value []byte, expire int) error {
	return nil
}

func (f *fakeRedisClient) Get(key string) (interface{}, error) {
	return nil, nil
}

func (f *fakeRedisClient) Expire(key string, expire int) error {
	return nil
}

func (f *fakeRedisClient) Incr(key string) (int64, error) {
	return 0, nil
}

func (f *fakeRedisClient) IncrAndExpire(key string, expire int) (int64, error) {
	return 0, nil
}

func (f *fakeRedisClient) Decr(key string) (int64, error) {
	return 0, nil
}

func (f *fakeRedisClient) PIncr(keys []string) ([]int64, error) {
	return nil, nil
}

func (f *fakeRedisClient) GetInt64(key string) (int64, error) {
	if v, ok := f.data[key]; ok {
		return v, nil
	}
	return 0, redis.ErrNil
}

func (f *fakeRedisClient) IncrBy(key string, delta int64) (int64, error) {
	return 0, nil
}

func (f *fakeRedisClient) NewScript(src string) redis_client.RedisScript {
	return f.scripts[src]
}

func TestQuotaPlan_HasBalance(t *testing.T) {
	client := newFakeRedisClient()

	t.Run("unlimited", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: true, Quota: 100, RedisKey: "key1"}
		hasBalance, balance, err := plan.HasBalance(client)
		assert.NoError(t, err)
		assert.True(t, hasBalance)
		assert.Equal(t, int64(100), balance)
	})

	t.Run("key not initialized", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: false, Quota: 100, RedisKey: "key2"}
		hasBalance, balance, err := plan.HasBalance(client)
		assert.NoError(t, err)
		assert.True(t, hasBalance)
		assert.Equal(t, int64(100), balance)
	})

	t.Run("has balance", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: false, Quota: 100, RedisKey: "key3"}
		client.data["key3"] = 50
		hasBalance, balance, err := plan.HasBalance(client)
		assert.NoError(t, err)
		assert.True(t, hasBalance)
		assert.Equal(t, int64(50), balance)
	})

	t.Run("exhausted", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: false, Quota: 100, RedisKey: "key4"}
		client.data["key4"] = 0
		hasBalance, balance, err := plan.HasBalance(client)
		assert.NoError(t, err)
		assert.False(t, hasBalance)
		assert.Equal(t, int64(0), balance)
	})

	t.Run("redis error", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: false, Quota: 100, RedisKey: "key5"}
		errClient := &fakeRedisClientWithGetErr{err: errors.New("connection refused")}
		_, _, err := plan.HasBalance(errClient)
		assert.Error(t, err)
	})
}

func TestQuotaPlan_Deduct(t *testing.T) {
	t.Run("amount <= 0", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", RedisKey: "key1", Quota: 100}
		remaining, err := plan.Deduct(newFakeRedisClient(), 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), remaining)
	})

	t.Run("unlimited", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Unlimited: true, RedisKey: "key1", Quota: 100}
		remaining, err := plan.Deduct(newFakeRedisClient(), 10)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), remaining)
	})

	t.Run("empty redis key", func(t *testing.T) {
		plan := &QuotaPlan{Id: "p1", Quota: 100}
		_, err := plan.Deduct(newFakeRedisClient(), 10)
		assert.Error(t, err)
	})

	t.Run("initialize on first deduct", func(t *testing.T) {
		client := newFakeRedisClient()
		client.scripts[deductScriptPattern()] = &fakeRedisScript{
			run: func(key string, args ...interface{}) (interface{}, error) {
				amount := args[0].(int64)
				quota := args[1].(int64)
				if _, ok := client.data[key]; !ok {
					client.data[key] = quota
				}
				if amount > client.data[key] {
					amount = client.data[key]
				}
				client.data[key] -= amount
				return client.data[key], nil
			},
		}
		plan := &QuotaPlan{Id: "p1", RedisKey: "key1", Quota: 100}
		remaining, err := plan.Deduct(client, 30)
		assert.NoError(t, err)
		assert.Equal(t, int64(70), remaining)
		assert.Equal(t, int64(70), client.data["key1"])

		remaining, err = plan.Deduct(client, 20)
		assert.NoError(t, err)
		assert.Equal(t, int64(50), remaining)
		assert.Equal(t, int64(50), client.data["key1"])
	})
}

// fakeRedisClientWithGetErr only implements GetInt64 for error testing.
type fakeRedisClientWithGetErr struct {
	err error
}

func (f *fakeRedisClientWithGetErr) Setex(key string, value []byte, expire int) error { return nil }
func (f *fakeRedisClientWithGetErr) Get(key string) (interface{}, error)             { return nil, nil }
func (f *fakeRedisClientWithGetErr) Expire(key string, expire int) error              { return nil }
func (f *fakeRedisClientWithGetErr) Incr(key string) (int64, error)                   { return 0, nil }
func (f *fakeRedisClientWithGetErr) IncrAndExpire(key string, expire int) (int64, error) {
	return 0, nil
}
func (f *fakeRedisClientWithGetErr) Decr(key string) (int64, error)                  { return 0, nil }
func (f *fakeRedisClientWithGetErr) PIncr(keys []string) ([]int64, error)            { return nil, nil }
func (f *fakeRedisClientWithGetErr) GetInt64(key string) (int64, error)              { return 0, f.err }
func (f *fakeRedisClientWithGetErr) IncrBy(key string, delta int64) (int64, error)   { return 0, nil }
func (f *fakeRedisClientWithGetErr) NewScript(src string) redis_client.RedisScript    { return nil }

func deductScriptPattern() string {
	return `
		local raw = redis.call('GET', KEYS[1])
		local current
		if raw == false then
			current = tonumber(ARGV[2])
			redis.call('SET', KEYS[1], current)
		else
			current = tonumber(raw)
		end
		local amount = tonumber(ARGV[1])
		local deduct = math.min(current, amount)
		if deduct > 0 then
			redis.call('DECRBY', KEYS[1], deduct)
		end
		return math.max(0, current - deduct)
	`
}
