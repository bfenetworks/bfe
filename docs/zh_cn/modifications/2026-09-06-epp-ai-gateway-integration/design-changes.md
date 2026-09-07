# BFE 对接 ai-gateway-epp 改造方案（ext-proc 对接生产化）

## 1. 背景与目标

本 fork 已实现初版 EPP ext-proc 集成：BFE cluster 与 EPP 地址一一对应（`cluster_conf.data` 中 `BalanceMode=EPP` + `EPPAddr`），对应"一 pool 一 EPP 进程"的原版 llm-d 模型。调度架构正在演进为 **ai-gateway-epp**（多 cluster 单进程调度器），与之配套的还有 ai-gateway-api 的实例组主备分配（见《EPP 主备池化部署方案》）。BFE 侧需要一轮改造才能对接新架构并达到生产可用。

改造目标（按优先级）：

1. **请求携带 cluster 名**：ai-gateway-epp 的 demux 按 ext-proc metadata 中的 pool 名把流路由到对应 Cell，BFE 必须在首个 ext-proc 消息中注入。
2. **`EPPAddr` 按序主备消费 + 滞回 failover**：当前只用 `addrs[0]`，没有故障转移。
3. **修复响应 body 静默丢块**：`ProcRespBody` 在 channel 满时丢弃数据块，导致 EPP 侧 token 计量偏差。
4. **gRPC 客户端生产化**：连接复用、TLS 证书校验、超时、熔断。
5. **可观测**：EPP 调用成功率、failover、降级等指标接入 bfe monitor。

## 2. 现状盘点

| 位置 | 内容 | 问题 |
|---|---|---|
| `bfe_util/epp/epp_client.go` | ext-proc gRPC 客户端、`EppResponseBodyFilter` 响应回传 | TLS `InsecureSkipVerify`；`ProcRespBody` 丢块（:183-190）；每请求新建 stream |
| `bfe_balance/bal_gslb/bal_gslb.go:124-168` | `initEPP`/`closeEPP` 连接生命周期 | 只用 `addrs[0]`（:147），注释掉的 `NewClient` 池化雏形未落地 |
| `bal_gslb.go:173-277` | `chooseBackendFromEPP` 构造并发送 RequestHeaders/RequestBody | **不携带 pool metadata**（`:206` 处 `MetadataContext` 被注释掉）；无超时 |
| `bal_gslb.go:492-528` | `BalanceEpp`：EPP 决策 → 查本地后端表 → 临时 backend | EPP 失败返回 error，由调用方回退本地均衡 |
| `bfe_server/reverseproxy.go:345-355` | 请求侧接入：EPP 模式调 `BalanceEpp`，失败回退 `Balance()` | 重试时会关闭旧 EPP client 重建（连接不复用） |
| `bfe_server/reverseproxy.go:915-921` | 响应侧回传：ResponseHeaders + `EppResponseBodyFilter` 流式回传 | 依赖被修复前的 `ProcRespBody` |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:67,413,765` | `BalanceModeEPP` 枚举、`EPPAddr []string` 及非空校验 | 配置结构已就绪，缺的只是消费逻辑 |

## 3. 对接契约（ai-gateway-epp 侧已定）

以下契约来自 ai-gateway-epp 的实现，BFE 侧按此改造：

**pool 名注入**（必需）：首个消息（RequestHeaders）的 `ProcessingRequest.MetadataContext.FilterMetadata` 中携带：

```
"llm-d.ai" : { "inference-pool": "<BFE cluster 名>" }
```

- 提取逻辑：`ai-gateway-epp/pkg/demux/server.go` `extractPool`（经 `llmdenvoy.ExtractMetadataValues`）。
- 缺失时：若 EPP 配置了 `defaultPool` 则兜底，否则返回 `Internal / missing inference-pool metadata`。
- 参考实现：`ai-gateway-epp/test/common/extproc_client.go:56-77`。

**调度结果返回**（已有实现，保持不变）：响应 `dynamic_metadata` 中 `envoy.lb → x-gateway-destination-endpoint` 给出选中端点地址。

**错误语义**（BFE 据此决定 failover / 回退）：

| gRPC 错误 | 含义 | BFE 行为 |
|---|---|---|
| `Internal / missing inference-pool metadata` | 本侧未注入 metadata | 视为 BFE 缺陷，修复 4.1；不重试 |
| `Internal / unknown inference pool` | EPP 无此 cluster 的 Cell | 不回重试本实例；记录日志；按 4.2 尝试同 cluster 下一 EPP 地址 |
| `Unavailable / cell is not serving` | Cell 处于备/未就绪（draining） | **可在同 cluster 的下一 EPP 地址（备实例）上重试** |

**主备地址来源**：`EPPAddr` 为有序列表，`[0]`=主、`[1]`=备，由 ai-gateway-api 经 server_data_conf 下发，语义见《server-data-conf 修改方案：GslbBasic.EPPAddr 字段语义定义》。

## 4. 改造点详细设计

### 4.1 注入 pool metadata

`bal_gslb.go` `chooseBackendFromEPP` 构造首个 `ProcessingRequest` 时填 `MetadataContext`：

```go
md, _ := structpb.NewStruct(map[string]any{"inference-pool": bal.name})
reqMsg := &extprocv3.ProcessingRequest{
    Request: &extprocv3.ProcessingRequest_RequestHeaders{
        RequestHeaders: epp.BuildEnvoyGRPCHeaders(req.OutRequest.Header, true, !hasBody),
    },
    MetadataContext: &corev3.Metadata{
        FilterMetadata: map[string]*structpb.Struct{"llm-d.ai": md},
    },
}
```

- pool 名取 `bal.name`（即 cluster 名，`NewBalanceGslb(name)` 时确定），无需依赖模块上下文。
- RequestBody 消息不要求重复携带（demux 只从首个消息提取），但带上无害。
- 需要新增 `structpb` / `corev3`  import（go-control-plane 已在依赖中）。

### 4.2 `EPPAddr` 按序主备消费 + 滞回 failover

改造 `initEPP` / 新增 EPP 实例选择层：

- **有序地址表**：`eppAddrs []string` 全量保存（已有），新增 `eppActive int` 当前活跃索引，初始 0。
- **健康检查**：每个 EPP 地址一个后台 goroutine（挂在 `BalanceGslb` 生命周期上，随 `closeEPP` 停止），用 gRPC health（`grpc.health.v1.Health/Check`）周期探活，与 EPP 侧就绪门控同源。
- **切换（failover）**：活跃地址连续失败 N 次（建议 3，可配）→ 切到下一可用地址；**切走后进入冷却期（默认 30~60s，可配），期内不切回**。
- **回切（failback）**：冷却期后，仅当更高优先级地址连续通过 M 次健康检查才切回。
- **全部不可用**：保持现有降级语义——`BalanceEpp` 失败 → `reverseproxy.go:353-355` 回退本地 `Balance()`（WRR），服务不中断、失去智能调度。
- **错误驱动的即时切换**：对 `Unavailable / cell is not serving` 和 `unknown inference pool`，不必等健康检查周期，可直接在下一地址重试该请求（注意与滞回协调：错误重试不改变健康检查状态机，只影响单请求路径）。
- **配置热更新**：`SetGslbBasic` 已在新配置时 `closeEPP` + `initEPP`，改为地址表对比后热切换活跃索引（低优先级地址变化不影响在飞请求）。

### 4.3 修复响应 body 丢块

`epp_client.go:183-190` `ProcRespBody` 的 channel 满即丢弃，导致 EPP 收到的响应 body 不完整、token 计量偏差：

- 改为带取消的阻塞发送： `select { case c.datach <- d: case <-c.ctx.Done(): }`，或扩大缓冲并按字节数（而非块数）限流。
- **必须保证 EndOfStream 一定送达**：`ProcRespHeader` 的收尾 goroutine 在 `datach` 关闭后无条件发送 EOS 消息（现状已如此，改造时保持该不变量），否则 EPP 侧流永不终结、计量丢失。
- 背压策略：EPP 消费慢时阻塞响应转发不可接受，故采用"有界缓冲 + 溢出计数打点（见 4.6）+ 溢出后跳过该流剩余 body 并记日志"——宁可整流放弃，不要静默部分丢失。

### 4.4 gRPC 客户端生产化

- **连接复用**：每 EPP 地址维护一个长连接 `ClientConn`（grpc 多路复用），stream 仍按请求建立（ext-proc 语义要求每请求一流）；替换 `reverseproxy.go:346-349` 重试即关 client 的逻辑——重试复用同连接建新 stream。
- **TLS**：`InsecureSkipVerify` 改为可配 CA 证书（cluster_conf 或全局配置），测试环境保留 insecure 开关。
- **超时**：连接建立、stream 首消息（RequestHeaders 的 Send/Recv）加 deadline；响应回传路径（`ProcRespHeader` goroutine）不阻塞主转发路径的前提下自带生命周期。
- **熔断**：滑动窗口错误率超阈值时短路（直接走本地均衡），半开探测恢复；与 4.2 的地址级 failover 互补（熔断是全局，failover 是地址级）。

### 4.5 配置项：`cluster_conf.data` 的 `GslbBasic` 扩展

#### 4.5.1 变更概览

- **变更位置**：`cluster_conf.data` 中每个 cluster 的 `GslbBasic` 节。该文件由 ai-gateway-api 经 server_data_conf 接口下发，BFE 侧消费入口为 `bfe_server/bfe_confdata_load.go:150` → `BalTable.SetGslbBasic`（`bal_table.go:173`）→ `BalanceGslb.SetGslbBasic`（`bal_gslb.go:88-111`），**热加载生效，无需重启**。
- **结构改动点**：`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:407-414` 的 `GslbBasicConf` 结构体新增可选字段（P0：`EPPCheck` / `EPPTimeout` / `EPPTLS`；P1：`EPPBreaker`）；`EPPAddr` 字段不变、仅语义收紧为有序主备。
- **兼容性**：全部新字段可选且带缺省值；旧 `cluster_conf.data`（无 `BalanceMode`、无 EPP 字段）加载行为完全不变。

#### 4.5.2 配置示例

非 EPP cluster（现状，不变）：

```json
"my-cluster": {
    "BackendConf":  { "...": "..." },
    "CheckConf":    { "...": "..." },
    "GslbBasic": {
        "CrossRetry": 0,
        "RetryMax": 2,
        "HashConf": {
            "HashStrategy": 0,
            "HashHeader": "Cookie:UID",
            "SessionSticky": false
        }
    }
}
```

EPP 模式 cluster（新字段全量展示）：

```json
"cluster-a": {
    "BackendConf":  { "...": "..." },
    "CheckConf":    { "...": "..." },
    "GslbBasic": {
        "CrossRetry": 0,
        "RetryMax": 2,
        "HashConf": {
            "HashStrategy": 0,
            "HashHeader": "Cookie:UID",
            "SessionSticky": false
        },
        "BalanceMode": "EPP",
        "EPPAddr": [
            "10.0.0.1:9002",
            "10.0.0.2:9002"
        ],
        "EPPCheck": {
            "CheckInterval":  "2s",
            "FailThreshold":  3,
            "Cooldown":       "45s",
            "SuccessThreshold": 2
        },
        "EPPTimeout": {
            "Connect": "500ms",
            "Call":    "3s"
        },
        "EPPTLS": {
            "Insecure": false,
            "CAFile": "/bfe/conf/epp/epp_ca.crt"
        },
        "EPPBreaker": {
            "WindowSize":       100,
            "MinVolume":        20,
            "ErrorRatePercent": 50,
            "OpenTimeout":      "30s"
        }
    }
}
```

#### 4.5.3 字段定义

**`BalanceMode`（已有字段，语义不变）**

| 项 | 值 |
|---|---|
| 类型 | `*string` |
| 取值 | `"WRR"`（缺省）/ `"WLC"` / 其它既有模式 / `"EPP"` |
| 说明 | 为 `"EPP"` 时本 cluster 走 ext-proc 调度（`cluster_conf_load.go:67`）；缺省或省略时为 WRR，EPP 相关字段被忽略 |

**`EPPAddr`（已有字段，语义收紧为有序主备）**

| 项 | 值 |
|---|---|
| 类型 | `*[]string`，元素 `host:port` |
| 元素语义 | `[0]` = 主 EPP；`[1]` = 备 EPP（同实例组另一实例）；`>2` 为扩容预留，按序消费 |
| 必填 | `BalanceMode = "EPP"` 时非空（既有校验 `cluster_conf_load.go:765-767`，保留） |
| 新增校验 | 元素格式必须为 `host:port`；列表内去重（主备同地址为配置错误，加载拒绝） |
| 来源 | ai-gateway-api 按"实例组分配"派生下发，语义契约见《server-data-conf 修改方案：GslbBasic.EPPAddr 字段语义定义》 |
| 单元素 | 合法：无备实例（测试/单实例组），主故障时降级本地均衡 |

**`EPPCheck`（新增，健康检查与滞回参数）**

| 字段 | 类型 | 缺省 | 说明 |
|---|---|---|---|
| `Disabled` | bool | false | true 时关闭后台健康检查（仅依赖 §3 错误驱动的单请求重试；生产不建议） |
| `CheckInterval` | string（duration） | `"2s"` | 健康检查探活周期（gRPC health，`grpc.health.v1`） |
| `FailThreshold` | int | 3 | 活跃地址连续失败次数达到该值即 failover 到下一地址 |
| `Cooldown` | string（duration） | `"45s"` | failover 后冷却期，期内不回切（防 flapping，主备方案的硬要求） |
| `SuccessThreshold` | int | 2 | 冷却期后，更高优先级地址需连续成功该次数才 failback |

**`EPPTimeout`（新增，调用超时）**

| 字段 | 类型 | 缺省 | 说明 |
|---|---|---|---|
| `Connect` | string（duration） | `"500ms"` | 建立 gRPC 连接/stream 的超时 |
| `Call` | string（duration） | `"3s"` | 首消息（RequestHeaders 的 Send+Recv 往返）超时；超时即视为本次 EPP 调用失败，走 §4.2 的失败路径 |

超时值应显著小于客户端可感知延迟：failover/回退本地均衡的前提是**快速失败**。

**`EPPTLS`（新增，传输安全）**

| 字段 | 类型 | 缺省 | 说明 |
|---|---|---|---|
| `Insecure` | bool | false | true = 跳过证书校验（等价现状行为，仅测试环境）；false = 必须用 `CAFile` 校验 |
| `CAFile` | string | 空 | EPP 服务端 CA 证书路径；`Insecure=false` 时必填（加载时校验文件可读）。证书文件可经 server_data_conf 的 extra_files 通道随配置下发到固定路径 |

**`EPPBreaker`（P1 新增，EPP 路径熔断）**

| 字段 | 类型 | 缺省 | 说明 |
|---|---|---|---|
| `Disabled` | bool | false | true 时关闭熔断（仅依赖地址级 failover；生产不建议） |
| `WindowSize` | int | 100 | 滑动窗口（最近调用结果环形缓冲）大小 |
| `MinVolume` | int | 20 | 窗口内最小调用量，达到后才评估错误率；必须 ≤ WindowSize |
| `ErrorRatePercent` | int | 50 | 错误率阈值（百分比，1~100），窗口错误率 ≥ 该值 → OPEN |
| `OpenTimeout` | string（duration） | `"30s"` | OPEN 持续时长，之后转 HALF-OPEN 放行探测；探测成功 → CLOSED 并清空窗口，失败 → 重新 OPEN |

熔断是 cluster 级、与地址级 failover 互补：failover 换地址，熔断整体停走 EPP（BalanceEpp 直接返回错误走既有本地均衡降级，不再发 EPP 调用）。窗口满或错误率达标即评估，阈值越界即 OPEN。

#### 4.5.4 加载、校验与消费链路

1. ai-gateway-api 导出 server_data_conf，`EPPAddr` 等字段变化会使 Version 变化（MD5 签名机制），触发 BFE 拉取热加载。
2. BFE `bfe_confdata_load.go` 加载并校验 `cluster_conf.data`：**新增字段校验失败（格式、主备同地址、`Insecure=false` 但 `CAFile` 缺失/不可读）→ 本次 reload 报错拒绝**，保留旧配置生效——与既有 `EPPAddr` 非空校验的 fail-fast 风格一致，不静默降级。
3. `BalanceGslb.SetGslbBasic`（`bal_gslb.go:88-111`）按 `BalanceMode` 分发：`EPP` → 构建/刷新 §4.2 的 EPP 实例状态机（地址表 + 健康检查 + 连接）；非 EPP → `closeEPP`。

#### 4.5.5 热加载行为

| 变更内容 | 生效方式 |
|---|---|
| `EPPAddr` 元素变化或顺序变化 | 平滑切换：不中断在飞请求；新请求按新地址表路由。活跃索引尽量对齐（若 `[0]` 未变则保持现状与 failover 状态） |
| `EPPCheck` / `EPPTimeout` 变化 | 下一个检查周期 / 下一次新建连接时生效 |
| `BalanceMode`：EPP → 非 EPP | `closeEPP`，后续请求走本地均衡 |
| `BalanceMode`：非 EPP → EPP | `initEPP`，从 `[0]` 开始建立连接与健康检查 |

#### 4.5.6 与既有字段的关系

- `RetryMax` / `CrossRetry` 在 EPP 模式保留现有语义：`BalanceEpp` 中 `RetryTime > RetryMax` 仍截断重试（`bal_gslb.go:500-505`）；`CrossRetry` 在 EPP 路径不生效（EPP 失败即回退，不做跨 sub-cluster 重试）。
- `HashConf` 在 EPP 模式不生效（后端由 EPP 决策），保留在配置中无害，便于模式回切。

### 4.6 可观测

新增 monitor 计数（接入 `bfe_server` 状态页）：

- EPP 调用成功/失败数（按 cluster、按错误类别：no-pool / unknown-pool / draining / transport）
- failover / failback 次数、当前活跃地址索引
- 本地均衡降级次数
- 熔断状态变化次数（open→half-open→closed 各计）
- 响应 body 回传溢出次数（4.3 的兜底打点）

**实现与取舍（P1 落地时补充）**：cluster 维度指标采用 prometheus CounterVec/GaugeVec（`github.com/prometheus/client_golang`，仓库既有依赖，mod_ai_rate_limit 已有同模式先例），经 `/monitor/epp_metrics` 以 prometheus text 格式输出：`epp_calls_total{cluster,result}`（result ∈ ok/no_pool/unknown_pool/draining/transport）、`epp_failover_total{cluster}`、`epp_failback_total{cluster}`、`epp_fallback_local_total{cluster}`、`epp_breaker_transitions_total{cluster,state}`、`epp_active_addr_index{cluster}`（gauge）。同时保留一份扁平 counter 进既有状态页（`BalErrState` 扩展 `ErrEppFailover/ErrEppFailback/ErrEppFallbackLocal/ErrEppBreakerOpen/HalfOpen/Closed`，走 KP_PROXY_STATE 反射机制；`EppState.RespBodyOverflow` 不变）——扁平 counter 无 label 能力，仅作状态页兜底，以带 label 指标为准。

## 5. 分期落地

| 期 | 内容 | 量级估计 |
|---|---|---|
| P0 | §4.1 metadata 注入 + §4.2 按序主备/滞回/健康检查 + §4.3 丢块修复 | ~1 周 |
| P1 | §4.4 客户端生产化（连接复用/TLS/超时/熔断）+ §4.6 指标 | ~1 周 |
| P2 | 派发标记驱动的精确重试（EPP 在派发后于响应 metadata 打标记，BFE 只对无标记请求重试）；双活跃告警对接 | 视运行情况 |

P0 完成即可支撑"2 实例组互为主备"的测试环境全链路验收：kill 主 EPP → BFE 滞回冷却后自动切备 → 该 cluster 请求成功率无持续跌落 → 恢复后不发生 failback 抖动 → 其余 cluster 无感。

## 6. 影响面与兼容性

- 不新增/调整模块，`bfe_modules.go` 顺序不变；改动集中在 `bfe_util/epp/`、`bal_gslb`、`reverseproxy`、`cluster_conf` 配置加载。
- 非 EPP 集群（WRR/WLC）行为完全不变。
- 现有"一 pool 一 EPP"部署形态仍兼容：单元素 `EPPAddr` 即无主备，实例故障走本地均衡降级。
- 配置新增字段均有缺省值，旧 `cluster_conf.data` 无需修改即可加载。

## 7. 关键文件索引

| 文件 | 改造点 |
|---|---|
| `bfe_balance/bal_gslb/bal_gslb.go:173-207` | §4.1 metadata 注入 |
| `bfe_balance/bal_gslb/bal_gslb.go:124-168` | §4.2 有序地址表 + 健康检查 + 滞回 |
| `bfe_util/epp/epp_client.go:42-57,183-190` | §4.4 TLS/连接、§4.3 丢块修复 |
| `bfe_server/reverseproxy.go:345-355` | 重试路径连接复用 |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | §4.5 新配置字段 |
| `ai-gateway-epp/pkg/demux/server.go`（外部仓库） | EPP 侧契约：pool 提取与错误语义 |
| `ai-gateway-epp/test/common/extproc_client.go`（外部仓库） | metadata 注入参考实现 |
