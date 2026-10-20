# TC-10 Sticky模式Redis跨重启复用与过期

## 用例编号与名称

TC-10 Sticky模式Redis跨重启复用与过期

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 Redis 映射跨 BFE 重启复用，并按 ExpireSeconds 过期。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- `CacheType=redis`；`ExpireSeconds=60`。
- 规则同 TC-09。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions | 无 stickyid | 写入 `bfe:stickyid:resp-10` |
| 2 | sticky.example.org | /v1/chat/completions | 重启 BFE 后带 `X-Sticky-Id: resp-10` | 跨重启命中 |
| 3 | sticky.example.org | /v1/chat/completions | Redis 虚拟时间推进 61s 后同 Header | 映射已过期 |

## 预期结果

- 步骤 1 后 Redis 键存在；步骤 2 命中同一后端（绑定后端累计 2 次）。
- 步骤 3 前键已过期（`Exists=false`）；请求仍返回 200（重新均衡）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC10_*` 函数
