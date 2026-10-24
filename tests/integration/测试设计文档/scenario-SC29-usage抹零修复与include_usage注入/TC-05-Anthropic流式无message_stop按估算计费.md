# TC-05 Anthropic 流式无 `message_stop` 按估算计费（issue #1415 生产桶A精确复刻）

## 目的

验证 issue #1415 的修复（Anthropic 臂）：anthropic-compat 上游**完全不含 usage 事件**（`message_start` 无 `usage` 字段）、不发 `message_delta`、不发 `message_stop`，以干净 HTTP EOF 收尾时，按批准的估算兜底计费，访问日志记估算值非零、带 `ai_usage_source=estimated`。

这是 issue 生产实证的精确复刻：2026-10-12 13 点窗口 51 行流式 `total=0` 呈完整断链签名（`ai_input_tokens == req_body_len/4`、`ai_output_tokens ≈ res_body_len/4 × 0.93`、`total=0 且 cost=0`），全部来自 deepseek anthropic-compat 端点的 omit-usage 流。修复前 openai/anthropic 适配器只认终止事件、无 EOF 兜底（gemini 先例仅豁免截断标记，未补计费完成标记）。

对应实现：`TestTC05_AnthropicCleanEofWithoutMessageStopBillsEstimate`（`scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_usage_snapshot`，`EstimateToken = true`，加载 `mod_access_pb3`。
- `cluster_usage_snapshot` 的 AIConf 声明 `ModelProtocols: ["openai","anthropic"]`，`claude-3` 经 ModelMapping 映射到 `deepseek-chat`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端 SSERaw 模式按序写三帧（`message_start` 不带 usage、content delta、content_block_stop）后 handler 直接返回——chunked 干净 EOF，**无 `message_stop`、无任何 usage**。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `usage-billing.example.org` |
| Path | `/v1/messages` |
| Header | `x-api-key: ak_user_a` |
| Header | `anthropic-version: 2023-06-01` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}` |

## 后端响应（SSE，omit-usage 无终止事件）

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-3-sonnet"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

（handler 返回，chunked 干净 EOF；无 message_stop、无 message_delta/usage）
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端发送 Anthropic 请求并完整读取 SSE 流直至连接关闭。
3. 停止 BFE，`common.ParseAccessLogAfterStop(logDir)` 解析 pb3 日志。
4. 断言透传内容、Redis 余额与 pb3 日志字段。

## 预期结果

- 客户端完整收到三帧（含 `msg_01`），**不得**收到 `message_stop`（parser 只读不改事件）。
- 估算口径（与实现同公式，逐事件 data 长度 /4 求和）：
  - `promptEst` = 请求体字节数 / 4；
  - `completionEst` = Σ `len(data_i)/4`（message_start、delta、block_stop 三帧，均为 guess 事件参与累加）；
  - `totalEst` = `promptEst + completionEst`。
- Redis 余额 = `10000000000 - (promptEst×300 + completionEst×900)`——非零扣减发生。
- pb3 日志（该请求）：`ai_input_tokens=promptEst`、`ai_output_tokens=completionEst`、`ai_total_tokens=totalEst`（三者均 > 0）、`ai_usage_source=estimated`。
- 修复前行为：anthropic 适配器 `IsStreamTerminal` 只认 `message_stop`/`[DONE]`，两个完成标记都置不上 ⇒ 零扣费。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
