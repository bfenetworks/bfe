# TC-02 RMB 配额余额不足拒绝请求

## 用例编号与名称

TC-02 RMB 配额余额不足拒绝请求

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 RMB 配额计划余额为 0 时，BFE 在认证阶段拒绝请求并返回 429 配额耗尽错误。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 0`。
3. mock 后端 `cluster_rmb` 已启动（本用例不应命中）。
4. 临时 BFE 配置已生成并加载，`cluster_rmb` 配置 `ModelTable`。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

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
- 响应 body 中包含错误码 `quota_exhausted` 或 `QUOTA_EXHAUSTED`。
- `cluster_rmb` 未被命中。
- Redis 中 `quota:plan_rmb` 保持为 0。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
