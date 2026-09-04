# TC-02 收到最终 usage 后中断按实际计费

## 目的

验证 issue #1352 修复的边界：客户端在收到最终 usage（`message_delta`，含 `usage.output_tokens`）之后中断连接，BFE 仍按已确认的实际 usage 扣费——既不按完整请求体估算，也不免费。

对应实现：`TestTC02_ClientAbortAfterFinalUsageBillsActualUsage`。

## 前置条件

- 同 TC-01。

## 请求构造

同 TC-01。

## 后端响应（SSE）

```
data: {"type":"message_start","message":{"id":"msg_01","type":"message","usage":{"input_tokens":320,"output_tokens":0}}}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":150}}

（阻塞，直至客户端中断后释放）

data: {"type":"message_stop"}
```

## 执行步骤

1. 客户端发送请求，读取 SSE 数据直至出现 `message_delta`（此时 BFE 已解析最终 usage）。
2. 客户端 RST 中断连接。
3. 释放 `SSEHold`，后端发送 `message_stop`；BFE 写客户端失败（`ErrClientWrite`）。
4. 等待请求结束，读取 Redis 余额并断言。

## 预期结果

- Redis 余额扣减金额 = `320*452 + 150*2262 = 483940`（定点）。
- 修复前行为：`message_start` 设置 `UsedQuota` 后 `message_delta` 的 completion tokens 被忽略，只按输入计费（`320*452 = 144640`），输出 token 漏计。

## 补充说明

本 TC 同时验证两个配套修复：

1. `message_start.message.usage`（真实 Anthropic 报文结构）被正确解析。
2. `message_delta` 只携带输出 token，与 `message_start` 的输入 token 合并后计费（`content_quota_usage.go` 的 final usage 合并逻辑）。

对应单元测试：`TestQuotaUsageProcessorMarksResponseCompletion`（`mod_body_process`）、`TestExtractUsageFieldsClaudeStreamingMessageUsage`（`bfe_model_protocol/anthropic`）。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
