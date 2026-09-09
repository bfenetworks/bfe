# TC-06 SSE 流式 1h 拆分计价

## 用例编号与名称

TC-06 SSE 流式 1h 拆分计价

## 所属场景

SC15 长度分档计费与 1h 缓存写价

## 版本声明

- `bfe`：当前源码版本（2026-09-09 长度分档与 1h 缓存写价支持起）

## 测试目的

验证 SSE 流式响应路径上，1h TTL 缓存写拆分逻辑与非流式一致：最终 usage 事件中的 `cache_creation.ephemeral_1h_input_tokens` 按 1h 价计价、缓存写余量按 5m 基础价计价（金额与 TC-03 完全一致）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. mock 后端 `cluster_sc15` 响应头 `Content-Type: text/event-stream`，SSE 响应体：

   ```
   data: {"choices":[{"delta":{"role":"assistant"}}]}

   data: {"choices":[{"delta":{"content":"hello"}}]}

   data: {"usage":{"prompt_tokens":20000,"completion_tokens":1000,"total_tokens":21000,"cache_write_tokens":15000,"cache_creation":{"ephemeral_1h_input_tokens":10000}}}

   ```

   前两个事件为内容增量，最后一个事件携带 usage（含 1h 部分）。
2. `claude-opus-4-8` 价格配置同 TC-03。
3. `quota:plan_rmb` 初始配额 10000000000。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `sc15.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | `Bearer ak_user_a` |
| Content-Type | application/json |
| body | `{"model":"claude-opus-4-8","stream":true}` |

## 预期结果

1. 响应 200，`cluster_sc15` 命中 1 次。
2. 异步扣款完成后，扣费金额与 TC-03 相同：
   `CalcCostUnits(5000, 3.077e-05)`（正常输入） +
   `CalcCostUnits(5000, 3.84625e-05)`（缓存写余量，5m 价） +
   `CalcCostUnits(10000, 6.154e-05)`（1h 部分，1h 价） +
   `CalcCostUnits(1000, 0.00015385)`（输出）；
   余额 = 10000000000 - 上述金额。
