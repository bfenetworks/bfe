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

-- Sliding Window QPM Rate Limiter
-- KEYS[1]: rate limit key
-- ARGV[1]: period (main window duration, seconds)
-- ARGV[2]: limit (max requests in main window)
-- ARGV[3]: emission_cost (requests consumed, usually 1)

local key = KEYS[1]
local period = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local emission_cost = tonumber(ARGV[3])

local t = redis.call('TIME')
local current_time = t[1]
local now = t[1] + t[2] / 1000000

redis.call('ZREMRANGEBYSCORE', key, 0, now - period)

local count = redis.call('ZCARD', key)

if count >= limit then
    return {current_time, 0, count, now}
end

local member = tostring(now) .. ':' .. tostring(t[2])
redis.call('ZADD', key, now, member)
redis.call('EXPIRE', key, math.ceil(period + 60))

return {current_time, 1, count + emission_cost, now}