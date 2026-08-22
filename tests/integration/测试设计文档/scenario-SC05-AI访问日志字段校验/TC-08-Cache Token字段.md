# TC-08 Cache Token 字段

## 用例编号与名称

TC-08 Cache Token 字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.1`

## 测试目的

验证后端返回包含 `cache_read_tokens` / `cache_write_tokens` 的 usage 时，`mod_access_pb3` 输出的 b2log 中 `ai_cache_read_tokens`、`ai_cache_write_tokens` 被正确填充，且 `ai_cost_value` 按 cache-aware 公式计算。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 100,
           "completion_tokens": 50,
           "total_tokens": 150,
           "cache_read_tokens": 30,
           "cache_write_tokens": 20
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `Provider = "mock-provider"`、`ModelTable.Currency = "RMB"`，且价格表包含 cache 价格：
   - `input_cost_per_token = 0.000001`
   - `output_cost_per_token = 0.000002`
   - `cache_read_input_token_cost = 0.0000005`
   - `cache_creation_input_token_cost = 0.0000015`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：
  - `Provider`: `mock-provider`
  - `ModelTable.Currency`: `RMB`
  - `ModelTable.Models[0].Model`: `deepseek-chat`
  - `Prices.input_cost_per_token`: `0.000001`
  - `Prices.output_cost_per_token`: `0.000002`
  - `Prices.cache_read_input_token_cost`: `0.0000005`
  - `Prices.cache_creation_input_token_cost`: `0.0000015`
  - `Keys`: 单 Key `key-primary`
- `plan_rmb`：
  - `Unit`: `RMB`
  - `Quota`: `10000000000`
  - `RedisKey`: `quota:plan_rmb`
- `ai_route.data` 中 `apikey_ak_user_a` 命中 `user_a-rmb`，target 为 `cluster_rmb`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中。
- b2log 中存在 1 条 `RequestLog`，且字段满足：
  - `ai_input_tokens` = `100`
  - `ai_output_tokens` = `50`
  - `ai_total_tokens` = `150`
  - `ai_cache_read_tokens` = `30`
  - `ai_cache_write_tokens` = `20`
  - `ai_cost_value` = `(100 - 30) * 100 + 30 * 50 + 20 * 150 + 50 * 200 = 21500`
  - `ai_cost_currency` = `"RMB"`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
