# TC-09 Audio Token 字段

## 用例编号与名称

TC-09 Audio Token 字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.3`

## 测试目的

验证后端返回包含 `audio_input_tokens` / `audio_output_tokens` 的 usage 时，`mod_access_pb3` 输出的 b2log 中 `ai_audio_input_tokens`、`ai_audio_output_tokens` 被正确填充，且 `ai_cost_value` 按 audio-aware 公式计算。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 4000,
           "completion_tokens": 500,
           "total_tokens": 4500,
           "audio_input_tokens": 1000,
           "audio_output_tokens": 200
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `Provider = "mock-provider"`、`ModelTable.Currency = "RMB"`，且价格表包含音频价格：
   - `input_cost_per_token = 0.00000178`
   - `output_cost_per_token = 0.00000715`
   - `input_cost_per_audio_token = 0.00002288`
   - `output_cost_per_audio_token = 0.00004576`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：
  - `Provider`: `mock-provider`
  - `ModelTable.Currency`: `RMB`
  - `ModelTable.Models[0].Model`: `gpt-audio-1.5`
  - `Prices.input_cost_per_token`: `0.00000178`
  - `Prices.output_cost_per_token`: `0.00000715`
  - `Prices.input_cost_per_audio_token`: `0.00002288`
  - `Prices.output_cost_per_audio_token`: `0.00004576`
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
| Body | `{"model":"gpt-audio-1.5"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中。
- b2log 中存在 1 条 `RequestLog`，且字段满足：
  - `ai_input_tokens` = `4000`
  - `ai_output_tokens` = `500`
  - `ai_total_tokens` = `4500`
  - `ai_audio_input_tokens` = `1000`
  - `ai_audio_output_tokens` = `200`
  - `ai_cost_value` = `(4000 - 1000) * 178 + 1000 * 2288 + (500 - 200) * 715 + 200 * 4576 = 3951700`
  - `ai_cost_currency` = `"RMB"`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
