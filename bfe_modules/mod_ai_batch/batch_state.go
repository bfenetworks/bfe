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

package mod_ai_batch

import (
	"errors"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/bfenetworks/go-lib/log"
)

// BATCH_* Redis key layout. All keys live in the same Redis cluster as
// QUOTA_* and the AIKeyAffinity bindings (mod_ai_batch.conf [Redis]).
//
//	BATCH_FILE:<cluster>:<file_id>  HASH {api_key_id, key_name, lines, bytes,
//	                                purpose, dir}              TTL 48h
//	BATCH_TASK:<batch_id>           HASH {api_key_id, cluster, key_name,
//	                                input_file_id, est_lines, reserved_units,
//	                                status, usage_in, usage_out, settle_units,
//	                                settle_status}             TTL 48h
//	BATCH_SETTLED:<batch_id>        STRING "1" (SET NX EX)     TTL 48h
//	BATCH_ACTIVE:<api_key_id>       ZSET member=batch_id score=created_unix
const (
	batchFileKeyPrefix    = "BATCH_FILE:"
	batchTaskKeyPrefix    = "BATCH_TASK:"
	batchSettledKeyPrefix = "BATCH_SETTLED:"
	batchActiveKeyPrefix  = "BATCH_ACTIVE:"
)

// errRedisDisabled is returned when no redis is configured; it is treated
// like any other redis error (fail-open) by callers.
var errRedisDisabled = errors.New("mod_ai_batch: redis not configured")

// batchFileMeta is the read view of a BATCH_FILE binding.
type batchFileMeta struct {
	ApiKeyId string
	KeyName  string
	Lines    int64
	Bytes    int64
	Purpose  string
	Dir      string // input | output
	BatchId  string // set for dir=output bindings (created by a batch)
}

// batchTaskMeta is the read view of a BATCH_TASK record.
type batchTaskMeta struct {
	ApiKeyId    string
	Cluster     string
	KeyName     string
	InputFileId string
	EstLines    int64
	Status      string
	Reserved    bool
}

// Atomic Lua script sources (single key each, safe for redis cluster).
// redis_client.NewScript takes the source string and wraps redigo's Script.
var (
	// ARGV: ttl, then field/value pairs. HSET + EXPIRE atomically so a
	// crashed write can never leave a binding without expiry.
	luaHashSetWithTtl = `
local n = redis.call('HSET', KEYS[1], unpack(ARGV, 2))
redis.call('EXPIRE', KEYS[1], ARGV[1])
return n
`

	luaHashGetAll = `
return redis.call('HGETALL', KEYS[1])
`

	// ARGV: ttl. SET NX EX; returns "OK" or nil.
	luaSetNxWithTtl = `
return redis.call('SET', KEYS[1], '1', 'NX', 'EX', ARGV[1])
`

	// ARGV: ttl, value. SET NX EX carrying a payload (affinity bindings).
	luaSetNxValueWithTtl = `
return redis.call('SET', KEYS[1], ARGV[2], 'NX', 'EX', ARGV[1])
`

	// ARGV: score, member, ttl.
	luaZAddWithTtl = `
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return 1
`

	// ARGV: cutoff score. Purge entries older than the in-flight window,
	// then return the cardinality (active batch count for the apikey).
	luaZCountActive = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZCARD', KEYS[1])
`

	luaZRem = `
return redis.call('ZREM', KEYS[1], ARGV[1])
`
)

func batchFileKey(cluster, fileId string) string {
	return batchFileKeyPrefix + cluster + ":" + fileId
}

func batchTaskKey(batchId string) string {
	return batchTaskKeyPrefix + batchId
}

func batchSettledKey(batchId string) string {
	return batchSettledKeyPrefix + batchId
}

func batchActiveKey(apiKeyId string) string {
	return batchActiveKeyPrefix + apiKeyId
}

func nowUnix() int64 {
	return time.Now().Unix()
}

// runScript executes a script on the module redis client; all failures are
// fail-open: the caller logs/counts and continues without batch state.
func (m *ModuleAiBatch) runScript(src string, key string, args ...interface{}) (interface{}, error) {
	if m.redisClient == nil {
		return nil, errRedisDisabled
	}
	return m.redisClient.NewScript(src).Run(key, args...)
}

// writeFileBinding records an input (upload) or output (discovered) file
// binding. All errors are swallowed (fail-open); the caller counts.
func (m *ModuleAiBatch) writeFileBinding(cluster, fileId string, meta map[string]string, ttl int) {
	if fileId == "" {
		return
	}
	args := make([]interface{}, 0, 1+len(meta)*2)
	args = append(args, strconv.Itoa(ttl))
	for k, v := range meta {
		args = append(args, k, v)
	}
	_, err := m.runScript(luaHashSetWithTtl, batchFileKey(cluster, fileId), args...)
	if err != nil {
		m.state.Inc("FILE_BIND_WRITE_FAIL", 1)
		log.Logger.Warn("mod_ai_batch: write file binding %s err: %v", fileId, err)
		return
	}
	m.state.Inc("FILE_BIND_WRITE", 1)
}

// readFileBinding returns the binding, or (nil, nil) on miss. A redis error
// is fail-open: returns (nil, err) and the caller decides (allow_log/deny).
func (m *ModuleAiBatch) readFileBinding(cluster, fileId string) (*batchFileMeta, error) {
	if m.redisClient == nil {
		return nil, errRedisDisabled
	}
	vals, err := m.redisClient.HGetAll(batchFileKey(cluster, fileId))
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	return &batchFileMeta{
		ApiKeyId: vals["api_key_id"],
		KeyName:  vals["key_name"],
		Lines:    parseInt64(vals["lines"]),
		Bytes:    parseInt64(vals["bytes"]),
		Purpose:  vals["purpose"],
		Dir:      vals["dir"],
		BatchId:  vals["batch_id"],
	}, nil
}

// writeTaskRecord creates/updates a BATCH_TASK hash.
func (m *ModuleAiBatch) writeTaskRecord(batchId string, fields map[string]string, ttl int) {
	if batchId == "" {
		return
	}
	args := make([]interface{}, 0, 1+len(fields)*2)
	args = append(args, strconv.Itoa(ttl))
	for k, v := range fields {
		args = append(args, k, v)
	}
	_, err := m.runScript(luaHashSetWithTtl, batchTaskKey(batchId), args...)
	if err != nil {
		m.state.Inc("TASK_WRITE_FAIL", 1)
		log.Logger.Warn("mod_ai_batch: write task %s err: %v", batchId, err)
		return
	}
	m.state.Inc("TASK_WRITE", 1)
}

// readTaskRecord returns the task record, or (nil, nil) on miss.
func (m *ModuleAiBatch) readTaskRecord(batchId string) (*batchTaskMeta, error) {
	if m.redisClient == nil {
		return nil, errRedisDisabled
	}
	vals, err := m.redisClient.HGetAll(batchTaskKey(batchId))
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	return &batchTaskMeta{
		ApiKeyId:    vals["api_key_id"],
		Cluster:     vals["cluster"],
		KeyName:     vals["key_name"],
		InputFileId: vals["input_file_id"],
		EstLines:    parseInt64(vals["est_lines"]),
		Status:      vals["status"],
		Reserved:    vals["reserved"] == "1",
	}, nil
}

// tryMarkSettled atomically claims the settlement slot of a batch. Returns
// true when the caller won the claim (must settle), false when the batch was
// already settled/released by another path (download interception vs the
// reconcile job's backstop settle). On redis errors it fails closed for
// double-settle protection (claims nothing).
func (m *ModuleAiBatch) tryMarkSettled(batchId string, ttl int) bool {
	if batchId == "" {
		return false
	}
	reply, err := m.runScript(luaSetNxWithTtl, batchSettledKey(batchId), strconv.Itoa(ttl))
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		log.Logger.Warn("mod_ai_batch: mark settled %s err: %v", batchId, err)
		return false
	}
	s, _ := redis.String(reply, nil)
	return s == "OK"
}

// activeAdd records an in-flight batch for the apikey (ZSET score=now).
func (m *ModuleAiBatch) activeAdd(apiKeyId, batchId string, ttl int) {
	if apiKeyId == "" || batchId == "" {
		return
	}
	_, err := m.runScript(luaZAddWithTtl, batchActiveKey(apiKeyId),
		nowUnix(), batchId, strconv.Itoa(ttl))
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		log.Logger.Warn("mod_ai_batch: active add %s err: %v", batchId, err)
	}
}

// activeCount purges entries older than windowSec and returns the count.
func (m *ModuleAiBatch) activeCount(apiKeyId string, windowSec int64) int64 {
	if apiKeyId == "" {
		return 0
	}
	cutoff := nowUnix() - windowSec
	reply, err := m.runScript(luaZCountActive, batchActiveKey(apiKeyId), strconv.FormatInt(cutoff, 10))
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
		return 0 // fail-open: do not reject on count errors
	}
	n, _ := redis.Int64(reply, nil)
	return n
}

// activeRemove drops a batch from the apikey's in-flight set.
func (m *ModuleAiBatch) activeRemove(apiKeyId, batchId string) {
	if apiKeyId == "" || batchId == "" {
		return
	}
	_, err := m.runScript(luaZRem, batchActiveKey(apiKeyId), batchId)
	if err != nil {
		m.state.Inc("REDIS_ERR", 1)
	}
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
