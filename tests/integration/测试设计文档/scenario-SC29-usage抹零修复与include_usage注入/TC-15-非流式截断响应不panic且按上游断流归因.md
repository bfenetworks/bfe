# TC-15 非流式截断响应不 panic 且按上游断流归因

> **状态更新（2026-11-12，「非流式边界」③ 规范降级）**：#1474「非流式边界」③ 已降级——客户端收尾为**优雅 FIN**（不再要求截断传输错误/RST，曾实施的异常终止修复已回退）。本用例客户端预期同步放宽为"不得收到完整伪造 body"；截断可检出性依赖响应分帧（定长短读/chunked 缺终止 chunk 由客户端 HTTP 栈自检），close-delimited 干净 EOF 为接受形态。`err_code=BK_READ_RESP_BODY` 与计费归因判据不变。QA 侧系统测试 SC3201-TC035 A-06 的客户端判据须同步放宽。

## 用例编号与名称

TC-15 非流式截断响应不 panic 且按上游断流归因（issue #1501，条款来源 issue #1474「非流式边界」）

## 所属场景

SC29 usage 抹零修复与 include_usage 注入

## 版本声明

- `bfe`：当前源码版本（含 issue #1501 修复）

## 测试目的

验证 `stream=false` 请求的上游 200 响应**body 中途截断**（`Content-Length` 已声明但连接被掐断）时：

1. BFE 不 panic——修复前 `GetBodyAccessor` 把失败读取的 `r.Body` 置空，`mod_body_process` 的 `JsonDecoder` 对 nil reader 解引用引发连接级 panic，请求在 `mod_access_pb3` 写日志前死亡；
2. 请求保留完整可观测性：RequestLog 行存在、`err_code=BK_READ_RESP_BODY`、`ai_usage_source=upstream_dropped`、按已转发内容计费（#1474 非流式边界①②③）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_usage_snapshot` 已启动；新增响应侧截断能力 `BodyTruncateAt`（issue #1501）：声明完整 `Content-Length`、写出前 128 字节后 `panic(http.ErrAbortHandler)` 掐断连接——非流式 kill -9 形态。
4. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`，`EstimateToken=true`（模板默认）。

## 配置构造

- 模型价格：`deepseek-chat`，`input_cost_per_token: 0.000003`（300 定点/token）、`output_cost_per_token: 0.000009`（900 定点/token）。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `usage.example.org` |
| Path | `/v1/messages` |
| x-api-key | `ak_user_a` |
| anthropic-version | `2023-06-01` |
| Body | `{"model":"claude-3","stream":false,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}` |

客户端使用容错读取（容忍截断传输错误，或因 close-delimit 而收到短 body）。

## 预期结果（PR #1504 评审修订后，true/false 双子臂）

- **无 panic**：`PanicClientConnServe` 前后差分 = 0（请求前后各读一次 `/monitor/proxy_state`）；先停 BFE 再读 `exception.log`，断言为空（进程停止冲刷日志，杜绝缓冲导致的空转通过）。
- **客户端**：HTTP 200 + 未完整收到声明的 body（截断传输错误或短读均可；**规范降级后**优雅 FIN 为接受收尾，不强制读错误——不得收到完整的伪造 body 即可）；无 SSE error 注入事件。
- **臂 A（EstimateToken=true，已转发内容口径）**：无字节送达客户端 ⇒ completion 估算为 0，扣减 = `len(请求体)/4 × 300` 定点；RequestLog `ai_usage_source=upstream_dropped`。
- **臂 B（EstimateToken=false）**：余额不变；RequestLog `ai_usage_source=unrecognized` 且 `ai_zero_cost_reason=upstream_dropped`。
- **RequestLog（两臂共有）**：行存在；`err_code=BK_READ_RESP_BODY`；`ai_stream=false`。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
