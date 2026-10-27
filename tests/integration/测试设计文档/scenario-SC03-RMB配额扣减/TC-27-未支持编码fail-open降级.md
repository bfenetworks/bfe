# TC-27 未支持编码的 fail-open 降级（issue #1468）

## 用例编号与名称

TC-27 未支持编码的 fail-open 降级（上游无视协商返回不可解码编码）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1468 修复）

## 测试目的

验证当上游无视收窄后的协商、返回网关**解不了**的编码时，系统按既定口径降级而不是出错：

1. **转发不受影响**：客户端收到的字节与上游逐字节一致、`Content-Encoding` 头保留、响应不被截断；
2. **计费降级可见**：`ai_usage_source = estimated`（估算兜底仍生效，不得冒充 `parsed`），
   `ResStreamDecodeSkipped` 计数 +1；
3. `ResStreamRawOnly` 不被误计（这是「构造 tap 失败」，而非「解压中途失败」）。

该用例锁定的是 issue #1468 唯一的残余风险路径：上游不遵守 `Accept-Encoding` 时，计费回到估算，
必须由监控计数与日志可见，而不是静默少收。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动：
   - 响应头：`Content-Type: text/event-stream`、`Content-Encoding: compress`（任意网关不支持的编码）；
   - 传输：chunked；
   - body：**明文 SSE 字节**（因为不解压，BFE 原样转发；客户端在此仅用于断言字节一致，不做内容解析）：
     ```
     data: {"choices":[{"delta":{"content":"hello"}}]}

     data: [DONE]

     ```
4. 临时 BFE 配置：
   - `Server.AIUpstreamAcceptEncoding = auto`；
   - `Server.EstimateToken = true`；
   - `cluster_rmb` 的 `ModelTable` 价格与 TC-25 相同。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- 无需新增 BFE 配置键。
- mock 后端使用既有 `ResponseHeaders`（`Content-Encoding: compress`）+ `NoContentLength` + `Body` 即可，
  不依赖 `SSECompress`（该编码无法压缩生成，也不需要）。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Accept-Encoding | `gzip, deflate, br, zstd` |
| Body | `{"model":"deepseek-flash","stream":true}` |

## 执行步骤

1. 启动嵌入式 Redis 并预置余额。
2. 启动 mock 后端（`Content-Encoding: compress` + chunked）。
3. 启动 `bfe` 进程，加载临时配置。
4. 发送上表请求，完整读取响应 body 与响应头。
5. 等待异步扣减，读取 Redis 余额与 pb3 访问日志。
6. 读取 `mod_body_process` 监控计数。

## 预期结果

- 响应状态码：200；`cluster_rmb` 收到 1 次命中。
- 转发不变：响应头 `Content-Encoding: compress` 保留；响应 body 与上游 body 逐字节一致、非空。
- 访问日志：
  - `ai_usage_source = estimated`
  - `ai_cache_read_tokens = 0`
  - `ai_output_tokens` ≈ 响应字节 / 4（估算指纹；本 fixture 明文很小，取 `>= 0` 的宽松判据，
    关键是 `ai_usage_source` 的取值）
- 监控计数：`ResStreamCompressed +1`、`ResStreamDecodeSkipped +1`、`ResStreamDecoded` 不增、
  `ResStreamRawOnly` 不增。
- 日志出现一条 Warn：`unsupported content-encoding "compress", stream usage falls back to estimation`。
- Redis：按估算金额扣减（不断言具体数值，只断言「发生了扣减且与估算口径一致」）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
