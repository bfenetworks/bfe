# TC-03 `ephemeral_1h_input_tokens` 拆 5m/1h 两段计价

## 用例编号与名称

TC-03 `ephemeral_1h_input_tokens` 拆 5m/1h 两段计价

## 所属场景

SC15 长度分档计费与 1h 缓存写价

## 版本声明

- `bfe`：当前源码版本（2026-09-09 长度分档与 1h 缓存写价支持起）

## 测试目的

验证配置了 `PriceCacheCreationInputTokenCost1h` 时，Anthropic 扩展字段 `usage.cache_creation.ephemeral_1h_input_tokens`（OpenAI 风格 body 承载，1h 部分已含在 `cache_write_tokens` 之内）按 1h 价计价，缓存写余量按 5m 基础缓存写价计价，普通输入按 `prompt_tokens - cache_write_tokens` 计价。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. mock 后端 `cluster_sc15` 返回 200 与响应体：
   `{"usage":{"prompt_tokens":20000,"completion_tokens":1000,"total_tokens":21000,"cache_write_tokens":15000,"cache_creation":{"ephemeral_1h_input_tokens":10000}}}`
2. `claude-opus-4-8` 配置 5m 缓存写价 `3.84625e-05` 与 1h 缓存写价 `6.154e-05`。
3. `quota:plan_rmb` 初始配额 10000000000。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `sc15.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | `Bearer ak_user_a` |
| Content-Type | application/json |
| body | `{"model":"claude-opus-4-8"}` |

## 预期结果

1. 响应 200，`cluster_sc15` 命中 1 次。
2. 异步扣款完成后，扣费金额 =
   `CalcCostUnits(5000, 3.077e-05)`（正常输入 20000-15000=5000，输入价） +
   `CalcCostUnits(5000, 3.84625e-05)`（缓存写余量 15000-10000=5000，5m 价） +
   `CalcCostUnits(10000, 6.154e-05)`（1h 部分 10000，1h 价） +
   `CalcCostUnits(1000, 0.00015385)`（输出 1000）；
   余额 = 10000000000 - 上述金额。
