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

-- KEYS[1]: unique dimension key for rate limiting
-- ARGV[1]: release step size (usually 1)
-- ARGV[2]: failover expiry time (seconds, e.g. 300), prevents unreleased concurrency if gateway crashes

local key = KEYS[1]
local decrement = tonumber(ARGV[1] or "1")
local ttl = tonumber(ARGV[2])

local current_time = 0
local t = redis.call('TIME')  --t[1]:s  t[2]:us
--local ms = math.floor((t[1] * 1000) + (t[2] / 1000))
current_time = t[1]

local current = tonumber(redis.call("GET", key) or "0")

if current <= 0 then
    -- If key has expired due to timeout or error, do not decrement to prevent negative concurrency count
    return {0}
else
    local next_cc = redis.call("DECRBY", key, decrement)
    -- Must set/refresh expiry to prevent deadlock/concurrency leak
    redis.call("EXPIRE", key, ttl)
    return {next_cc}
end
