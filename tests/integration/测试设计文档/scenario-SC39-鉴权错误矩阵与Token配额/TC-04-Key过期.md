# TC-04 Key 过期

## 用例编号与名称

TC-04 Key 过期

## 所属场景

SC39 鉴权错误矩阵与 Token 配额计划

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证过期 Key 返回 `KEY_EXPIRED`。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动，返回 150 token 的 usage。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`。

## 配置构造

- `ak_auth`：`expired_time` 设为当前时间之前。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | Bearer ak_auth | {"model":"gpt-4"} | 过期 Key |

## 预期结果

- HTTP 403，`error.code=KEY_EXPIRED`。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC04_*` 函数
