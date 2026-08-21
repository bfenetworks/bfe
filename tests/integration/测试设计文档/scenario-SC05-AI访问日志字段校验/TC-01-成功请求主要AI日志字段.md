# TC-01 成功请求主要 AI 日志字段

## 用例编号与名称

TC-01 成功请求主要 AI 日志字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.2.0`

## 测试目的

验证一次成功的 RMB 配额请求在 `mod_access_pb3` 输出的 b2log 中，各 AI 可观测字段被正确填充。

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
           "total_tokens": 150
       }
   }
   ```
4. 临时 BFE 配置已生成并加载，`cluster_rmb` 配置 `Provider = "mock-provider"`、`ModelTable.Currency = "RMB"`。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`，其 `key_id = "user_a_key_id"`，并携带标签 `[{"tagname":"department","tagvalue":"ai-team"}]`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：
  - `Provider`: `mock-provider`
  - `ModelTable.Currency`: `RMB`
  - `ModelTable.Models[0].Model`: `deepseek-chat`
  - `Prices.input_cost_per_token`: `0.000001`
  - `Prices.output_cost_per_token`: `0.000002`
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
  - `ai_apikey_id` = `"user_a_key_id"`（不是原始 key `ak_user_a`）
  - `ai_apikeytags` 包含 1 条记录：`tagname="department"`、`tagvalue="ai-team"`
  - `ai_requested_model` = `"deepseek-chat"`
  - `ai_target_model` = `"deepseek-chat"`
  - `ai_provider` = `"mock-provider"`
  - `ai_cost_value` = `100 * 100 + 50 * 200 = 20000`
  - `ai_cost_currency` = `"RMB"`
  - `ai_input_tokens` = `100`
  - `ai_output_tokens` = `50`
  - `ai_total_tokens` = `150`
  - `ai_route_rule_hits` 包含 1 条记录：`rule_owner="ak_user_a"`、`rule_owner_type="apikey"`、`rule_name="user_a-rmb"`
  - `ai_cluster_key_names` 包含 1 条记录：`cluster_name="cluster_rmb"`、`key_name="key-primary"`
  - `ai_auth_hit_quota_plans` 包含 `["plan_rmb"]`
  - `ai_retry_count` 未设置或为 0

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
