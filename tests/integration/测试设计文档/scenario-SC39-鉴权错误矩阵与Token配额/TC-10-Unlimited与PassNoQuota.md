# TC-10 Unlimited 与 PassNoQuota

## 用例编号与名称

TC-10 Unlimited 与 PassNoQuota

## 所属场景

SC39 鉴权错误矩阵与 Token 配额计划

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证两条放行路径：Key 级 `unlimited_quota=true` 与计划级 `PassNoQuota=true`。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动，返回 150 token 的 usage。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`。

## 配置构造

- 子用例 1：`ak_auth.unlimited_quota=true`，不绑定配额计划。
- 子用例 2：`plan-pass.PassNoQuota=true`，`quota:pass=0`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | Bearer ak_auth | {"model":"gpt-4"} | unlimited Key |
| 2 | auth.example.org | /v1/chat/completions | Bearer ak_auth | {"model":"gpt-4"} | PassNoQuota 计划 |

## 预期结果

- 子用例 1：HTTP 200，Redis 中没有任何键（未发生余额检查/扣减）。
- 子用例 2：HTTP 200（余额 0 仍放行）。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC10_*` 函数
