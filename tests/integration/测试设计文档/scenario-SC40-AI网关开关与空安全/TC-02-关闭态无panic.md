# TC-02 关闭态无 panic

## 用例编号与名称

TC-02 关闭态无 panic

## 所属场景

SC40 AI 网关开关与空安全

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证关闭态下已加载 AI 模块对 `AiBasicInfo == nil` 的空安全处理（`74f2841c` 回归）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动。
3. `bfe.conf` 固定加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`；`EnableAiGateway` 由用例改写。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`；`token_rule.data` 声明不限量 Key `ak_auth`。

## 配置构造

- `EnableAiGateway=false`，其余模块保持加载。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1-10 | auth.example.org | /v1/chat/completions | 奇数次带 Key/偶数次不带 | {"model":"gpt-4"} | 连续 10 次请求 |

## 预期结果

- 10 次请求全部 HTTP 200，后端命中 10 次。
- BFE 进程存活：`/monitor/module_status` 返回 200。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC02_*` 函数
