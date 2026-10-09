# TC-02 EstimateToken 关：按真实嵌套 usage 计费与日志（issue #1401）

## 1. 测试目的

验证修复的**开关无关性**：`EstimateToken=false` 下，同一 Kimi 嵌套 usage 流仍按真实 usage 计费与落日志。修复前该开关下缺陷全量暴露——日志 `(0, -1, 0)`（负 output tokens 脏数据）、计费副本进清零块按 0 扣费。

## 2. 前置条件

- 同 TC-01，但 `EstimateToken = false`（测试在 `builder.Build()` 后重写生成的 `bfe.conf`；显式 conf 值覆盖程序默认值，`InjectStreamUsage = true` 保持不变）。
- mock 后端回放序列与 TC-01 完全相同。

## 3. 测试步骤

同 TC-01。

## 4. 预期结果

同 TC-01 第 1、3、4 条：透传不变；Redis 余额 = 10000000000 − 23313300；日志 `ai_input_tokens=77207`、`ai_output_tokens=168`（**非 -1 哨兵**）、`ai_total_tokens=77375`、`ai_cache_read_tokens=73472`。

## 5. 修复前行为（判别说明）

回退未生效且 `EstimateToken=false` 时：auth 不预置 prompt 估算、流式不累计估算 → 共享对象保持 `(0, -1, 0)`（`CompletionTokens` 为鉴权预置哨兵），计费副本进清零块 → 按 0 扣费（余额不变）。`ai_output_tokens=168` 与余额断言双判别。
