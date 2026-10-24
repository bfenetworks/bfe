# TC-01 Cookie模式首次请求注入并绑定

## 用例编号与名称

TC-01 Cookie模式首次请求注入并绑定

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 Cookie 模式首次请求返回加密的后端 Cookie，并绑定实际服务的后端。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- 规则：`Type=Cookie`、`CookieKey=bfe_ssbl`、`MaskCode/StandbyMaskCode` 显式配置、`MaxAge=3600`、`RenewWindow=1800`。
- 两个后端均返回 200。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions | 无 Cookie | 首次请求 |

## 预期结果

- HTTP 200；`Set-Cookie: bfe_ssbl=<密文>`；值为掩码后的 base64（不含明文地址）。
- 解密后含 `ip`/`port`/`subcluster`（本场景子集群键为 `sub_sub`）。
- 恰好 1 个后端命中 1 次。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC01_*` 函数
