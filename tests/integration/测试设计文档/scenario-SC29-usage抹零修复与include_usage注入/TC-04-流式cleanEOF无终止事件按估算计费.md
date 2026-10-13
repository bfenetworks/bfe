# TC-04 流式 clean-EOF 无 `[DONE]` 按估算计费（issue #1415）

## 目的

验证 issue #1415 的修复（OpenAI 臂）：上游 SSE 流**不发终止事件也不发 usage**，以干净 HTTP EOF 收尾时，响应仍被判定"完整完成"，`EstimateToken=true` 下按批准的估算兜底计费，访问日志记估算值非零、带 `ai_usage_source=estimated`。

本 TC 覆盖的缺陷路径生产实证（2026-10-12 13 点窗口，51 行/小时）：完整交付（200、`err_code` 空、`ai_stream_truncated=false`）但上游省略 `[DONE]`/usage 的流，`MarkResponseCompleted()` 永不置位 ⇒ 估算门控恒假 ⇒ 确定产生真实推理消耗却按 0 扣费。既有 TC-02 的 mock 发送 `[DONE]`，只覆盖"终止事件 present + usage 缺失"路径，本形态修复前无任何 TC 触达。

对应实现：`TestTC04_CleanEofWithoutDoneBillsEstimate`（`scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_usage_snapshot`，`EstimateToken = true`，加载 `mod_access_pb3`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端 SSERaw 模式：写一帧 `data: <content chunk>\n\n` 后 handler 直接返回——chunked 响应在终止 chunk 处干净 EOF，**无 `[DONE]`、无 usage chunk**。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `usage-billing.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}` |

## 后端响应（SSE，无终止事件）

```
data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"hello"}}]}

（handler 返回，chunked 干净 EOF；无 [DONE]）
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端发送请求并完整读取 SSE 流直至连接关闭。
3. 停止 BFE，`common.ParseAccessLogAfterStop(logDir)` 解析 pb3 日志。
4. 断言 Redis 余额与 pb3 日志字段。

## 预期结果

- 客户端完整收到 content 帧。
- 估算口径（与实现同公式，**不含 `[DONE]` 帧**）：
  - `promptEst` = 请求体字节数 / 4（鉴权阶段播种）；
  - `completionEst` = 各 SSE 事件 data 长度 / 4 之和（仅 content chunk）；
  - `totalEst` = `promptEst + completionEst`。
- Redis 余额 = `10000000000 - (promptEst×300 + completionEst×900)`——**非零扣减发生**（修复前 `total=0 且 cost=0`）。
- pb3 日志（该请求）：`ai_input_tokens=promptEst`、`ai_output_tokens=completionEst`、`ai_total_tokens=totalEst`（三者均 > 0）、`ai_usage_source=estimated`。
- 修复前行为：无终止事件 ⇒ `IsResponseCompleted()` 恒 false ⇒ `unrecognized` 清零守卫触发 ⇒ 零扣费且（旧构建）无任何信号。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
