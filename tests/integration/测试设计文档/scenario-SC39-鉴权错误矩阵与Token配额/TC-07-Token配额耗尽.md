# TC-07 Token 配额耗尽

## 用例编号与名称

TC-07 Token 配额耗尽

## 所属场景

SC39 鉴权错误矩阵与 Token 配额计划

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证余额被用尽后下一次请求返回 `QUOTA_EXHAUSTED`，并区分于「计划过期 / Redis 故障」。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动，返回 150 token 的 usage。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`。

## 配置构造

- `plan-token`：`Unit=total_token`，初始余额 50（小于单次 usage 150）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | Bearer ak_auth | {"model":"gpt-4"} | 余额 50，扣至 0 |
| 2 | auth.example.org | /v1/chat/completions | Bearer ak_auth | {"model":"gpt-4"} | 余额 0 |

## 预期结果

- 步骤 1：HTTP 200（扣减按 `min(余额, usage)` 截断），`quota:token` 变为 0。
- 步骤 2：HTTP 429，`error.code=QUOTA_EXHAUSTED`，`details.limit_type=api_key_quota`，`details.quota_plan_id=plan-token`。
- 访问日志 `ai_auth_reject_reason=QUOTA_EXHAUSTED`、`ai_auth_reject_quota_plans=[plan-token]`。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC07_*` 函数
