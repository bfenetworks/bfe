# TC-14 total_token 配额为 0 且 RedisKey 未初始化时拒绝请求

## 用例编号与名称

TC-14 total_token 配额为 0 且 RedisKey 未初始化时拒绝请求

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 `Unit=total_token` 的 QuotaPlan 配置 `Quota=0` 且 Redis 余额 key 从未初始化（key 不存在）时，BFE 在认证阶段返回 429 配额耗尽错误，而不是 500 `INTERNAL_QUOTA_ERROR`。

> 背景：`QuotaPlan.HasBalance` 查询 Redis 时，key 不存在返回 `redis.ErrNil`。旧实现将 `ErrNil` 视为内部错误返回 500；修复后视为余额 0，返回 `QuotaExhausted`。本用例是该修复的回归测试。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，**不预置** `quota:plan_token`（key 不存在）。
3. mock 后端 `cluster_rmb` 已启动（本用例不应命中）。
4. 临时 BFE 配置已生成并加载，`cluster_rmb` 配置 `ModelTable`。
5. `ak_user_a` 绑定 Token 配额计划 `plan_token`。

## 配置构造

- `plan_token`：
  - `Unit`: `total_token`
  - `Quota`: `0`
  - `RedisKey`: `quota:plan_token`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- `token_rule.data` 加载成功，BFE 正常启动监听。
- 响应状态码：429（而非 500）。
- 响应 body 中包含配额错误信息，不包含内部错误信息。
- `cluster_rmb` 未被命中。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
