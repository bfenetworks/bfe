# TC-06 clean-EOF 流 + `EstimateToken=false`：零扣费但有信号（issue #1415 / #1410）

## 目的

验证 issue #1415 修复在估算开关关闭时的行为边界：clean-EOF 流仍被标记"完整完成"（`response_completed=true`），但 `EstimateToken=false` 下请求不可按估算计费——**零扣费且带 `ai_usage_source=unrecognized` 信号**（issue #1410/#1414 口径），证明该形态在任何配置下都不静默。

同时验证完成标记本身不依赖估算开关：`response_completed=true` 与 `allow_estimate=false` 的组合必须进入 `UsageUnrecognized` 分支（warn 日志 `response_completed=true allow_estimate=false`），而不是被误标为可估算。

对应实现：`TestTC06_CleanEofEstimateTokenOffStaysZeroWithSignal`（`scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go`，env 变体 `newTestEnvWithEstimateToken(t, false)`——生成的 `bfe.conf` 被改写为 `EstimateToken = false`）。

## 前置条件

- BFE 已启动并加载 `cluster_usage_snapshot`，**`EstimateToken = false`**，加载 `mod_access_pb3`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端 SSERaw 模式：一帧 content chunk 后干净 EOF（同 TC-04，无 `[DONE]`、无 usage）。

## 请求构造

同 TC-04（`POST /v1/chat/completions`，Bearer `ak_user_a`，`stream:true` 请求体）。

## 后端响应（SSE，无终止事件）

同 TC-04。

## 执行步骤

1. 以 `EstimateToken = false` 配置启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端发送请求并完整读取 SSE 流直至连接关闭。
3. 停止 BFE，`common.ParseAccessLogAfterStop(logDir)` 解析 pb3 日志。
4. 断言 Redis 余额与 pb3 日志字段。

## 预期结果

- Redis 余额不变（`10000000000`）——估算关闭时不可计费。
- pb3 日志（该请求）：`ai_total_tokens = 0`、`ai_usage_source = unrecognized`（信号路径，#1410/#1414）。
- BFE warn 日志出现 `usage unrecognized and not estimatable, billing skipped: ... response_completed=true allow_estimate=false`——证明 clean-EOF 完成标记已置位、不可计费归因于估算开关。
- 对照：TC-04（同形态、`EstimateToken=true`）非零扣减——两 TC 合并证明修复只打通了"已批准的估算兜底"通路，未改变开关关闭时的既定口径。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
