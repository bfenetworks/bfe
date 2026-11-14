# TC-29 OpenAI 压缩流式（gzip SSE）计费（issue #1468）

## 用例编号与名称

TC-29 OpenAI 压缩流式响应计费（`Content-Encoding: gzip` + `Transfer-Encoding: chunked` 的逐事件 SSE）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1468 修复）

## 测试目的

验证上游把 SSE 流式响应压缩后返回时，BFE 仍能从**解压后的明文**中提取 usage，并据此完成 RMB 计费：

1. 访问日志 `ai_input_tokens` / `ai_output_tokens` / `ai_cache_read_tokens` 反映上游真实 usage（而非估算），`ai_usage_source = parsed`；
2. RMB 扣减按逐项公式定点等值：未命中输入 × `input_cost_per_token` + cache_read × `cache_read_input_token_cost` + 输出 × `output_cost_per_token`；
3. **转发逐字节不变**：客户端收到的仍是上游那串 gzip 字节与 `Content-Encoding: gzip` 响应头（解压只服务解析）。

修复前（issue #1468）：`mod_body_process` 对带 `Content-Encoding` 的响应直接旁路为 `LineDecoder`，
压缩流完全不解析，`ai_cache_read_tokens` 恒为 0、`ai_output_tokens ≈ 压缩后字节 / 4`、
`ai_usage_source = estimated`，RMB 按「全量输入 × input 单价」多扣。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，使用**流式压缩 SSE 模式**：
   - 响应头：`Content-Type: text/event-stream`、`Content-Encoding: gzip`；
   - 传输：写出响应头后 flush 强制 chunked；
   - body：逐事件构造以下明文 SSE 帧，每帧经同一个 `gzip.Writer` 写入并 `Flush()`（模拟上游逐事件 flush），
     因此压缩字节是多帧刷出的**一个 gzip member**：
     ```
     data: {"id":"chatcmpl-tc29","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hello"}}]}

     data: {"id":"chatcmpl-tc29","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":296476,"completion_tokens":160,"total_tokens":296636,"prompt_tokens_details":{"cached_tokens":296320}}}

     data: [DONE]

     ```
4. 临时 BFE 配置已加载：
   - `cluster_rmb` 的 `ModelTable` 包含模型 `deepseek-flash`，价格：
     - `input_cost_per_token`: `0.000001` → 定点整数 `100`
     - `output_cost_per_token`: `0.000002` → 定点整数 `200`
     - `cache_read_input_token_cost`: `0.00000002` → 定点整数 `2`
   - `Server.EstimateToken = true`（默认值，用于证明修复后**不再走估算**）；
   - `Server.AIUpstreamAcceptEncoding = auto`（默认值）。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `common/mock_backend.go` 新增 `SSECompress` 能力（本用例取 `"gzip"`）：在 `SSEEvents`/`SSERaw`
  模式下把每帧经流式压缩器写入并 Flush，同时写出 `Content-Encoding` 头、强制 chunked。
  既有 `ResponseHeaders + Body` 只能模拟「一次性压缩的整块」，无法覆盖跨帧增量解压，故必须新增该能力。
- 无需新增 BFE 侧配置键（默认值即目标行为）。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Accept-Encoding | `gzip, deflate, br, zstd` |
| Content-Type | `application/json` |
| Body | `{"model":"deepseek-flash","stream":true}` |

## 执行步骤

1. 启动嵌入式 Redis 并预置余额。
2. 启动 mock 后端（gzip 压缩的流式 SSE）。
3. 启动 `bfe` 进程，加载临时配置。
4. 发送上表请求，完整读取响应 body 与响应头。
5. 等待异步 Redis 扣减完成后读取余额。
6. 读取 pb3 访问日志中该请求的 AI usage 字段。
7. 校验 `mod_body_process` 监控计数：`ResStreamCompressed`、`ResStreamDecoded` 各 +1，
   `ResStreamDecodeSkipped` / `ResStreamRawOnly` 不增。

## 预期结果

- 响应状态码：200；`cluster_rmb` 收到 1 次命中。
- 转发不变：响应头 `Content-Encoding: gzip`；响应 body 与上游 gzip 字节**逐字节一致**，
  `gunzip(body)` 等于前置条件 3 的明文 SSE；响应 body 非空（不得被截断）。
- 访问日志：
  - `ai_input_tokens = 296476`
  - `ai_output_tokens = 160`
  - `ai_cache_read_tokens = 296320`
  - `ai_total_tokens = 296636`
  - `ai_usage_source = parsed`
- Redis 扣减（定点整数，不得以容差替代）：
  - normal_input = 296476 - 296320 = 156
  - 扣减金额 = 156 × 100 + 296320 × 2 + 160 × 200 = 15600 + 592640 + 32000 = **640240**
  - 剩余 = `10000000000 - 640240 = 99999359760`
- 修复前对照：`ai_cache_read_tokens = 0`、`ai_output_tokens ≈ len(gzip body)/4`、`ai_usage_source = estimated`，
  扣减金额 ≈ `296476 × 100` 量级（多扣约 46 倍）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
