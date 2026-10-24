# 流量镜像（mod_traffic_mirror）系统设计

## 1. 背景与目标

### 1.1 背景

AI 网关在将请求正常转发给生产上游的同时，需要把一份完整副本（方法、路径、Header、Body，可按规则改写）异步发送给镜像目标集群，用于发布前的双跑验证。镜像目标的处理结果对客户端完全不可见——响应完整读空后丢弃，仅用于统计与验证。

与通用 L7 流量镜像不同，镜像对象是"一次模型推理调用"而非"一次 HTTP 事务"，因此产生若干 AI 特有诉求：

- SSE 流式响应必须完整读空至 `[DONE]`（半途断开会使镜像集群取消推理、统计不到 usage）；
- 读空过程需轻量解析 `usage` / `error` / `finish_reason`（"有没有报错"要细到 `rate_limit_exceeded` 而非只有 502）；
- 规则需支持改写 body 的 `model` 字段（生产走模型 A、镜像到部署模型 B 的集群做双跑验证）；
- 分层超时（连接建立 / 首字节 TTFT / 总时长），总时长默认匹配分钟级长推理；
- TTFT / 吞吐指标与镜像 token 成本统计。

### 1.2 目标

1. 新增 `mod_traffic_mirror` 模块：规则匹配 + 百分比采样 + model 改写 + 镜像目标 cluster + SSE 读空与 AI 语义统计；
2. 主链路零影响：镜像动作 = 一次规则匹配 + 一次 body 拷贝 + 一次非阻塞提交；
3. 复用现有 `cluster_table.data` 中定义的镜像目标集群（健康检查、负载均衡、后端超时）；
4. 访问日志与 Prometheus 全埋点，所有丢弃路径可观测；
5. 镜像流量与计费/鉴权/限流完全隔离，不进任何客户账单。

### 1.3 非目标（一期）

WebSocket 镜像、>8MB 大 Body 镜像（跳过并计数）、镜像响应 diff、L4 镜像、流量录制回放。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新模块 | 新建 `mod_traffic_mirror`：HandleForward 触发镜像，同步快照 + 异步发送，SSE 读空丢弃 | `bfe/bfe_modules/mod_traffic_mirror/*.go`（新建） |
| 模块注册 | 在 `mod_ai_cache` 之后、`mod_body_process` 之前插入 `mod_traffic_mirror` | `bfe/bfe_modules/bfe_modules.go` |
| 负载均衡 | `BalanceGslb` 新增无副作用的 `PickBackend()`；`BalTable` 增加包级全局注册 | `bfe/bfe_balance/bal_gslb/bal_gslb.go`、`bfe/bfe_balance/bal_table.go` |
| 服务启动 | 启动时把 server 的 balTable 注册为全局 BalTable | `bfe/bfe_server/bfe_server.go` |
| 基础信息 | `AiBasicInfo` 新增镜像命中字段（同步字段，供访问日志） | `bfe/bfe_basic/request_ai_basic.go` |
| 访问日志 | protobuf 新增 `mirror_hit`(842) ~ `mirror_error`(850)，赋值逻辑 | `bfe/bfe_modules/mod_access_pb3/request_log.go`（pb 定义在 bfe-access-pb 仓库 v0.3.8） |
| 配置 | 新增模块配置与规则文件样例 | `conf/mod_traffic_mirror/`（新建） |
| 文档 | 模块配置文档 | `docs/zh_cn/configuration/mod_traffic_mirror/`（新建） |

## 3. 总体架构

```text
客户端请求
    │
    ▼
模块管线（auth → route → cache → body_process 改写 → ...）
    │
    ▼
clusterInvoke：选中后端
    │
    ├── HandleForward 回调 ──────────────────────────────┐
    │    mod_traffic_mirror:                              │
    │    1. ruleTable.Search(product).Match(req)          │
    │    2. CtxMirrored 已置位? → 跳过（fallback 去重）    │
    │    3. sampleHit(percentage) ?                       │
    │    4. GetBytes() 拷 body（all=false → 跳过计数）     │
    │    5. Header 裁剪 + body model 改写                  │
    │    6. 快照目标/Header/Body/超时，提交异步任务         │
    │    （主链路继续，零等待）                            │
    │                                                    ▼
    ▼                                            ┌─ 镜像 goroutine（不持 req）
transport.RoundTrip（主请求）                     │  7. breaker 检查（熔断中→丢弃计数）
    │                                            │  8. 分层超时 http.Client.Do
    ▼                                            │  9. 完整读空响应：
上游响应 → 客户端 ◀──（主链路不受影响）            │     SSE → 读到 [DONE]
                                                 │     解析 usage/error/finish_reason
                                                 │     字节/时长双上限
                                                 │ 10. 记 status/TTFB/latency/usage
                                                 │     → Prometheus，响应 io.Discard
                                                 └─
```

### 3.1 模块注册位置

`bfe/bfe_modules/bfe_modules.go`，注册在 `mod_ai_cache` 之后、`mod_body_process` 之前，仅注册一个 `HandleForward` 回调。

`HandleForward` 在 `clusterInvoke` 内、选中后端后触发，晚于所有 `HandleFoundProduct`（路由/鉴权）与 `HandleAfterLocation`（含 `mod_body_process` 请求改写）回调。因此**无论模块注册在 body_process 前后，镜像副本始终是"改写后、转发前"的请求**——即实际发往生产的同一份字节。

`mod_ai_cache` 命中短路时请求不会进入 `clusterInvoke`，自然不会被镜像，语义正确（缓存命中请求不代表真实上游行为）。

## 4. 核心设计决策

### 4.1 挂载点：`HandleForward`

`clusterInvoke` 选中后端后、构造转发请求并 `RoundTrip` 前触发 `HandleForward`。此时路由结果（`AiRouteResult`）、鉴权结果（`AiBasicInfo.ClientApiKey` 等）均已就绪，镜像规则可引用这些上下文做条件匹配；且请求体已缓冲（AI 模式下被包装为 bytes_body），可直接拷贝。

备选挂点不成立：`HandleAfterLocation` 缺少后端上下文；`HandleReadResponse` 太晚（请求已发出）。

### 4.2 每请求只镜像一次（fallback 重试去重）

`HandleForward` 在 fallback 重试时会被多次调用（`ServeHTTPForAI` 的 `aiClusterInvoke` 重试循环每次重试都重新走 `clusterInvoke`）。若不去重，同一客户端请求会被镜像多次，镜像集群 QPS 虚高、成本翻倍。

方案：镜像提交成功后置 once 标记 `req.SetContext(CtxMirrored, true)`；入口检查该标记，已存在则直接放行。采样未命中、规则未命中、body 超限跳过等路径不置位。

### 4.3 异步 goroutine 不持有 `req` 引用

`basicReq.SvrDataConf` 在 `clusterInvoke` 返回后被置 nil，goroutine 内禁止再访问。本方案约束**异步侧完全不引用 `*bfe_basic.Request`**：

- 目标地址、Header、Body、超时等在提交异步任务前**全部快照**进 `mirrorTask`（值拷贝）；
- 避免 goroutine 持有整个请求对象 10 分钟（总时长上限）导致内存滞留；
- 异步结果**不回写** `AiBasicInfo`（见 6.1 的时序说明），只进 Prometheus。

### 4.4 Body 来源与上限

- 直接 `req.HttpRequest.GetBodyAccessor().GetBytes()` 拷贝；
- `GetBytes()` 返回 `all=false`（body 超过可访问缓冲上限，默认 2MB、可配置至 8MB）时按排除规则跳过并计数（`mirror_skip_total{reason="body_limit"}`）；
- 拷贝为独立字节切片，与主链路解耦。

### 4.5 客户端断连处理

镜像与主链路完全解耦：客户端断连后主链路取消，但**镜像连接默认继续读完**（样本完整性优先）；由总时长上限兜底。实现上镜像 goroutine 不响应客户端 context，只受自身的 `TotalTimeoutMs` 约束。

### 4.6 计费/鉴权隔离

镜像是网关内部发出的 fire-and-forget 子请求：由模块内独立 `http.Client` 直接发往镜像后端，不进入 BFE 自身监听端口、不重建 `bfe_basic.Request`，整个模块管线（`mod_ai_token_auth` 配额扣减、`mod_ai_rate_limit` 限流）不会二次执行，镜像 token 消耗归属内部成本中心。

## 5. 镜像目标后端选择：`BalanceGslb.PickBackend()`

镜像请求要在**异步 goroutine 里**挑一个镜像目标后端。既有的 `Balance(req)` 是为"主链路转发"设计的，不能直接复用，为此在 `bfe/bfe_balance/bal_gslb/bal_gslb.go` 新增无副作用的 `PickBackend()`，并配套 `BalTable` 包级全局注册（`bfe/bfe_balance/bal_table.go` 的 `SetGlobalBalTable` / `GetGlobalBalTable`，由 `bfe_server/bfe_server.go` 启动时注册）。

### 5.1 为什么不能用 `Balance(req)`

**1. 副作用会污染主请求。** `Balance(req)` 大量读写主请求字段：

- 读写 `req.RetryTime`（重试上限判断）——镜像 goroutine 并发改它会和主链路互相干扰；
- 失败时设置 `req.ErrCode` / `req.ErrMsg`——镜像选不到后端会让主请求"看起来转发失败"；
- 写 `req.Backend.SubclusterName`、`req.Stat.IsCrossCluster`——篡改主请求的真实记录。

镜像只是旁路，选不到目标最多算镜像丢弃，绝不能影响主请求本身。`PickBackend()` 全程不碰 `req`。

**2. 会话亲和和 hash 在这里没有意义。** 主链路会根据 `req` 算 hash key、按 `SessionSticky` 绑定同一用户到同一后端。镜像请求是独立的一次探测，不应继承主请求的会话粘性/hash key——镜像流量应按集群权重均匀分布，所以 `PickBackend()` 直接传 `nil` hash key、固定用 `WrrSmooth`，跳过粘性逻辑。

**3. 重试语义不成立。** `Balance` 的跨集群重试依赖 `req.RetryTime` 递增和 `req.Stat` 标记。镜像 goroutine 里不该有"跨集群重试"，一次选不到就丢弃计数即可。

### 5.2 为什么需要从 BalTable 全局入口解析

镜像发生在 `HandleForward` 回调（反向代理之前），而 BFE 反向代理路径把 BalTable 挂在 `SvrDataConf` 上、在 `clusterInvoke` 之后就置 nil 了，异步 goroutine 拿不到。因此模块侧改为通过包级全局 `bal_table.GetGlobalBalTable()` 按 cluster 名查到 `BalanceGslb`，再调 `PickBackend()`。

另一点：`bal_gslb` 的平衡锁 `lock` 是私有的，模块侧无法自己安全地调 `subClusterBalance` + `balance`，必须在包内提供一个带锁入口——这是 `PickBackend()` 放在 `bal_gslb.go` 而非模块内的直接原因。

### 5.3 代价与取舍

`PickBackend()` 与 `Balance()` 共用同一把锁、都推进 WRR 平滑权重状态，理论上会让主链路的后端分布多一个极小扰动；换来的是镜像目标选择**复用既有的健康检查、黑名单、slowstart、子集群权重逻辑**，不需要模块自己维护一套后端列表，也不会和主链路产生不一致的可用性判断。

## 6. 访问日志与监控

### 6.1 访问日志字段

`bfe-access-pb`（v0.3.8）protobuf 新增字段（842-850 区间）：

| 编号 | 字段名 | 类型 | 说明 |
|---|---|---|---|
| 842 | `mirror_hit` | bool | 该请求是否被镜像（提交成功即记 true） |
| 843 | `mirror_cluster` | string | 镜像目标 cluster 名 |
| 844 | `mirror_status` | int32 | 保留位：镜像响应状态码。**一期异步结果不回写**，置空 |
| 845-850 | `mirror_latency_us` / `mirror_ttfb_us` / `mirror_prompt_tokens` / `mirror_completion_tokens` / `mirror_finish_reason` / `mirror_error` | - | 同上，一期保留为空 |

**异步结果不回写的设计决策**：镜像读空完成时间通常晚于 `HandleRequestFinish`（访问日志写出点），回写大多是无效写入；且 goroutine 持有 `req` 引用 10 分钟会造成内存滞留。因此访问日志只记录同步可得字段（`mirror_hit` / `mirror_cluster`），异步完成的 status/usage/TTFB 全部走 Prometheus 实时看板。

### 6.2 Prometheus 指标（`mirror_state.go`）

```text
mirror_req_total{product, cluster, model}        # 被镜像请求数（提交成功）
mirror_submit_drop_total                          # 队列/semaphore 满丢弃
mirror_skip_total{reason}                         # 采样未命中 / body超限 / 改写失败 / 熔断
mirror_resp_status_total{cluster, status}         # 镜像响应状态码分布
mirror_error_type_total{cluster, type, code}      # OpenAI error.type/code
mirror_finish_reason_total{cluster, reason}       # stop/length/content_filter/error
mirror_ttfb_ms_bucket{cluster}                    # 首字节延迟直方图（TTFT 近似）
mirror_latency_ms_bucket{cluster}                 # 总延迟直方图
mirror_tokens_total{cluster, kind}                # prompt/completion token 量
mirror_inflight{cluster}                          # 当前镜像并发
mirror_fail_total{cluster, reason}                # 发送/读空失败（熔断器消费）
mirror_circuit_open_total{cluster}                # 熔断丢弃数
mirror_resp_truncated_total{cluster}              # 响应读空被双上限截断数
```

module_state2 计数器与 Prometheus 指标并存（BFE 模块惯例）。

## 7. 非功能性设计

| 维度 | 设计 |
|---|---|
| 主链路性能 | 镜像动作 = 规则匹配 + body 拷贝 + 非阻塞提交，主链路零等待 |
| 主链路保护 | 镜像构建/提交失败静默吞掉只计数；所有丢弃路径（采样未命中/body 超限/改写失败/队列满/熔断）均有独立计数 |
| 故障隔离 | semaphore 硬上限 + 队列满丢弃 + 熔断冷却 + 分层超时，goroutine 堆积有界；镜像目标宕机不影响主请求 |
| 数据合规 | Header 黑名单默认剔除 Authorization/Cookie/X-Api-Key；镜像目标集群需与生产同等级访问控制与审计 |
| 成本控制 | 比例采样默认保守；`bodyRewrites` 可将 model 指到 mock/平价模型；token 成本指标归属内部成本中心 |

## 8. 风险与应对

| 风险 | 应对 |
|------|------|
| 生产数据复制到镜像集群（合规） | Header 黑名单剔除鉴权/会话头；低比例起步 + 合规评审前置；镜像目标集群同等级访问控制与审计 |
| 镜像流量产生真实模型成本 | 比例采样默认保守；镜像目标可配置为 mock server 或平价模型；token 成本指标 + 内部成本中心归属 |
| SSE 长连接占用镜像资源 | 独立并发池 + 双上限 + 丢弃/截断计数告警；容量按"峰值 QPS × 比例 × 平均流时长"评估 |
| 镜像风暴反压网关 | semaphore 硬上限 + 队列满丢弃 + 熔断冷却 + 分层超时 |
| fallback 重试导致重复镜像 | `CtxMirrored` once 标记保证每请求仅镜像一次 |
| SvrDataConf / req 生命周期 | 提交前快照全部信息，goroutine 不持 req 引用；异步结果不回写 AiBasicInfo |
