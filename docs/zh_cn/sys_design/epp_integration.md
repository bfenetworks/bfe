# BFE 对接 EPP 调度设计（ext-proc）

## 1. 背景与目标

### 1.1 背景

BFE 原有负载均衡为内置策略（WRR / WLC 等），调度决策完全在 BFE 进程内完成。AI 网关场景下，后端为 vLLM 等推理实例，调度需要感知**实时负载、前缀缓存亲和、会话亲和、流控排队**等状态，这些状态由独立的调度器 **ai-gateway-epp**（基于 llm-d 演进的多 cluster 单进程 EPP）维护。

对接方式为 Envoy [ext-proc](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/ext_proc.proto) gRPC 协议：BFE 在转发前把请求（含 header、cluster 名）发给 EPP，EPP 返回选中的后端实例地址，BFE 按该地址转发，并在响应阶段把响应 header / body 回传给 EPP 用于 token 计量。

### 1.2 目标

1. cluster 粒度启用：`cluster_conf.data` 中 `BalanceMode = "EPP"` 的 cluster 走 EPP 调度，其余 cluster 行为不变；
2. 请求携带 cluster 名（pool metadata），供 EPP demux 路由到对应调度 Cell；
3. `EPPAddr` 有序主备消费：健康检查、滞回 failover / failback、错误驱动的单请求重试；
4. gRPC 客户端生产化：连接复用、TLS 证书校验、超时、熔断；
5. 响应 body 完整回传（不丢块），保证 EPP 侧计量准确；
6. 可观测：cluster 维度的 EPP 调用、failover、降级、熔断指标。

### 1.3 非目标

- 不实现 P2 的"派发标记精确重试"（EPP 在派发后于响应 metadata 打标记，BFE 只对无标记请求重试），待 EPP 侧定义标记契约后另行实施；
- 不改动既有 WRR / WLC 均衡路径。

## 2. 术语定义

| 术语 | 定义 |
|------|------|
| EPP | Endpoint Picker，基于 ext-proc 协议的外部调度器（本文指 ai-gateway-epp）。 |
| ext-proc | Envoy 外部处理 gRPC 协议（`envoy.service.ext_proc.v3`），BFE 与 EPP 之间的对接协议。 |
| pool / cluster | 推理实例池，与 BFE 的 cluster 一一对应；BFE 在 ext-proc metadata 中以 cluster 名标识。 |
| Cell | EPP 内部为某个 cluster 建立的调度单元；主 EPP 为服务态，备 EPP 为热数据冷准入态。 |
| 主 / 备 EPP | `EPPAddr` 为有序列表：`[0]` 为主、`[1]` 为备（同实例组另一实例），由 ai-gateway-api 下发。 |
| failover | 活跃 EPP 地址故障后切换到下一地址。 |
| failback | 更高优先级地址恢复健康后切回。 |
| 降级 | EPP 全部不可用时回退 BFE 本地均衡（WRR），服务不中断、失去智能调度。 |

## 3. 总体架构

### 3.1 在 BFE 中的位置

```
客户端请求
   │
   ▼
bfe_server / reverseproxy.go
   │  BalanceMode == EPP ?
   ├─ 是 ──► bal_gslb.BalanceEpp()
   │           │  chooseBackendFromEPP()
   │           │    ├─ eppRuntime：有序地址表 + 活跃索引 + 健康检查状态机
   │           │    ├─ eppBreaker：滑动窗口熔断
   │           │    └─ epp.EppClient：ext-proc stream（经 eppRuntime 管理的 ClientConn）
   │           │         RequestHeaders(+MetadataContext: llm-d.ai/inference-pool)
   │           │              │
   │           │              ▼
   │           │         EPP（ai-gateway-epp，主或备）
   │           │              │  dynamic_metadata: envoy.lb → x-gateway-destination-endpoint
   │           │              ▼
   │           │  本地 subcluster 后端表查地址 → BfeBackend
   │           ▼
   │        转发到选中后端
   │           │
   │        响应阶段：ProcRespHeader + EppResponseBodyFilter 流式回传 EPP
   │
   └─ 否 ──► bal_gslb.Balance()（既有 WRR/WLC 路径，不变）
```

### 3.2 核心组件

| 组件 | 位置 | 职责 |
|------|------|------|
| `eppRuntime` | `bfe_balance/bal_gslb/epp_runtime.go` | 每 cluster 一个：有序地址表、活跃索引、gRPC 长连接池、后台健康检查、滞回 failover/failback |
| `eppBreaker` | `bfe_balance/bal_gslb/epp_breaker.go` | cluster 级滑动窗口熔断（OPEN / HALF-OPEN / CLOSED） |
| `EppClient` | `bfe_util/epp/epp_client.go` | ext-proc 消息收发、响应 header/body 回传、响应 body 过滤器 |
| 配置加载 | `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `GslbBasic` 新增 `BalanceMode=EPP`、`EPPAddr`、`EPPCheck`、`EPPTimeout`、`EPPTLS`、`EPPBreaker` |
| 指标 | `bfe_balance/bal_gslb/epp_metrics.go` | cluster 维度 prometheus 指标，经 `/monitor/epp_metrics` 输出 |

每个 cluster 的 `BalanceGslb` 独立持有自己的 `eppRuntime` 与 `eppBreaker`：不同 cluster 的 EPP 地址、健康状态、熔断状态互不影响——这与"每 cluster 独立主备"的部署模型一致。

## 4. 详细设计

### 4.1 协议契约（与 ai-gateway-epp 的对接约定）

**pool 名注入（请求方向）**：首个 ext-proc 消息（RequestHeaders）的 `ProcessingRequest.MetadataContext.FilterMetadata` 携带：

```
"llm-d.ai" : { "inference-pool": "<BFE cluster 名>" }
```

- pool 名取 `bal.name`（`NewBalanceGslb(name)` 时确定，即 cluster 名）；
- EPP demux 只从首个消息提取，RequestBody 消息不要求重复携带；
- 缺失时 EPP 若有 `defaultPool` 配置则兜底，否则返回 `Internal / missing inference-pool metadata`。

**调度结果返回（响应方向）**：EPP 在响应 `dynamic_metadata` 的 `envoy.lb → x-gateway-destination-endpoint` 中给出选中端点地址。

**错误语义与 BFE 行为**：

| gRPC 错误 | 含义 | BFE 行为 |
|---|---|---|
| `Internal / missing inference-pool metadata` | 本侧未注入 metadata | 视为 BFE 缺陷，不重试 |
| `Internal / unknown inference pool` | EPP 无此 cluster 的 Cell | 不在本实例重试；按 4.2 在同 cluster 下一 EPP 地址上重试该请求 |
| `Unavailable / cell is not serving` | Cell 处于备/未就绪（draining） | 可在同 cluster 的下一 EPP 地址（备实例）上重试该请求 |

### 4.2 有序地址表与滞回 failover

`eppRuntime`（`epp_runtime.go`）为每个 cluster 维护：

- **有序地址表与活跃索引**：`eppAddrs []string` 全量保存，`active` 为当前活跃索引，初始 0（主）；
- **每地址一条 gRPC 长连接**：`grpc.ClientConn` 在 `newEPPRuntime` 时按 `EPPTimeout.Connect` 超时建立，ext-proc stream 按请求创建（协议要求每请求一流），多路复用同一条连接；
- **后台健康检查**：每地址一个探活循环（`healthLoop` / `probe`），用 gRPC health（`grpc.health.v1.Health/Check`）按 `EPPCheck.CheckInterval` 周期探测，与 EPP 侧就绪门控同源；
- **failover**：活跃地址连续失败 `EPPCheck.FailThreshold` 次 → 切到下一可用地址，该地址进入 `EPPCheck.Cooldown` 冷却期，期内不回切（防 flapping）；
- **failback**：冷却期后，更高优先级地址需连续成功 `EPPCheck.SuccessThreshold` 次才切回；
- **错误驱动的即时重试**：`isEPPRetryable`（`bal_gslb.go:444`）判定 `unknown inference pool` 与 `cell is not serving` 时，不等健康检查周期，直接在同一请求的下一地址上重试；该重试不改变健康检查状态机，只影响单请求路径；
- **全部不可用**：`BalanceEpp` 返回错误，`reverseproxy.go` 回退本地 `Balance()`（WRR），并计入降级指标。

### 4.3 熔断

`eppBreaker`（`epp_breaker.go`）为 cluster 级、与地址级 failover 互补：

- 滑动窗口（最近调用结果环形缓冲）大小 `WindowSize`，窗口内调用量达到 `MinVolume` 后才评估错误率；
- 错误率 ≥ `ErrorRatePercent` → OPEN：此后 `BalanceEpp` 在调用 EPP 前直接短路返回错误，走本地均衡降级，不再发出 EPP 调用；
- OPEN 持续 `OpenTimeout` 后转 HALF-OPEN 放行探测；探测成功 → CLOSED 并清空窗口，失败 → 重新 OPEN；
- 每次调用结果（成功/失败，含超时与传输错误）经 `breaker.record()` 记入窗口。

### 4.4 请求路径

`reverseproxy.go` 转发前（`:345-355` 附近）：

1. `BalanceMode == EPP` 时调 `bal.BalanceEpp(request)`；
2. 重试场景不复位 EPP client（连接复用，不再关旧建新）；
3. `BalanceEpp` 内：`RetryTime > RetryMax` 截断（`RetryMax` 在 EPP 模式同样生效）；熔断短路检查；`chooseBackendFromEPP` 选地址、建 stream、发送 RequestHeaders（含 pool metadata）、等待首响应取 `x-gateway-destination-endpoint`；
4. 拿到地址后在本地 subcluster 后端表 `LookUpBackend`：找到则复用既有 `BfeBackend`（含健康检查、连接池），未找到则以地址构造临时 backend（`EPP_temp`）并记日志；
5. 选中的 `EppClient` 存入请求上下文 `REQ_CTX_EPP`，随请求贯穿转发与响应阶段。

### 4.5 响应回传路径

`reverseproxy.go` 响应阶段（`:915-921` / h2 路径 `:1368-1372`）：

1. `ProcRespHeader(res.Header, false)` 回传响应头；
2. `NewEppResponseBodyFilter(res.Body, eppClient)` 包装响应 body，转发路径 `Read` 的同时把数据块经 `ProcRespBody` 流式回传 EPP；
3. **完整性保证**：`ProcRespBody` 有界缓冲（可配字节预算 `WithRespBodyBufBudget`）满时不丢块——溢出采取"整流放弃"策略（`abortRespBody` 跳过该流剩余 body 并打点），**宁可整流放弃，不静默部分丢失**；EndOfStream 消息在流收尾时无条件送达，保证 EPP 侧流终结、计量不丢。

### 4.6 配置

配置入口为 `cluster_conf.data` 每 cluster 的 `GslbBasic` 节（详见 [cluster_conf.data 配置说明](../configuration/server_data_conf/cluster_conf.data.md) §6/§6.1），由 ai-gateway-api 经 server_data_conf 接口下发，**热加载生效，无需重启**。

| 字段 | 说明 |
|------|------|
| `BalanceMode` | `"EPP"` 时启用；缺省 WRR，EPP 字段被忽略 |
| `EPPAddr` | EPP 地址有序列表：`[0]`=主、`[1]`=备；必填非空、元素 `host:port`、列表内不允许重复 |
| `EPPCheck` | 健康检查与滞回：`Disabled` / `CheckInterval`(默认 2s) / `FailThreshold`(3) / `Cooldown`(45s) / `SuccessThreshold`(2) |
| `EPPTimeout` | `Connect`(500ms，建连) / `Call`(3s，首消息往返) |
| `EPPTLS` | `Insecure` / `CAFile`；缺省时保持 TLS 但跳过证书校验（兼容旧部署）；非 Insecure 时 CAFile 必填且加载时校验可读 |
| `EPPBreaker` | `Disabled` / `WindowSize`(100) / `MinVolume`(20) / `ErrorRatePercent`(50) / `OpenTimeout`(30s) |

**热加载行为**：`SetGslbBasic` 按地址表对比平滑切换——新请求按新地址表路由，活跃索引尽量对齐（`[0]` 未变则保持现状与 failover 状态）；`EPPCheck`/`EPPTimeout` 变化在下一检查周期/下一次建连生效；`BalanceMode` 在 EPP 与非 EPP 间切换时分别 `initEPP` / `closeEPP`。**配置校验失败（格式错误、主备同地址、CAFile 缺失）→ 本次 reload 拒绝并保留旧配置生效**，不静默降级。

**与既有字段的关系**：`RetryMax` 在 EPP 模式约束重试；`CrossRetry` 在 EPP 路径不生效（EPP 失败即回退本地，不做跨 sub-cluster 重试）；`HashConf` 在 EPP 模式不生效（后端由 EPP 决策），保留便于模式回切。

### 4.7 可观测

**带 label 指标**（prometheus text 格式，端点 `/monitor/epp_metrics`）：

| 指标 | 类型 | labels | 含义 |
|------|------|--------|------|
| `epp_calls_total` | Counter | cluster, result | EPP 调用数；result ∈ ok / no_pool / unknown_pool / draining / transport |
| `epp_failover_total` | Counter | cluster | failover 次数 |
| `epp_failback_total` | Counter | cluster | failback 次数 |
| `epp_fallback_local_total` | Counter | cluster | 降级本地均衡次数 |
| `epp_breaker_transitions_total` | Counter | cluster, state | 熔断状态变化（open/half_open/closed） |
| `epp_active_addr_index` | Gauge | cluster | 当前活跃地址索引 |

**扁平 counter**（既有状态页反射机制，`BalErrState` 扩展）：`ErrEppFailover` / `ErrEppFailback` / `ErrEppFallbackLocal` / `ErrEppBreakerOpen/HalfOpen/Closed`，经 `/monitor/bal_state` 输出，无 label 能力，仅作状态页兜底。

## 5. 降级与兼容性

| 场景 | 行为 |
|------|------|
| 活跃 EPP 故障 | 滞回 failover 到备地址；单请求级错误即时重试下一地址 |
| 全部 EPP 地址不可用 / 熔断 OPEN | 回退本地 WRR 均衡，服务不中断、失去智能调度，恢复后自动回归 EPP |
| 单元素 `EPPAddr`（无备） | 合法（测试/单实例组），实例故障走本地均衡降级 |
| 非 EPP cluster | 行为完全不变 |
| 旧 `cluster_conf.data`（无 EPP 字段） | 全部新字段可选带缺省，无需修改即可加载 |
| 响应 body 回传溢出 | 整流放弃该流 body 并打点，响应转发本身不受影响 |

## 6. 关键文件索引

| 文件 | 内容 |
|------|------|
| `bfe_balance/bal_gslb/epp_runtime.go` | 有序地址表、健康检查、滞回 failover/failback |
| `bfe_balance/bal_gslb/epp_breaker.go` | cluster 级滑动窗口熔断 |
| `bfe_balance/bal_gslb/bal_gslb.go` | `BalanceEpp` 主流程、`chooseBackendFromEPP`（pool metadata 注入）、`isEPPRetryable` |
| `bfe_util/epp/epp_client.go` | ext-proc 客户端、TLS、超时、响应回传与 body 过滤器 |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `GslbBasic` EPP 配置字段与校验 |
| `bfe_server/reverseproxy.go` | 请求侧接入（`BalanceEpp` + 失败回退）与响应侧回传 |
| `bfe_balance/bal_gslb/epp_metrics.go` | cluster 维度 prometheus 指标 |
| `bfe_server/web_server.go:74` | `/monitor/epp_metrics` 端点注册 |

关联文档：[cluster_conf.data 配置说明](../configuration/server_data_conf/cluster_conf.data.md) §6.1。
