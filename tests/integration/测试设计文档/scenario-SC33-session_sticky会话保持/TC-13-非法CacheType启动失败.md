# TC-13 非法CacheType启动失败

## 用例编号与名称

TC-13 非法CacheType启动失败

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 CacheType 非法时 BFE 启动失败（fail-fast）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- 将 `mod_session_sticky.conf` 的 `CacheType` 改写为 `bogus`。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| — | — | — | 不发起业务请求 | 只验证启动 |

## 预期结果

- BFE 未在超时内监听 HTTP 端口，`StartBFERaw` 返回错误。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC13_*` 函数
