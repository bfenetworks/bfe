# TC-13 ImageGeneration 图片输入 Token 字段

## 用例编号与名称

TC-13 ImageGeneration 图片输入 Token 字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.5`

## 测试目的

验证图像生成接口同时返回 `image_count` 与图片输入 token 时，`mod_access_pb3` 输出的 b2log 中 `ai_image_count`、`ai_image_input_tokens`、`ai_total_tokens`、`ai_cost_value` 等字段被正确填充：

- `ai_mode` 应为 `image_generation`；
- `ai_image_count` 等于响应 `usage.image_count`；
- `ai_image_input_tokens` 等于响应 `usage.input_token_details.image_tokens`；
- `ai_total_tokens` 在 image_generation 模式下按 `image_count` 扣减；
- `ai_cost_value` 按 `image_count * output_cost_per_image + image_input_tokens * input_cost_per_image_token` 计算。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "image_count": 1,
           "input_token_details": {
               "image_tokens": 200
           }
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `Provider = "mock-provider"`、`ModelTable.Currency = "RMB"`，且价格表包含图像生成模型：
   - `Model`: `gpt-image-1`
   - `Mode`: `image_generation`
   - `output_cost_per_image = 0.04`
   - `input_cost_per_image_token = 0.0000005`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：
  - `Provider`: `mock-provider`
  - `ModelTable.Currency`: `RMB`
  - `ModelTable.Models` 增加 `gpt-image-1`：
    - `Mode`: `image_generation`
    - `Prices.output_cost_per_image`: `0.04`
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
| Path | `/v1/images/generations` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-image-1","n":1}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中。
- b2log 中存在 1 条 `RequestLog`，且字段满足：
  - `ai_mode` = `"image_generation"`
  - `ai_requested_model` = `"gpt-image-1"`
  - `ai_target_model` = `"gpt-image-1"`
  - `ai_image_count` = `1`
  - `ai_image_input_tokens` = `200`
  - `ai_total_tokens` = `1`
  - `ai_cost_value` = `1 * 4000000 + 200 * 50 = 4010000`
  - `ai_cost_currency` = `"RMB"`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
