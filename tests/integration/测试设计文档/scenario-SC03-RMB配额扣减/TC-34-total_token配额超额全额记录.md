# TC-34 total_token 配额超额全额记录

## 用例编号与名称

TC-34 total_token 配额超额全额记录（issue #1476）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1476 修复）

## 测试目的

验证 token 单位配额计划（`deductToken` 路径）与 RMB 路径同构：请求 usage（150 token）超过剩余额度（100 token）时全额扣减，余额如实变负（−50），不再截断到 0。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_token = 100`。
3. mock 后端 `cluster_rmb` 已启动，返回 usage 为 `prompt_tokens=100, completion_tokens=50, total_tokens=150` 的响应。
4. `ak_user_a` 绑定 total_token 配额计划 `plan_token`。

## 配置构造

- `plan_token`：
  - `Unit`: `total_token`
  - `Quota`: `100`
  - `RedisKey`: `quota:plan_token`
  - `PassNoQuota`: `false`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：200，`cluster_rmb` 命中 1 次。
- Redis `quota:plan_token` = **−50**（修复前为 0，超出的 50 token 被丢弃）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
