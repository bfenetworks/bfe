# TC-22 Anthropic 非流式 gzip+chunked 计费（issue #1406）

## 用例编号与名称

TC-22 Anthropic 非流式 gzip+chunked 计费（响应体 gzip 压缩 + chunked 传输 + Content-Type 带 charset）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1406 修复）

## 测试目的

验证当 Anthropic 协议的非流式响应以上游常见的三种形态组合返回时，BFE 的 usage 采集仍然有效：

1. `Transfer-Encoding: chunked`（无 `Content-Length`，`ContentLength == -1`）；
2. `Content-Encoding: gzip`（客户端自带 `Accept-Encoding: gzip`，反向代理 transport
   `DisableCompression: true` 故不会透明解压）；
3. `Content-Type: application/json; charset=utf-8`（带媒体类型参数）。

修复前（issue #1406），三者叠加使 `ai_input_tokens` 停留在鉴权估算值、`ai_output_tokens` 与
`ai_total_tokens` 为 0，RMB 计费按 0 入账。修复后 usage 必须被完整识别：日志字段反映响应真实
usage，RMB 按完整公式扣减。

同时验证「解压仅作用于解析副本」：转发给客户端的仍应是原始 gzip 字节与
`Content-Encoding: gzip` 响应头，逐字节不变。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动：
   - 响应头：`Content-Type: application/json; charset=utf-8`、`Content-Encoding: gzip`；
   - 传输：先 flush 响应头以强制 chunked（`NoContentLength = true`）；
   - body：gzip 压缩后的 Anthropic 非流式响应体：
     ```json
     {
         "id": "msg_01",
         "type": "message",
         "role": "assistant",
         "content": [{"type": "text", "text": "hi"}],
         "usage": {
             "input_tokens": 2000,
             "output_tokens": 1500,
             "cache_read_input_tokens": 8000
         }
     }
     ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `claude-sonnet-4-5`，价格：
   - `input_cost_per_token`: `0.000001` → 定点整数 `100`
   - `output_cost_per_token`: `0.000002` → 定点整数 `200`
   - `cache_read_input_token_cost`: `0.0000005` → 定点整数 `50`
   - `cache_creation_input_token_cost`: `0.0000015` → 定点整数 `150`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `claude-sonnet-4-5`（价格见前置条件 4）。
- mock 后端新增 gzip 响应开关（写头时置 `Content-Encoding: gzip`，body 写出 gzip 压缩字节），
  与既有 `NoContentLength`、`ResponseHeaders`（携带 `application/json; charset=utf-8`）组合使用。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Accept-Encoding | `gzip` |
| Body | `{"model":"claude-sonnet-4-5"}` |

## 执行步骤

1. 启动嵌入式 Redis 并预置余额。
2. 启动 mock 后端（gzip + chunked + 带 charset 的 JSON 响应）。
3. 启动 `bfe` 进程，加载临时配置。
4. 发送上表请求，读取响应（含响应头与原始 body）。
5. 等待异步 Redis 扣减完成后读取余额。
6. 读取 pb3 访问日志中该请求的 AI usage 字段。

## 预期结果

- 响应状态码：200；`cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 余额按完整公式扣减：
  - Anthropic `input_tokens` 不含 cache 命中部分，故
    normal_input = 2000 + 8000 - 8000 = 2000
  - 扣减金额 = 2000 * 100 + 8000 * 50 + 1500 * 200 = 900000
  - 剩余 = `10000000000 - 900000 = 99999100000`
- 访问日志 usage 字段反映响应真实 usage：
  - `ai_input_tokens = 10000`（2000 + 8000，含 cache_read 语义）
  - `ai_output_tokens = 1500`
  - `ai_cache_read_tokens = 8000`
  - `ai_total_tokens = 11500`
- 转发不变：客户端响应头仍为 `Content-Encoding: gzip`，响应 body 与上游 gzip 字节逐字节一致，
  解压后等于前置条件 3 的 JSON。
- 修复前：`ai_output_tokens = 0`、`ai_total_tokens = 0`、RMB 扣减 = 0。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。