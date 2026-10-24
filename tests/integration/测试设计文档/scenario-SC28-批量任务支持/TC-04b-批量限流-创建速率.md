# TC-04b-批量限流-创建速率

## 测试步骤与断言

rate_limit 策略仅配 batch_limits.max_create_rpm=1 并绑定 ak_batch：第一次 POST /v1/batches 成功（预留写入）；第二次同请求返回 429，响应体含策略名 ratelimitBatch。

## 观测点

访问日志、miniredis（RL_BATCH_rlp-batch_rpm 计数）、mock 后端命中数。
