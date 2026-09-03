# TC-04 `peak` tier + 缓存命中 SSE 流式计费

## 用例编号与名称

TC-04 `peak` tier + 缓存命中 SSE 流式计费

## 所属场景

SC09 RMB 分时段计费

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证在 `peak` tier 下，SSE 流式响应的最终 chunk 含 `cache_read_tokens` 时，BFE 按 tier cache 拆分公式扣减 RMB 配额。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200，响应头包含 `Content-Type: text/event-stream`，body 为 SSE 格式：
   ```text
   data: {"choices":[{"delta":{"role":"assistant"}}]}

   data: {"choices":[{"delta":{"content":"hello"}}]}

   data: {"usage":{"prompt_tokens":8000,"completion_tokens":1500,"total_tokens":9500,"cache_read_tokens":5000,"cache_write_tokens":1000}}

   ```
4. 临时 BFE 配置已生成并加载，`cluster_rmb` 配置 `ModelTable`：
   - `Tiers` 中 `peak` 覆盖全周全天；
   - `deepseek-chat` 的默认 input/output/cache_read 价格分别为 `0.000001` / `0.000002` / `0.0000005`；
   - `deepseek-chat` 的 `TierPrices.peak` input/output/cache_read 价格分别为 `0.000002` / `0.000004` / `0.000001`。
5. `bfe.conf` 已启用 `mod_body_process`，确保 SSE 流式响应被处理。
6. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Tiers[0]`：
  - `Name`: `peak`
  - `TimeRanges[0].Weekdays`: `[0,1,2,3,4,5,6]`
  - `TimeRanges[0].Start`: `00:00`
  - `TimeRanges[0].End`: `23:59`
- `cluster_rmb.AIConf.ModelTable.Models[0]`：
  - `Model`: `deepseek-chat`
  - `Mode`: `chat`
  - `Prices.input_cost_per_token`: `0.000001`
  - `Prices.output_cost_per_token`: `0.000002`
  - `Prices.cache_read_input_token_cost`: `0.0000005`
  - `TierPrices.peak.input_cost_per_token`: `0.000002`
  - `TierPrices.peak.output_cost_per_token`: `0.000004`
  - `TierPrices.peak.cache_read_input_token_cost`: `0.000001`
- `plan_rmb`：
  - `Unit`: `RMB`
  - `Quota`: `10000000000`
  - `RedisKey`: `quota:plan_rmb`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `tier.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat","stream":true}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中。
- Redis 中 `quota:plan_rmb` 的余额变为：
  - `normal_input = 8000 - 5000 - 1000 = 2000`（`cache_creation_input_token_cost` 未配置按 0 处理，cache_write token 从 normal_input 中剥离后不重复计费）
  - 扣减金额 = `2000 * 200 + 5000 * 100 + 1000 * 0 + 1500 * 400 = 1500000`
  - 剩余 = `10000000000 - 1500000 = 9999850000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
