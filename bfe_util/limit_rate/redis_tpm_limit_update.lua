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

-- tpm_token_limit_update.lua
-- 0: retrieve parameters
-- key
local key = KEYS[1]
-- args
local current_time = tonumber(ARGV[1])       -- return time in stage one
local bucket_size_sec = tonumber(ARGV[2])    -- bucket size (seconds)
local tokens_to_consume = tonumber(ARGV[3])  -- tokens consumed by this request

--return: 
----nil

local newVal = 0
if current_time <= 0 then
    return {newVal}
end

--bucket window:[current_bucket_start,  current_bucket_start + bucket_size_sec]
local current_bucket_start = math.floor(current_time / bucket_size_sec) * bucket_size_sec

-- current bucket usage
-- local current_bucket_tokens_str = redis.call('HGET', key, current_bucket_start)
-- if not current_bucket_tokens_str then 
--     --do nothing
-- else
--     newVal = redis.call('HINCRBY', key, current_bucket_start, tokens_to_consume)
-- end
newVal = redis.call('HINCRBY', key, current_bucket_start, tokens_to_consume)

return {newVal}

