# TC-08 Sticky模式来源优先级

## 用例编号与名称

TC-08 Sticky模式来源优先级

## 所属场景

SC33 session_sticky 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 stickyid 来源优先级 Cookie > Header > URIParam > JSON 请求字段。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_sticky` 内启动 2 个 mock 后端（同一子集群，权重相同），用于区分 sticky 是否命中。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_session_sticky`。
4. 需要 Redis 的用例启动 miniredis，并由 harness 将 `[redis] Bns` 重写为测试实例。

## 配置构造

- `CacheType=redis` 启动 BFE。
- 规则：`Type=Sticky`、`Header=X-Sticky-Id`、`URIParam=sticky_id`、`StickyRequestField=previous_response_id`。
- 直接向 Redis 写入 4 个 stickyid 的映射：sid-cookie→A、sid-header→B、sid-uri→A、sid-json→B。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | sticky.example.org | /v1/chat/completions?sticky_id=sid-uri | Cookie=sid-cookie + Header=sid-header + JSON 字段 | 四源冲突 |
| 2 | sticky.example.org | /v1/chat/completions?sticky_id=sid-uri | Header=sid-header + JSON 字段 | 无 Cookie |
| 3 | sticky.example.org | /v1/chat/completions?sticky_id=sid-uri | 仅 URIParam + JSON 字段 | 无 Cookie/Header |
| 4 | sticky.example.org | /v1/chat/completions | 仅 JSON 字段 | 仅有请求体字段 |

## 预期结果

- 步骤 1 命中 A（Cookie 优先）；步骤 2 命中 B（Header 次之）。
- 步骤 3 命中 A（URIParam 再次之）；步骤 4 命中 B（JSON 字段最后）。
- 最终命中数 A=2、B=2。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC08_*` 函数
