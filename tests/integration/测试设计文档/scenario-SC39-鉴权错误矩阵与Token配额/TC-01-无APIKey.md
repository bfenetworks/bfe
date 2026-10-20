# TC-01 无 API Key

## 用例编号与名称

TC-01 无 API Key

## 所属场景

SC39 鉴权错误矩阵与 Token 配额计划

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证缺少 api-key 时返回 `NO_API_KEY`，请求不转发，访问日志记录拒绝原因。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动，返回 150 token 的 usage。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`。

## 配置构造

- `token_rule.data`：`ak_auth` 绑定 `plan-token`（`Unit=total_token`，余额 100000）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | （无） | {"model":"gpt-4"} | 不带 Authorization |

## 预期结果

- HTTP 401，`error.code=NO_API_KEY`；后端命中 0。
- 访问日志 `ai_auth_reject_reason=NO_API_KEY`。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC01_*` 函数
