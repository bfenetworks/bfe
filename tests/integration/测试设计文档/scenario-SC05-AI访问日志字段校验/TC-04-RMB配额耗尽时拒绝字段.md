# TC-04 RMB 配额耗尽时拒绝字段

## 用例编号与名称

TC-04 RMB 配额耗尽时拒绝字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.2.0`

## 测试目的

验证当 RMB 配额余额不足导致认证拒绝时，访问日志中 `ai_auth_reject_reason` 与 `ai_auth_reject_quota_plans` 正确输出。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01，但 Redis 中 `quota:plan_rmb = 0`。
2. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `plan_rmb`：
  - `Unit`: `RMB`
  - `Quota`: `0`
  - `RedisKey`: `quota:plan_rmb`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：429。
- `cluster_rmb` 收到 0 次命中。
- b2log 中存在 1 条 `RequestLog`，且字段满足：
  - `ai_auth_reject_reason` 非空，包含 `QUOTA_EXHAUSTED` 或对应原因描述
  - `ai_auth_reject_quota_plans` = `["plan_rmb"]`
  - `ai_apikey_id` = `"user_a_key_id"`
  - `ai_route_rule_hits` 为空（请求在认证阶段被拒绝，未进入路由）
  - `ai_cluster_key_names` 为空

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
