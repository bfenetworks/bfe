# TC-23 mode 无价格条目回落 chat 价计费（issue #1408）

## 用例编号与名称

TC-23 mode 无价格条目回落 chat 价计费（(model, mode) miss → fallback 到同 model 的 chat 价）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1408 修复，需 `bfe-access-pb >= v0.3.14` 的 `ai_price_miss` 字段）

## 测试目的

验证 issue #1408 的修复语义：当请求 mode 在价格表中没有对应 `(model, mode)` 条目、但同 model 存在 `chat` 条目时，成本按 `chat` 价计算（产品决策：未单独定价的 mode 与 chat 同价），请求被标记 `ai_price_miss` 并计入 `PriceFallbackUsed`；同时验证两个边界：

1. 同 model 的 `chat` 请求计费不受回落改动影响（金额与修复前一致）；
2. 价格表中完全没有条目的 model 仍按 0 计费（`chat` 基准不兜底，`PriceLookupMiss` 计数）。

修复前：`(deepseek-chat, responses)` 无条目 ⇒ `LookupModelPrice` 返回 nil ⇒ 按 0 计费且无任何日志标记（生产 /responses 入口 13 次请求全部漏计）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 返回 200、`Content-Type: application/json`，body 为 `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`。
4. `cluster_rmb` 的 `ModelTable` **只含 `deepseek-chat` 的 `chat` 条目**（`input_cost_per_token=1e-6`、`output_cost_per_token=2e-6`，即定点 100/200），**不含任何 `mode: "responses"` 条目**——即生产部署形态（`defaultRMBAIConf`）。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## BFE 请求

同一对照 Key 依次发送 3 次 POST（host 均为 `rmb.example.org`）：

| 臂 | Path | Body | 预期计费 |
|---|---|---|---|
| chat 对照臂 | `/v1/chat/completions` | `{"model":"deepseek-chat"}` | 命中 chat 条目：`100×100 + 50×200 = 20000` |
| responses 复现臂 | `/v1/responses` | `{"model":"deepseek-chat"}` | `(deepseek-chat, responses)` miss → 回落 chat 价：同 `20000`；标记 `ai_price_miss`、`PriceFallbackUsed` +1 |
| 无基线边界臂 | `/v1/chat/completions` | `{"model":"no-such-model"}` | 价格表无该 model 任何条目且 mode=chat 不回退：按 0 计费、`PriceLookupMiss` +1 |

## 预期结果

- 三次请求均返回 HTTP 200；`cluster_rmb` 共 3 次命中。
- 异步扣减完成后 `quota:plan_rmb` 余额 = `10000000000 − 20000 − 20000 = 99999960000`（无基线臂不扣减）。
- 访问日志：responses 复现臂与无基线臂的 `ai_price_miss = true`（SC03 未加载 `mod_access_pb3` 时此断言由单元测试 `TestReqAiInfoGenPriceMiss` 覆盖）；responses 臂的 `ai_cost_value = 20000` 与同 model chat 臂同口径。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
