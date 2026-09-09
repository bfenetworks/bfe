# TC-01 30 万输入 token 超 272k 阈值按档价扣费

## 用例编号与名称

TC-01 30 万输入 token 超 272k 阈值按档价扣费

## 所属场景

SC15 长度分档计费与 1h 缓存写价

## 版本声明

- `bfe`：当前源码版本（2026-09-09 长度分档与 1h 缓存写价支持起）

## 测试目的

验证当请求输入 token 总数超过 272k 阈值时，`gpt-5.5` 的输入与输出**整单**按 `PriceInputCostPerTokenAbove272kTokens` / `PriceOutputCostPerTokenAbove272kTokens` 档价计费，而不是按基础价或仅超出部分按档价。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 已编译含长度分档计费的 `bfe` 可执行文件。
2. mock 后端 `cluster_sc15` 返回 200 与响应体：
   `{"usage":{"prompt_tokens":300000,"completion_tokens":50000,"total_tokens":350000}}`
3. `cluster_sc15.AIConf.ModelTable` 中 `gpt-5.5` 配置基础价与 272k 档价（档价必须不同于基础价，否则用例无法证明档选择生效——测试内有 `want == base` 的 setup 自检）。
4. `quota:plan_rmb` 初始配额 10000000000。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `sc15.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | `Bearer ak_user_a` |
| Content-Type | application/json |
| body | `{"model":"gpt-5.5"}` |

## 预期结果

1. 响应 200，`cluster_sc15` 命中 1 次。
2. 异步扣款完成后（sleep 500ms），扣费金额 =
   `CalcCostUnits(300000, 4.862e-05)`（输入按档价） +
   `CalcCostUnits(50000, 0.00021879)`（输出按档价）；
   余额 = 10000000000 - 上述金额。
3. 若实际按基础价扣费则余额不符，用例失败（证明档价选择生效）。
