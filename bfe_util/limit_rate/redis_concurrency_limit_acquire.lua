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

-- KEYS[1]: unique dimension key for rate limiting (e.g. "cc:tenant_A:model_r1")
-- ARGV[1]: max allowed concurrent connections
-- ARGV[2]: failover expiry time (seconds, e.g. 300), prevents unreleased concurrency if gateway crashes

local key = KEYS[1]
local max_cc = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

local current_time = 0
local t = redis.call('TIME') 
--local ms = math.floor((t[1] * 1000) + (t[2] / 1000))
current_time = t[1]

-- Get current concurrency count
local current = tonumber(redis.call("GET", key) or "0")

-- Check if over limit
if current >= max_cc then
    return {current_time, 0, current} -- Trigger rate limiting, reject connection
else
    -- Not over limit, increment concurrency count
    local next_cc = redis.call("INCR", key)
    -- Must set/refresh expiry to prevent deadlock/concurrency leak
    redis.call("EXPIRE", key, ttl)
    return {current_time, 1, next_cc} -- Return the latest concurrent connection count
end
