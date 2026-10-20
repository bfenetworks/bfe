# TC-03 Cookie属性

## 用例编号与名称

TC-03 Cookie属性

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 MaxAge/Domain/Path/Secure/HttpOnly 属性按配置下发。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- 规则：`CookieKey=sess`、`Domain=.example.org`、`Path=/`、`MaxAge=600`、`RenewWindow=600`、`Secure=true`、`HttpOnly=true`。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions | 无 Cookie | 首次请求 |

## 预期结果

- `Set-Cookie` 含 `Max-Age=600`、`Path=/`、`Domain=example.org`（前导点被序列化去掉）、`Secure`、`HttpOnly`。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC03_*` 函数
