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

-- KEYS[1]: unique rate limiting key (e.g. "rate:qpm:tenant_A:model_v3")
-- ARGV[1]: max allowed burst buffer size (min 1)
-- ARGV[2]: emission period (seconds), fixed at 60 for QPM
-- ARGV[3]: allowed requests per period (standard QPM limit, e.g. 1000)
-- ARGV[4]: amount consumed by current request (usually 1)
-- ARGV[5]: current high-precision system timestamp (seconds, float, e.g. Go's time.Now().UnixNano() / 1e9)

local key = KEYS[1]
local burst = tonumber(ARGV[1])
local period = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local emission_cost = tonumber(ARGV[4])

--local now = tonumber(ARGV[5])

local current_time = 0
local t = redis.call('TIME') 
--local ms = math.floor((t[1] * 1000) + (t[2] / 1000))
current_time = t[1]

-- Redis cluster high-precision time
local now = t[1] + t[2] / 1000000


-- Calculate the theoretical emission increment (T) for the current request
local increment = (period / limit) * emission_cost

-- Max allowed burst time delay offset (Burst * T)
local burst_offset = burst * (period / limit)

-- Get the last calculated theoretical arrival time (TAT) from Redis
local tat = tonumber(redis.call("GET", key))
if tat == nil then
    -- First request, initialize TAT to current time
    tat = now
else
    -- If last TAT lags behind current time, connection has been idle, reset to current time
    tat = math.max(tat, now)
end

-- Calculate what the new TAT would be if this request is accepted
local new_tat = tat + increment

-- Core sliding window check: if new TAT minus burst offset still exceeds current time, the request exceeds the limit
if new_tat - burst_offset > now then
    -- return 0 -- trigger rate limiting, reject
    return {current_time, 0, tostring(tat), tostring(now)}
else
    -- Allow, update TAT timestamp in Redis
    redis.call("SET", key, new_tat)
    -- Dynamically set expiry, longer than one full sliding window to prevent memory leak
    redis.call("EXPIRE", key, math.ceil(period + 60))
    -- return 1 -- allow
    return {current_time, 1, tostring(new_tat), tostring(now)}
end
