# TC-21 Responses API 无缓存价格计费（subset 语义，issue #1389）

## 用例编号与名称

TC-21 Responses API 无缓存价格计费（未配置 cache 价格时 `cached_tokens` 不重复计费）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1389 修复；该修复更正 issue #1381 引入的 additive 前提）

## 测试目的

验证 OpenAI Responses API 的 subset 语义计费：未配置 cache 价格时，`input_tokens` 已含
`cached_tokens`（`total_tokens = input_tokens + output_tokens`），cached 部分不得按
input 价重复计费。

修复前（issue #1389，源自 ai-gateway-api#206），`ParseOpenAIUsageFields` 按 #1381 的
additive 假设把 PromptTokens 归一化为 `input_tokens + cached_tokens`；未配置 cache 价格时
`calcChatCost` 不触发 cache 拆分，虚增后的 PromptTokens 全部按 input 价计费，导致 cached
tokens 被重复计费一次（受控 fixture `input=100/output=50/cached=20`、input 价 1e-5 元/token：
实扣 220000、应扣 200000 定点单位）。

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
           "input_tokens": 100,
           "output_tokens": 50,
           "total_tokens": 150,
           "input_tokens_details": {"cached_tokens": 20}
       }
   }
   ```
   （`total_tokens=150=100+50` 自证 `input_tokens=100` 已含 cached 20，属 OpenAI subset 语义。）
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `gpt-5-codex`
   （mode 为 `responses`），**仅**配置 input/output 价格，不配置任何 cache 价格：
   - `input_cost_per_token`: `0.00001` → 定点整数 `1000`
   - `output_cost_per_token`: `0.00002` → 定点整数 `2000`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `gpt-5-codex`（mode `responses`，
  价格见前置条件 4）。
- mock 后端 `Body = responsesNoCacheUsageResponse`。

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
- Redis 中 `quota:plan_rmb` 的余额扣减：
  - subset 语义：PromptTokens = `input_tokens` = 100（已含 cached 20）；未配置 cache
    价格时 `calcChatCost` 不触发 cache 拆分，100 全部按 input 价计费一次
  - 扣减金额 = 100 * 1000 + 50 * 2000 = 200000
  - 剩余 = `10000000000 - 200000 = 9999800000`
- 修复前本用例扣减为 220000（PromptTokens 被归一化为 120，cached 20 按 input 价重复计费）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
