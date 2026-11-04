# TC-04 中止零扣费臂的信号（EstimateToken 关闭）

## 目的

验证 issue #1414 / #1478：`EstimateToken = false` 时，客户端中断且未拿到最终 usage 的请求保持零扣费，且该形态在监控与日志两侧与"上游 usage 未识别"可区分——两臂覆盖中断分支吸收的两种形态：

- **A1**：客户端在**任何 usage 事件出现前**中断；
- **A2**：中断前已解析到 `message_start` 的初始 usage（`UsedQuota > 0`，非最终事件）。

对应实现：`TestTC04_AbortBillingSkippedSignals`（`scenario-SC12-client-abort-billing/sc12_client_abort_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_client_abort`，**`EstimateToken = false`**（对照臂的 env 由 `newTestEnvWithEstimateToken(t, false)` 生成：与 TC-01~03 共用模板，仅翻转该开关）。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端进入 SSE 模式：发送首批帧后阻塞在 `SSEHold`，释放后追加一个大体积帧强制 `ErrClientWrite`。

## 请求构造

与 TC-01 相同（Host `abort-billing.example.org`、Path `/anthropic/v1/messages`、Bearer token、`stream:true`）。

## 后端响应（SSE）

- A1：`data: {"type":"ping"}` → 阻塞 → 释放后追加 1MB `content_block_delta`；
- A2：`data: {...message_start，含 input_tokens 320/output_tokens 0...}` → 阻塞 → 释放后追加 1MB `content_block_delta`。

## 执行步骤

1. 启动 BFE（EstimateToken=false）、mock 后端与内存 Redis，预置余额。
2. 两臂分别：读取到同步 marker（`ping` / `message_start`）→ `SO_LINGER=0` RST → 释放 `SSEHold`。
3. 等待请求收尾，读取 Redis 余额与 `mod_ai_token_auth` 计数器。

## 预期结果

- 两臂 Redis 余额均不变（`remaining = 10000000000`）——估算关闭，未拿到最终 usage 的中断请求保持零扣费（issue #1352 行为保留）。
- 计数器（两臂各 +1，累计）：`AbortBillingSkipped` +1/臂、`BillingZeroCost` +1/臂（绑定 RMB 计划且计费为 0）、`UsageUnrecognized` 保持 0（中断不是上游 usage 问题）、`AbortBillingEstimated` 保持 0（未发生估算计费）。
- 行级 `ai_usage_source=aborted` / `ai_zero_cost_reason=aborted` 由单元测试覆盖（本场景未加载 `mod_access_pb3`）。

## 与其他用例的关系

- 计费臂（`EstimateToken = true`）见 TC-01（issue #1478）；
- 计数器拆分口径见 `rmb_quota.md` §7.5 与 issue #1414/#1478 的修改说明。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。