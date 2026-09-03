# TC-14 Chat 图片输入 Token 字段

## 用例编号与名称

TC-14 Chat 图片输入 Token 字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.5`

## 测试目的

验证 chat 接口返回图片输入 token 时，`mod_access_pb3` 输出的 b2log 中 `ai_image_input_tokens`、`ai_input_tokens`、`ai_output_tokens`、`ai_total_tokens`、`ai_cost_value` 等字段被正确填充：

- `ai_mode` 应为 `chat`；
- `ai_image_input_tokens` 等于响应 `usage.input_token_details.image_tokens`；
- `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` 等于响应 usage 中的对应字段；
- `ai_cost_value` 按 `(prompt_tokens - image_input_tokens) * input_cost_per_token + image_input_tokens * input_cost_per_image_token + completion_tokens * output_cost_per_token` 计算。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 1000,
           "completion_tokens": 200,
           "total_tokens": 1200,
           "input_token_details": {
               "image_tokens": 300
           }
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `Provider = "mock-provider"`、`ModelTable.Currency = "RMB"`，且价格表包含 chat 视觉模型：
   - `Model`: `qwen3-vl-embedding`
   - `Mode`: `chat`
   - `input_cost_per_token = 0.000001`
   - `output_cost_per_token = 0.000002`
   - `input_cost_per_image_token = 0.0000005`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：
  - `Provider`: `mock-provider`
  - `ModelTable.Currency`: `RMB`
  - `ModelTable.Models` 增加 `qwen3-vl-embedding`：
    - `Mode`: `chat`
    - `Prices.input_cost_per_token`: `0.000001`
    - `Prices.output_cost_per_token`: `0.000002`
    - `Prices.input_cost_per_image_token`: `0.0000005`
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
| Body | `{"model":"qwen3-vl-embedding"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中。
- b2log 中存在 1 条 `RequestLog`，且字段满足：
  - `ai_mode` = `"chat"`
  - `ai_requested_model` = `"qwen3-vl-embedding"`
  - `ai_target_model` = `"qwen3-vl-embedding"`
  - `ai_input_tokens` = `1000`
  - `ai_output_tokens` = `200`
  - `ai_image_input_tokens` = `300`
  - `ai_total_tokens` = `1200`
  - `ai_cost_value` = `(1000 - 300) * 100 + 300 * 50 + 200 * 200 = 125000`
  - `ai_cost_currency` = `"RMB"`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
