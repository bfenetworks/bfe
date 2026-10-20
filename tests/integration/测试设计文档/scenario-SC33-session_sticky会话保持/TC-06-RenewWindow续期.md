# TC-06 RenewWindow续期

## 用例编号与名称

TC-06 RenewWindow续期

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证到期后重新签发 Cookie，未到期不重签。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- 同 TC-01。
- Arm 1：构造 `renewtime` 已过的 Cookie；Arm 2：构造 `renewtime` 为未来的 Cookie。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions | 续期已到 | 期望重签 |
| 2 | sticky.example.org | /v1/chat/completions | 续期未到 | 期望不重签 |

## 预期结果

- Arm 1：响应返回 `Set-Cookie`。
- Arm 2：响应不返回 `Set-Cookie`。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC06_*` 函数
