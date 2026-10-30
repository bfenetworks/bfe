# TC-32 RMB 配额超额全额记录

## 用例编号与名称

TC-32 RMB 配额超额全额记录（issue #1476）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1476 修复：扣减 Lua 去掉 `math.min` 截断，余额可为负）

## 测试目的

验证请求成本超过剩余额度时，扣减**全额**执行、Redis 余额如实变负（不再截断到 0），超额部分保留在余额中（`used = quota − remaining` 可推出真实消耗）；负余额在鉴权时视为"无余额"，后续请求被拒绝；欠费触发 `QuotaOverage` 计数。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis（miniredis，Lua 脚本真实执行）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000`（1e-8 定点单位，即 0.0001 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 usage 为 `prompt_tokens=100, completion_tokens=50` 的响应。
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `ModelTable`（input 单价 0.000001 元/token、output 0.000002 元/token，对应定点 100 / 200）。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`（`PassNoQuota=false`）。

## 配置构造

- `plan_rmb`：
  - `Unit`: `RMB`
  - `Quota`: `10000`
  - `RedisKey`: `quota:plan_rmb`
  - `PassNoQuota`: `false`

单次请求成本 = `100×100 + 50×200 = 20000` 定点单位，是余额的 2 倍。

## BFE 请求

发送 2 次 POST 请求（第 2 次在第 1 次扣减完成后发出）：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 第 1 次请求：200，`cluster_rmb` 命中 1 次；Redis `quota:plan_rmb` = **−10000**（修复前为 0，超额 10000 被丢弃）。
- `mod_ai_token_auth` 监控中 `QuotaOverage ≥ 1`。
- 第 2 次请求：429（负余额 = 无余额），`cluster_rmb` 不再新增命中。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
