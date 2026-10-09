# TC-02 显式关闭 include_usage 按估算兜底

## 目的

验证 issue #1398 的估算兜底与统计视图隔离：客户端显式设置 `stream_options.include_usage=false` 时 BFE 不注入（尊重显式选择），上游不回 usage chunk；请求正常完成（`[DONE]`）时按"缺 Usage 按 ContentLength/4 估算"的批准口径兜底计费，且**访问日志记估算值非零**（修复前清零块将日志与计费一并抹 0）。

本 TC 是 issue #1398 主缺陷（42.7% 清零行）的直接回归：同样的"完成但无 usage"形状，修复后日志 `ai_total_tokens` 必须非零。

对应实现：`TestTC02_ExplicitIncludeUsageFalseFallsBackToEstimate`（`scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_usage_snapshot`，`EstimateToken = true`、`InjectStreamUsage = true`，加载 `mod_access_pb3`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端 OpenAI SSE 模式：只发送 content chunk 与 `[DONE]`（模拟"客户端要求不回 usage"的上游行为）。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `usage-billing.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"gpt-4","stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}` |

## 后端响应（SSE）

```
data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"hello"}}]}

data: [DONE]
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端发送请求并完整读取 SSE 流直至连接关闭。
3. 停止 BFE，`common.ParseAccessLogAfterStop(logDir)` 解析 pb3 日志。
4. 断言 mock 后端收包体、Redis 余额、pb3 日志字段。

## 预期结果

- mock 后端收到的请求体 `stream_options.include_usage == false`，**未被改写为 true**。
- 估算口径（与实现同公式）：
  - `promptEst` = 鉴权阶段原始请求体字节数 / 4；
  - `completionEst` = 各 SSE 事件 data 长度 / 4 之和（content chunk 与 `[DONE]` 事件）；
  - `totalEst` = `promptEst + completionEst`。
- Redis 余额 = `10000000000 - (promptEst×300 + completionEst×900)`。
- pb3 日志（该请求）：`ai_input_tokens=promptEst`、`ai_output_tokens=completionEst`、`ai_total_tokens=totalEst`，三者均 > 0。
- 修复前行为：`!IsFinalUsageSeen && !estimateBillable` 时清零块将共享对象全字段置 0（`EstimateToken=true` 下完成的流虽可豁免计费，但旧实现中日志读同一对象被抹零；`EstimateToken=false` 下连计费也归 0）——本断言对"日志非零"直接防回归。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
