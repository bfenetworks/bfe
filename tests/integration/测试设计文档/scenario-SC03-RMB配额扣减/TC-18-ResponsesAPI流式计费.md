# TC-18 Responses API 流式计费（response.completed 终结事件，issue #1381）

## 用例编号与名称

TC-18 Responses API 流式计费（SSE，`response.completed` 携带最终 usage）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1381 修复）

## 测试目的

验证 Responses API（Codex 形态：`POST /v1/responses`，`stream:true`）的 SSE 流被
`response.completed` 事件正常终结时，BFE 能解析该事件 `response.usage` 下的最终
usage（`input_tokens`/`output_tokens`/`total_tokens`/`input_tokens_details.cached_tokens`），
识别其为最终 usage 事件，并在请求结束后按完整公式扣减 RMB 配额。

修复前（issue #1381），usage 解析只认顶层 `usage.*` 且终结判定只认
`message_stop`/`[DONE]`，因此 `response.completed` 的 usage 全被解析为 0、最终
usage 标记永不置位；客户端收完流后正常关闭连接产生的 `CLIENT_CLOSE` 又被 #1352
守卫判为"客户端提前中止"，整请求零扣费（Redis 只有密钥查询与 TTL 更新）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200、`Content-Type: text/event-stream`，
   body 为 SSE 序列：
   ```
   data: {"type":"response.created","response":{"id":"resp_01","status":"in_progress"}}

   data: {"type":"response.output_text.delta","delta":"hello"}

   data: {"type":"response.completed","response":{"id":"resp_01","status":"completed","usage":{"input_tokens":2000,"output_tokens":1500,"total_tokens":3500,"input_tokens_details":{"cached_tokens":8000}}}}
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `gpt-5-codex`
   （mode 为 `responses`），价格：
   - `input_cost_per_token`: `0.000001` → 定点整数 `100`
   - `output_cost_per_token`: `0.000002` → 定点整数 `200`
   - `cache_read_input_token_cost`: `0.0000005` → 定点整数 `50`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `gpt-5-codex`（mode `responses`，
  价格见前置条件 4）。
- mock 后端 `ResponseHeaders = {"Content-Type": "text/event-stream"}`，
  `Body = responsesStreamUsageResponse`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/responses` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-5-codex","stream":true}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按完整公式扣减：
  - Responses API 的 `input_tokens` 不含 cache 命中部分，故解析时归一化
    PromptTokens = 2000 + 8000 = 10000，billable
    normal_input = 10000 - 8000 = 2000
  - 扣减金额 = 2000 * 100 + 8000 * 50 + 1500 * 200 = 900000
  - 剩余 = `10000000000 - 900000 = 99999100000`
- 修复前本用例扣减为 0（usage 未解析 + 中止守卫跳过了扣费）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
