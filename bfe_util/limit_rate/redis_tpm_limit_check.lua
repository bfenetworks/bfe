--[[
  Copyright (c) 2026 The BFE Authors.
  
  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at
  
      http://www.apache.org/licenses/LICENSE-2.0
  
  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
]]

-- tpm_token_limit_check.lua
--
-- Lua script implementing Redis Hash LLM TPM (Tokens Per Minute) sliding window rate limiting
-- Implements dual threshold checking: overall TPM threshold and per-bucket peak threshold
-- This operation attempts to consume a certain number of tokens
--
redis.replicate_commands()

-- 0: retrieve parameters
-- key
local key = KEYS[1]
-- args
local tpm_threshold = tonumber(ARGV[1])      -- overall TPM threshold
local window_size_sec = tonumber(ARGV[2])    -- window size (seconds)
local bucket_peak_limit = tonumber(ARGV[3])  -- per-bucket peak threshold
local bucket_size_sec = tonumber(ARGV[4])    -- bucket size (seconds)
local tokens_to_consume = tonumber(ARGV[5])  -- tokens consumed by this request

--return: 
----(current_time, is bucket hit threshold, whold tw hit threshold, bucket reminder token, whole tw reminder token )

local current_time = 0
local t = redis.call('TIME')  --t[1]:s  t[2]:us
--local ms = math.floor((t[1] * 1000) + (t[2] / 1000))
current_time = t[1]

--bucket window:[current_bucket_start,  current_bucket_start + bucket_size_sec]
local current_bucket_start = math.floor(current_time / bucket_size_sec) * bucket_size_sec

--whole window:[current_time - window_size_sec, current_time]
local cutoff_time = current_time - window_size_sec

-- clear buckets (field <= cutoff_time)
local fields_to_delete = {}
local all_fields = redis.call('HKEYS', key) 
for i, field_str in ipairs(all_fields) do
    local field_ts = tonumber(field_str)
    if field_ts and field_ts <= cutoff_time then
        table.insert(fields_to_delete, field_str)
    end
end

if #fields_to_delete > 0 then
    -- redis.call('HDEL', key, unpack(fields_to_delete))
    local batch_size = 256
    for i = 1, #fields_to_delete, batch_size do
        local batch = {}
        for j = i, math.min(i + batch_size - 1, #fields_to_delete) do
            table.insert(batch, fields_to_delete[j])
        end
        redis.call('HDEL', key, unpack(batch))
    end
end

local current_reminder_tokens = 0
local total_reminder_tokens = 0

local current_fields_and_values = redis.call('HGETALL', key)
-- HGETALL returns {field1, value1, field2, value2, ...}
local total_consumed_tokens = 0
local current_bucket_tokens = 0
for i = 1, #current_fields_and_values, 2 do
    local field_str = current_fields_and_values[i]
    local count_str = current_fields_and_values[i + 1]
    local field_ts = tonumber(field_str)
    local count = tonumber(count_str)
    if field_ts and count and field_ts > cutoff_time then
        total_consumed_tokens = total_consumed_tokens + count
    end
    if (current_bucket_start == field_ts) then
        current_bucket_tokens = count
    end
end

local is_bucket_allowed = 0
if (current_bucket_tokens + tokens_to_consume) <= bucket_peak_limit then
    is_bucket_allowed = 1
    current_bucket_tokens = current_bucket_tokens + tokens_to_consume
    current_reminder_tokens = bucket_peak_limit - current_bucket_tokens
end

local is_whole_allowed = 0
if (total_consumed_tokens + tokens_to_consume) <= tpm_threshold then
    is_whole_allowed = 1
    total_consumed_tokens = total_consumed_tokens + tokens_to_consume
    total_reminder_tokens = tpm_threshold - total_consumed_tokens
end

if (is_bucket_allowed==1) and (is_whole_allowed==1) then
    -- incre current bucket
    redis.call('HINCRBY', key, current_bucket_start, tokens_to_consume)
end

local ttl = 4 * window_size_sec
redis.call('EXPIRE', key, ttl)

return {current_time, is_bucket_allowed,is_whole_allowed,current_reminder_tokens,total_reminder_tokens}
