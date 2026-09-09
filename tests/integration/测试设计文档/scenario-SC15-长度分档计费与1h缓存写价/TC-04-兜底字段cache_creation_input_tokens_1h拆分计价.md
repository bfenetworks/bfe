# TC-04 兜底字段 `cache_creation_input_tokens_1h` 拆分计价

## 用例编号与名称

TC-04 兜底字段 `cache_creation_input_tokens_1h` 拆分计价

## 所属场景

SC15 长度分档计费与 1h 缓存写价

## 版本声明

- `bfe`：当前源码版本（2026-09-09 长度分档与 1h 缓存写价支持起）

## 测试目的

验证 relay 扁平兜底字段 `usage.cache_creation_input_tokens_1h`（非嵌套 `cache_creation` 结构）与 TC-03 的 Anthropic 扩展字段走相同的 1h 拆分计价逻辑：1h 部分按 1h 价、缓存写余量按 5m 基础价。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. mock 后端 `cluster_sc15` 返回 200 与响应体：
   `{"usage":{"prompt_tokens":20000,"completion_tokens":1000,"total_tokens":21000,"cache_write_tokens":15000,"cache_creation_input_tokens_1h":8000}}`
2. `claude-opus-4-8` 价格配置同 TC-03。
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
   `CalcCostUnits(7000, 3.84625e-05)`（缓存写余量 15000-8000=7000，5m 价） +
   `CalcCostUnits(8000, 6.154e-05)`（1h 部分 8000，1h 价） +
   `CalcCostUnits(1000, 0.00015385)`（输出 1000）；
   余额 = 10000000000 - 上述金额。
