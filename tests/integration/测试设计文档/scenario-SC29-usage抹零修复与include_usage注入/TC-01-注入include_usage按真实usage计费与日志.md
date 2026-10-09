# TC-01 注入 include_usage 按真实 usage 计费与日志

## 目的

验证 issue #1398 的治本修复：OpenAI 流式 chat completion 请求未显式设置 `stream_options.include_usage` 时，BFE 自动注入 `stream_options.include_usage=true`（`Server.InjectStreamUsage`，出厂默认），上游据此回发真实 final usage chunk，计费与访问日志按真实 usage 落数。

同时验证两个配套点：

- 注入不覆盖客户端显式设置（本 TC 无 `stream_options`，正臂；反臂见 TC-02）。
- 访问日志（mod_access_pb3）读共享 `TokenUsage`：usage chunk 解析出的分项与 `UsedQuota` 原样落日志。

对应实现：`TestTC01_InjectIncludeUsageBillsActualUsage`（`scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_usage_snapshot`，`EstimateToken = true`、`InjectStreamUsage = true`，加载 `mod_access_pb3`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端 OpenAI SSE 模式：`Content-Type: text/event-stream`，依次发送 content chunk、usage chunk（`choices: []`）、`[DONE]`。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `usage-billing.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`（**不带** `stream_options`） |

## 后端响应（SSE）

```
data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"hello"}}]}

data: {"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}

data: [DONE]
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端发送请求并完整读取 SSE 流直至连接关闭。
3. 停止 BFE，`common.ParseAccessLogAfterStop(logDir)` 解析 pb3 日志。
4. 断言 mock 后端收包体、Redis 余额、pb3 日志字段。

## 预期结果

- mock 后端收到的请求体 `stream_options.include_usage == true`（JSON 布尔），其余字段（`model`/`stream`/`messages`）保持语义不变。
- Redis 余额 = `10000000000 - (10×300 + 20×900)` = `10000000000 - 21000`。
- pb3 日志（该请求）：`ai_input_tokens=10`、`ai_output_tokens=20`、`ai_total_tokens=30`。
- 修复前行为：客户端不带 `include_usage` 时上游依法不回 usage chunk（本 mock 模拟"收到 include_usage 才回"的上游），该类请求大量落成 `total_tokens=0`（issue 实测 42.7%）；修复后链路按真实 usage 落数。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
