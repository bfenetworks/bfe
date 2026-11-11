# TC-08 上游断流注入 Anthropic 原生错误事件并按估算计费

## 用例编号与名称

TC-08 上游断流注入 Anthropic 原生错误事件并按估算计费（issue #1474）

## 所属场景

SC29 usage 抹零修复与 include_usage 注入

## 版本声明

- `bfe`：当前源码版本（含 issue #1474 修复）

## 测试目的

验证 issue #1474 的 Anthropic 协议臂：上游流中途死亡时，注入客户端的错误事件必须使用 **Anthropic 协议原生信封**（`event: error` + `{"type":"error",...}` 数据），而非 OpenAI 形态——Anthropic 客户端按 `event:` 行分发，错误形态不匹配会被当作未知事件静默丢弃。同时验证断流按已转发内容估算计费、`ai_usage_source=upstream_dropped`。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_usage_snapshot` 已启动，SSE 按顺序返回 `message_start`、`content_block_delta` 两帧后突然中止连接（`SSEAbortAfter = 2`，kill -9 形态）。
4. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`，`EstimateToken=true`（模板默认）；`ModelProtocols` 声明 `openai` 与 `anthropic`（Anthropic 请求按 `x-api-key` 头识别协议）。
5. `NormalizeUpstreamError.StreamTruncatedError` 保持默认（开启注入）。

## 配置构造

- 模型价格：`deepseek-chat`，`input_cost_per_token: 0.000003`（300 定点/token）、`output_cost_per_token: 0.000009`（900 定点/token）。
- 注入的 Anthropic 错误帧：`event: error` + 数据 `{"type":"error","error":{"type":"api_error","message":"upstream stream terminated abnormally."}}`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `usage.example.org` |
| Path | `/v1/messages` |
| x-api-key | `ak_user_a` |
| anthropic-version | `2023-06-01` |
| Body | `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}` |

客户端使用容错读取（容忍流末尾的传输错误）。

## 预期结果

- 响应状态码：200；客户端收到已转发的 `message_start` 帧；收到 `event: error` 信封的注入错误事件（Anthropic 原生形态）。
- Redis `quota:plan_rmb` 扣减 = `promptEst×300 + completionEst×900`：`promptEst = len(请求体)/4`；`completionEst = (len(message_start) + len(content_block_delta) + len(注入错误帧))/4`。
- 访问日志：`ai_usage_source=upstream_dropped`。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
