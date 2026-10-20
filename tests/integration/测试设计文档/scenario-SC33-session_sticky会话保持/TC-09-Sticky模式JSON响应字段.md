# TC-09 Sticky模式JSON响应字段

## 用例编号与名称

TC-09 Sticky模式JSON响应字段

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证从 JSON 响应体提取 stickyid 并用于后续请求。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- 本地缓存模式（`CacheType=local`）。
- 规则：`Type=Sticky`、`Header=X-Sticky-Id`、`StickyResponseField=response_id`。
- 后端响应体含 `"response_id":"resp-9"`。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions | 无 stickyid | 建立映射 |
| 2 | sticky.example.org | /v1/chat/completions | Header: X-Sticky-Id=resp-9 | 使用响应字段映射 |

## 预期结果

- 两次请求命中同一后端（累计 2 次），另一后端 0 次。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC09_*` 函数
