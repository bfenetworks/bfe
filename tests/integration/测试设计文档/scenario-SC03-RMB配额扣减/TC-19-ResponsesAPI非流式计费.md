# TC-19 Responses API 非流式计费（顶层 usage + Responses API 字段名，issue #1381）

## 用例编号与名称

TC-19 Responses API 非流式计费（create-response 对象，顶层 `usage` + `input/output_tokens`）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1381 修复）

## 测试目的

验证 Responses API 非流式 create-response 对象的计费：usage 位于**顶层** `usage` 下
（不是流式事件的 `response.usage`），字段名为 `input_tokens` / `output_tokens` /
`total_tokens` / `input_tokens_details.cached_tokens`。解析链必须以 Responses API
特有的 `input_token(s)_details` 字段为门控进入该形态，同时不能把 Anthropic body
（同样有顶层 `usage.input_tokens`，但缓存字段是 `cache_read_input_tokens`）误判进来。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200、`Content-Type: application/json`，body：
   ```json
   {
       "id": "resp_01",
       "status": "completed",
       "usage": {
           "input_tokens": 2000,
           "output_tokens": 1500,
           "total_tokens": 3500,
           "input_tokens_details": {"cached_tokens": 8000}
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `gpt-5-codex`
   （mode 为 `responses`），价格同 TC-18：
   - `input_cost_per_token`: `0.000001` → 定点整数 `100`
   - `output_cost_per_token`: `0.000002` → 定点整数 `200`
   - `cache_read_input_token_cost`: `0.0000005` → 定点整数 `50`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `gpt-5-codex`（mode `responses`，
  价格见前置条件 4）。
- mock 后端 `Body = responsesUsageResponse`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/responses` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-5-codex"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按完整公式扣减：
  - `input_tokens` 不含 cache 命中部分，解析时归一化
    PromptTokens = 2000 + 8000 = 10000，billable
    normal_input = 10000 - 8000 = 2000
  - 扣减金额 = 2000 * 100 + 8000 * 50 + 1500 * 200 = 900000
  - 剩余 = `10000000000 - 900000 = 99999100000`
- 修复前该形态由跨协议兜底链按 Anthropic 语义解析，能扣减但丢失 cache-read 优惠
  （8000 cached 按 input 全价计），且 `input_token_details`（单复数）历史拼写差异
  使部分 relay 的 cached_tokens 完全丢失。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
