# TC-07 上游流中途断流按已转发内容估算计费并注入协议错误事件（OpenAI 臂）

## 用例编号与名称

TC-07 上游流中途断流按已转发内容估算计费并注入协议错误事件（OpenAI 臂）（issue #1474）

## 所属场景

SC29 usage 抹零修复与 include_usage 注入

## 版本声明

- `bfe`：当前源码版本（含 issue #1474 修复）

## 测试目的

验证上游在流式输出中途异常死亡（kill -9 形态：无 usage、无 `[DONE]`、无终止 chunk）时：

1. 已转发给客户端的内容是真实消耗，按已转发内容估算计费（`EstimateToken=true`），计费金额与访问日志镜像估算值；
2. 归因正确：`err_code` 记 `BK_READ_RESP_BODY`（读侧错误），**不**再误报为 `CLIENT_WRITE`；访问日志 `ai_usage_source=upstream_dropped`；
3. 客户端收到 BFE 注入的协议错误事件（`upstream_stream_truncated`），明确告知答案不完整。

修复前：断流被归因为客户端写失败，请求走客户端中断路径——未拿到 final usage 时零扣费（#1352 口径），且客户端无感知（流静默截断）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_usage_snapshot` 已启动，SSE 按顺序返回 3 个 content chunk 后**突然中止连接**（`SSEAbortAfter = 3`，`panic(http.ErrAbortHandler)`，模拟上游进程被 kill：无终止 chunk）。
4. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`，`EstimateToken=true`（模板默认）。
5. `NormalizeUpstreamError.StreamTruncatedError` 保持默认（开启注入）。

## 配置构造

- 模型价格：`deepseek-chat`，`input_cost_per_token: 0.000003`（300 定点/token）、`output_cost_per_token: 0.000009`（900 定点/token）。
- 注入的错误帧载荷（计入估算，见预期结果）：`{"error":{"message":"upstream stream terminated abnormally.","type":"server_error","param":null,"code":"upstream_stream_truncated"}}`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `usage.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}` |

客户端使用容错读取（容忍流末尾的传输错误，因 BFE 收尾时无干净 chunked 终止符）。

## 预期结果

- 响应状态码：200；客户端收到 3 个已转发 content 帧；**未**收到 `[DONE]`（上游没发）；收到注入的错误事件，含 `upstream_stream_truncated`。
- Redis `quota:plan_rmb` 扣减 = `promptEst×300 + completionEst×900`：`promptEst = len(请求体)/4`（鉴权期播种）；`completionEst = (3×len(content帧) + len(注入错误帧))/4`（逐事件估算器计入全部已转发事件，含注入帧）。
- 访问日志：`ai_total_tokens = promptEst + completionEst`（镜像估算，#1398 语义）、`ai_usage_source=upstream_dropped`、`err_code=BK_READ_RESP_BODY`。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
