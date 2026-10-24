# 新增 `mod_traffic_mirror` 模块（流量镜像 / Shadow Traffic）

## 1. 背景

本文档是需求《流量镜像需求分析（AI 网关场景）》在 BFE 数据面的落地方案，与需求文档一一对应。

**功能定义**：网关在将请求正常转发给原上游的同时，**异步**地将该请求的一份完整副本（方法、路径、Header、Body，可按规则改写）发送给镜像目标集群；镜像目标的处理结果对客户端完全不可见（响应完整读空后丢弃），仅用于统计与验证。典型用途是发布前验证——"copy 过来，验证一下有没有问题，没问题再发布"。

**与通用 L7 镜像的本质差异**：镜像对象是"一次模型推理调用"而非"一次 HTTP 事务"，由此产生六个 AI 特有设计点（需求文档第 2 章）：

1. SSE 流式响应必须完整读空至 `[DONE]`（半途断开会使镜像集群取消推理、统计不到 usage）；
2. 读空过程轻量解析 `usage` / `error` / `finish_reason`（"有没有报错"要细到 `rate_limit_exceeded` 而非只有 502）；
3. 规则支持改写 body 的 `model` 字段（生产走模型 A、镜像到部署模型 B 的集群做双跑验证）；
4. 分层超时（连接建立 / 首字节 TTFT / 总时长，总时长默认匹配分钟级长推理）；
5. TTFT / 吞吐（output tokens/s）指标；
6. 镜像 token 成本统计（每次镜像 = 真实推理费用 + GPU 占用）。

**一期目标**：规则匹配 + 百分比采样 + model 改写 + 镜像目标 cluster + SSE 读空与 AI 语义统计 + Prometheus/访问日志埋点，主链路零影响。

**明确不做**（一期非目标）：WebSocket 镜像、>8MB 大 Body 镜像（跳过并计数）、镜像响应 diff、L4 镜像、流量录制回放。通用 HTTP 流量镜像能力内核支持，但一期验收只覆盖 AI/OpenAI 协议流量。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 新模块 | 新建 `mod_traffic_mirror`：HandleForward 触发镜像，同步快照 + 异步发送，SSE 读空丢弃 | `bfe/bfe_modules/mod_traffic_mirror/*.go`（新建） |
| 模块注册 | 在 `mod_ai_cache` 之后、`mod_body_process` 之前插入 `mod_traffic_mirror` | `bfe/bfe_modules/bfe_modules.go` |
| 基础信息 | `AiBasicInfo` 新增镜像命中相关字段（同步字段，供访问日志） | `bfe/bfe_basic/request_ai_basic.go` |
| 访问日志 | protobuf 新增 `mirror_hit` ~ `mirror_error`（建议 842-850 区间，以 proto 实际空闲编号为准） | `bfe-access-pb`（独立仓库）、`bfe/bfe_modules/mod_access_pb3/request_log.go` |
| 配置 | 新增模块配置与规则文件样例 | `bfe/conf/mod_traffic_mirror/mod_traffic_mirror.conf`、`mirror_rule.data`（新建） |
| 文档 | 模块配置文档 | `bfe/docs/zh_cn/configuration/mod_traffic_mirror/`（新建） |
| 测试 | 单元测试 + 集成测试 | `bfe/bfe_modules/mod_traffic_mirror/*_test.go`、`integration-test/`（独立仓库） |

配套变更（其他仓库，本文档不展开）：`ai-gateway-api` 规则 CRUD 与配置导出、`ai-gateway-web` 管理页、`ai-gateway-observability` Grafana 看板、Doris 报表"流量镜像"页签。

## 3. 模块注册位置

`bfe/bfe_modules/bfe_modules.go` 当前 AI 模块执行顺序：

```text
mod_ai_token_auth   // HandleFoundProduct：API Key 校验与配额计划绑定
mod_ai_route        // HandleFoundProduct：路由决策
mod_ai_cache        // HandleAfterLocation / HandleReadResponse：精确缓存
mod_body_process    // HandleAfterLocation / HandleReadResponse：请求改写、SSE usage 解析
mod_ai_rate_limit
mod_access_pb3
```

插入位置（after `mod_ai_cache`）：

```go
// mod_ai_cache
mod_ai_cache.NewModuleAiCache(),

// mod_traffic_mirror
// Requirement: after mod_ai_route / mod_ai_token_auth (mirror rules match on
// resolved AI context like model/apikey at HandleForward, which fires after
// all HandleFoundProduct / HandleAfterLocation callbacks); before
// mod_access_pb3 (AiBasicInfo mirror fields must be set before access logging)
mod_traffic_mirror.NewModuleTrafficMirror(),

// mod_body_process
mod_body_process.NewModuleBodyProcess(),
```

仅注册一个回调：

```go
cbs.AddFilter(bfe_module.HandleForward, m.mirrorHandler) // 镜像触发：快照 + 提交
```

> 注册顺序与 `HandleForward` 触发时机的关系：`HandleForward` 回调在 `clusterInvoke` 内、选中后端后触发（`bfe/bfe_server/reverseproxy.go:379-389`），晚于所有 `HandleFoundProduct`（路由/鉴权）与 `HandleAfterLocation`（含 `mod_body_process` 请求改写）回调。因此**无论模块注册在 body_process 前后，镜像副本始终是"改写后、转发前"的请求**——即实际发往生产的同一份字节（需求 8.1 节顺序注记，一期不提供"改写前镜像"开关）。
>
> `mod_ai_cache` 命中短路时请求不会进入 `clusterInvoke`，自然不会被镜像，语义正确（缓存命中请求不代表真实上游行为）。

## 4. 挂载点与关键生命周期约束

### 4.1 挂载点：`HandleForward`

`clusterInvoke`（`bfe/bfe_server/reverseproxy.go`）选中后端后、构造转发请求并 `RoundTrip` 前，触发 `HandleForward` 回调（`reverseproxy.go:379-389`）。此时：

- 集群/后端已定（`request.Trans.Backend` 已就绪），路由结果（`AiRouteResult`）、鉴权结果（`AiBasicInfo.ClientApiKey` 等）均已就绪，镜像规则可引用这些上下文做条件匹配；
- 请求体已缓冲（AI 模式下进入管线前 body 已被包装为 bytes_body），可直接拷贝。

备选挂点不成立：`HandleAfterLocation` 缺少后端上下文；`HandleReadResponse` 太晚（请求已发出，body 需重新构造）。

### 4.2 每请求只镜像一次（fallback 重试去重）

`HandleForward` 在 fallback 重试时会被多次调用：`ServeHTTPForAI` 的 `aiClusterInvoke` 重试循环（`reverseproxy.go:1309` 附近）每次重试都会重新走 `clusterInvoke`。若不去重，同一客户端请求会被镜像多次，镜像集群 QPS 虚高、成本翻倍。

**方案**：镜像提交成功后置 once 标记 `req.SetContext(CtxMirrored, true)`；`mirrorHandler` 入口检查该标记，已存在则直接放行。只在"确认提交（含队列满丢弃前的尝试）"后置位；采样未命中、规则未命中、body 超限跳过等路径不置位（这些路径本来就不产生镜像流量，置不置位无差异，统一不置位语义更简单）。

### 4.3 异步 goroutine 不持有 `req` 引用

`basicReq.SvrDataConf` 在 `clusterInvoke` 返回后被置 nil（`reverseproxy.go:884`，AI 重试路径 `:1350`），goroutine 内禁止再访问。更进一步，本方案约束**异步侧完全不引用 `*bfe_basic.Request`**：

- 目标地址、Header、Body、超时等在提交异步任务前**全部快照**进 `mirrorTask`（值拷贝）；
- 避免 goroutine 持有整个请求对象 10 分钟（总时长上限）导致内存滞留；
- 异步结果**不回写** `AiBasicInfo`（见 10.1 的时序说明），只进 Prometheus。

### 4.4 Body 来源与上限

- AI 模式下 body 通常已被包装为 bytes_body（`http_conn.go:551` 提取 model 时触发），镜像模块直接 `req.HttpRequest.GetBodyAccessor().GetBytes()`（`bfe/bfe_http/request.go:976`）拷贝；
- `GetBytes()` 返回 `all=false`（body 超过可访问缓冲上限，默认 2MB、可配置至 `MaxAccessibleBodySize = 8MB`，`request.go:1005`）时按排除规则跳过并计数（`mirror_skip_total{reason="body_limit"}`）；
- `TotalBodyBufferSize` 全局限额触顶同样跳过，不报错；
- 拷贝为独立字节切片，与主链路解耦（主链路继续读取/转发不受影响）。

### 4.5 WebSocket 天然不涉及

WS 流量在模块管线之前分流到 `bfe_websocket`（`http_conn.go:470-496`），不经过 reverseproxy 模块管线，镜像模块收不到，无需特殊处理。

## 5. 配置模型

沿用 BFE 标准模块配置模式（基础 conf + product 规则 data 文件，`web_monitor` 热加载）。

### 5.1 模块基础配置 `conf/mod_traffic_mirror/mod_traffic_mirror.conf`

分层超时是 AI 场景的硬需求：流式推理总时长可达分钟级，不能用通用短超时。

| 配置项 | 类型 | 参数含义 | 必填 | 默认值 | 合法性条件 |
|--------|------|----------|------|--------|------------|
| Basic.ProductRulePath | String | 镜像规则文件路径 | Y | - | 文件须存在且可读 |
| Basic.ConnectTimeoutMs | Integer | 连接建立超时（毫秒） | N | 2000 | > 0 |
| Basic.TTFBTimeoutMs | Integer | 首字节超时（毫秒，TTFT 兜底） | N | 30000 | > 0 |
| Basic.TotalTimeoutMs | Integer | 镜像请求总时长上限（毫秒，SSE 读空兜底） | N | 600000 | ≥ TTFBTimeoutMs |
| Basic.MaxMirrorBodyBytes | Integer | 请求体镜像大小上限（字节） | N | 2097152（2MB） | > 0，≤ MaxAccessibleBodySize |
| Basic.MaxResponseBodyBytes | Integer | 镜像响应最大读取字节（超出截断丢弃并计数） | N | 16777216（16MB） | > 0 |
| Basic.MaxConcurrent | Integer | 模块级镜像并发上限（semaphore） | N | 1024 | > 0 |
| Basic.QueueCapacity | Integer | 提交队列容量，满则丢弃并计数 | N | 4096 | ≥ 0（0 表示不排队直接执行） |
| Basic.CircuitBreakerFailThreshold | Integer | 连续失败多少次触发熔断 | N | 50 | > 0 |
| Basic.CircuitBreakerCooldownSec | Integer | 熔断冷却秒数 | N | 30 | > 0 |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | false | - |

```ini
[basic]
ProductRulePath = ../conf/mod_traffic_mirror/mirror_rule.data
ConnectTimeoutMs = 2000
TTFBTimeoutMs = 30000
TotalTimeoutMs = 600000
MaxMirrorBodyBytes = 2097152
MaxResponseBodyBytes = 16777216
MaxConcurrent = 1024
QueueCapacity = 4096
CircuitBreakerFailThreshold = 50
CircuitBreakerCooldownSec = 30

[log]
OpenDebug = false
```

### 5.2 规则文件 `conf/mod_traffic_mirror/mirror_rule.data`

**结构沿用 BFE AI 模块惯例**：`Config: map[product] → 规则列表`，`Search(product)` + 逐条 `Cond.Match(req)`，与 `mod_ai_cache` / `mod_ai_rate_limit` 的 rule table 模式一致，使 `ai-gateway-api` / `conf-agent` 可复用同一套生成与下发逻辑。

```json
{
  "Version": "1.0",
  "Config": {
    "default": [
      {
        "cond": "req_path_prefix_in(\"/v1/chat/completions\", true) && req_body_json_in(\"model\", \"gpt-4o\", false)",
        "mirrorCluster": "cluster_shadow_v2",
        "percentage": 10,
        "removeHeaders": ["Authorization", "Cookie", "X-Api-Key"],
        "setHeaders": {"X-Bfe-Mirror": "true"},
        "bodyRewrites": [
          {"path": "model", "value": "deepseek-v3"}
        ],
        "pathRewrite": ""
      }
    ]
  }
}
```

| 字段 | 说明 |
|------|------|
| `cond` | BFE 条件表达式；为空则该 product 内全部匹配。AI 语义条件复用现有 body 条件原语（`req_body_json_in("model", ...)`，`bfe/bfe_basic/condition/primitive.go:1132` 的 `HttpReqBodyJsonGet` 系），按 path/header/body 字段组合表达；需求文档中的 `req_ai_model_in` 一期以 `req_body_json_in("model", ...)` 等价表达，如需语义糖后续在 condition 包统一封装 |
| `mirrorCluster` | `cluster_table.data` 中已定义的 cluster 名；复用现有健康检查、负载均衡、后端超时配置 |
| `percentage` | 0-100，按比例随机采样；100 = 全量镜像（发布前集中验证窗口） |
| `removeHeaders` | Header 黑名单剔除（FR-5），默认建议含 `Authorization`、`Cookie`、`X-Api-Key` |
| `setHeaders` | 注入标识头（FR-5/FR-14），默认注入 `X-Bfe-Mirror: true` 与追踪 logid |
| `bodyRewrites` | GJSON PATH 级 body 字段改写（FR-6，一期仅支持 `model` 字段）；实现新模型双跑验证 |
| `pathRewrite` | 可选路径改写（镜像目标路径不同时使用；非空时替换整个请求路径、query 保留），空表示不改写 |

### 5.3 Header 处理规则（FR-5）

构建镜像 Header 的顺序：

1. 拷贝原始 Header；
2. 剔除 hop-by-hop 头（`bfe_basic.HopHeaders`：Connection/Keep-Alive/Proxy-* 等）；
3. 剔除规则 `removeHeaders` 黑名单；
4. 注入 `setHeaders`（含默认 `X-Bfe-Mirror: true`）与 `X-Bfe-Logid`（从 `req.LogId` 快照，便于跨集群对账）。

### 5.4 热加载

与 `mod_ai_rate_limit` / `mod_ai_cache` 一致：`MonitorHandlers` 经 `web_monitor.RegisterHandlers` 暴露规则文件与基础配置的 reload 接口，返回 `文件名=Version` 供 conf-agent 对账；`conf-agent` 侧无需改动，标准 prober → file_store → trigger 流程自动覆盖新模块。

## 6. 模块文件结构

```text
bfe/bfe_modules/mod_traffic_mirror/
├── mod_traffic_mirror.go      # 模块入口、回调注册、生命周期、热加载
├── conf_mod_traffic_mirror.go # 基础配置解析
├── mirror_rule_table.go       # product + cond 规则表 + 匹配
├── mirror_rule_load.go        # 规则加载与热更新
├── mirror_sender.go           # 异步发送：http.Client、semaphore、提交队列
├── mirror_task.go             # 镜像任务结构（快照的目标/headers/body/改写）
├── mirror_body_rewrite.go     # body JSON 字段改写（model 重写，GJSON）
├── mirror_resp_read.go        # 响应读空：SSE 至 [DONE]、usage/error/finish_reason 解析
├── mirror_breaker.go          # 简单熔断器（连续失败计数 + 冷却窗口）
├── mirror_state.go            # module_state2 计数器 + Prometheus 指标
└── *_test.go                  # 单元测试（testing + testify）
```

## 7. 核心流程设计

### 7.1 总体流程

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

### 7.2 主链路侧（`mirrorHandler`）

主链路只做"规则匹配 + 快照 + 非阻塞提交"，不做任何网络 IO；任何失败静默吞掉只计数。

```go
func (m *ModuleTrafficMirror) mirrorHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
    // 1. 规则匹配：product + cond（AI 语义条件）
    rule := m.ruleTable.Search(req.Route.Product).Match(req)
    if rule == nil {
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 2. fallback 重试去重：每请求只镜像一次
    if req.GetContext(CtxMirrored) != nil {
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 3. 百分比采样
    if !sampleHit(rule.Percentage) {
        m.state.MirrorSkipSample.Inc(1)
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 4. 拷贝 body（超限跳过并计数，FR-11）
    body, all := req.HttpRequest.GetBodyAccessor().GetBytes()
    if !all || int64(len(body)) > m.conf.Basic.MaxMirrorBodyBytes {
        m.state.MirrorSkipBodyLimit.Inc(1)
        return bfe_module.BfeHandlerGoOn, nil
    }

    // 5. body 字段改写（FR-6：model 重写等；失败降级为原 body 并计数）
    mBody, err := rewriteBody(body, rule.BodyRewrites)
    if err != nil {
        m.state.MirrorSkipRewrite.Inc(1)
        mBody = body
    }

    // 6. 同步快照（SvrDataConf 在 clusterInvoke 返回后置 nil，禁止异步访问；
    //    goroutine 亦不允许持有 req，全部信息在此快照进 task）
    task := &mirrorTask{
        Method:     req.HttpRequest.Method,
        URL:        cloneURL(req.HttpRequest.URL),     // 含 pathRewrite 应用
        Header:     buildMirrorHeader(req, rule),      // 去 hop-by-hop、去黑名单、加标识
        Body:       mBody,                            // 独立拷贝，与主链路解耦
        Target:     m.snapshotTarget(rule),           // MirrorCluster → 后端地址快照
        Percentage: rule.Percentage,
        StartAt:    time.Now(),
    }

    // 7. 同步字段供访问日志（异步结果不回写，见 10.1）
    req.SetContext(CtxMirrored, true)
    aiInfo := req.GetAiBasicInfo()
    if aiInfo != nil {
        aiInfo.MirrorHit = true
        aiInfo.MirrorCluster = rule.MirrorCluster
    }

    // 8. 非阻塞提交（semaphore/队列满 → 丢弃并计数，不影响主链路）
    if !m.sender.Submit(task) {
        m.state.MirrorSubmitDrop.Inc(1)
    }
    return bfe_module.BfeHandlerGoOn, nil
}
```

### 7.3 异步侧（sender）

发送客户端仿 `mod_auth_request`（`bfe/bfe_modules/mod_auth_request/mod_auth_request.go:305-311`）：标准库 `http.Client`、`CheckRedirect` 禁 redirect、独立 `Transport` 连接池；分层超时通过 `Transport.DialContext`（连接）+ `httptrace`（首字节）+ `context.WithTimeout`（总时长）实现，与主链路超时完全解耦。

```go
func (s *mirrorSender) exec(t *mirrorTask) {
    defer s.sem.Release()
    s.state.MirrorInflight.WithLabelValues(t.Target.Cluster).Dec()

    // 1. 目标熔断检查（FR-12）
    if s.breaker.Open(t.Target) {
        s.state.MirrorCircuitOpen.Inc(1)
        return
    }

    // 2. 分层超时发送（FR-9：连接 / TTFB / 总时长）
    resp, ttfb, err := s.doWithLayeredTimeout(t)
    if err != nil {
        s.state.MirrorFail.Inc(1)
        s.breaker.OnFail(t.Target)
        return
    }

    // 3. 完整读空 + 轻量解析（FR-8）；SSE 读到 [DONE]，字节/时长双上限
    result := readAndParse(resp.Body, s.conf.Basic.MaxResponseBodyBytes,
        time.Duration(s.conf.Basic.TotalTimeoutMs)*time.Millisecond)
    resp.Body.Close()

    // 4. 成功/失败分类计数 + 熔断器反馈
    s.breaker.OnSuccess(t.Target)
    s.state.MirrorRespStatus.WithLabelValues(t.Target.Cluster,
        strconv.Itoa(resp.StatusCode)).Inc(1)
    s.state.MirrorFinishReason.WithLabelValues(t.Target.Cluster, result.FinishReason).Inc(1)
    s.state.MirrorErrorType.WithLabelValues(t.Target.Cluster,
        result.ErrorType, result.ErrorCode).Inc(1)
    s.state.MirrorTokens.WithLabelValues(t.Target.Cluster, "prompt").Add(float64(result.Usage.PromptTokens))
    s.state.MirrorTokens.WithLabelValues(t.Target.Cluster, "completion").Add(float64(result.Usage.CompletionTokens))
    s.reportLatency(t.Target.Cluster, ttfb, t.StartAt)
    // 结果只进 Prometheus；不回写 AiBasicInfo（见 10.1）
}
```

### 7.4 客户端断连处理（FR-15）

镜像与主链路完全解耦：客户端断连后主链路取消，但**镜像连接默认继续读完**（样本完整性优先，半途断开会让镜像集群取消推理、验证失真）；由总时长上限兜底。实现上镜像 goroutine 不响应客户端 context，只受自身的 `TotalTimeoutMs` 约束。

## 8. SSE 读空与轻量解析（`mirror_resp_read.go`）

### 8.1 读空策略

- **SSE（`Content-Type: text/event-stream`）**：逐事件解析，读到 `data: [DONE]` 或流结束（EOF）才算完成；只累积轻量语义字段，正文 `delta` 不保留（`io.Discard`）。
- **非流式**：`io.Copy(io.Discard, body)` 读空，期间按 JSON 解析 `usage` / `error` / `finish_reason`。
- **双上限**：累计读取字节 > `MaxResponseBodyBytes`（16MB）或读空时长 > `TotalTimeoutMs` → 截断丢弃，记 `mirror_resp_truncated_total` 计数。"未完成读空"与"上游异常断流"区分计数，后者计入 `mirror_fail_total` 供熔断器消费。

### 8.2 解析字段

| 字段 | 来源（OpenAI 协议） | 用途 |
|------|--------------------|------|
| usage.prompt_tokens | 最后一个（或非流式唯一的）含 usage 的 chunk | token 成本、吞吐估算 |
| usage.completion_tokens | 同上 | token 成本、吞吐估算（completion_tokens / 总时长 ≈ output tokens/s） |
| finish_reason | choices[].finish_reason（stop/length/content_filter/error） | 镜像 finish_reason 分布 |
| error.type / error.code / error.message | 非 200 响应体 OpenAI error JSON | AI 语义错误分类（rate_limit_exceeded、context_length_exceeded、model_not_found 等） |

SSE 事件解析参考 `mod_body_process` 的 SSE 解码器语义，需处理跨 chunk partial event、`[DONE]` 与前序 delta 同包、非流式 JSON 分段到达等边界。解析失败不视为镜像失败（body 照样读空丢弃），仅记 debug 日志与解析失败计数，避免误触发熔断。

## 9. 熔断与并发保护

### 9.1 并发控制（FR-7）

- `MaxConcurrent` 信号量硬上限控制模块级镜像并发（SSE 长连接占用高，独立计数）；
- 提交队列 `QueueCapacity` 作为突发缓冲，满则丢弃并计 `mirror_submit_drop_total`；
- in-flight 数暴露 `mirror_inflight{cluster}` 指标，容量评估按"峰值 QPS × 比例 × 平均流时长"测算。

### 9.2 熔断器（FR-12）

按镜像目标（cluster 维度）独立熔断：连续失败（连接失败/超时/5xx）达 `CircuitBreakerFailThreshold` 次 → 打开，冷却 `CircuitBreakerCooldownSec` 秒内该目标直接丢弃并计 `mirror_circuit_open_total`；冷却结束后放一次探测请求，成功则关闭。**熔断只做"暂停发新请求"，已在途的读空不受影响。**

## 10. 访问日志与监控

### 10.1 访问日志字段（`bfe-access-pb`）

701-900 AI 字段区间当前已用至 841 附近（781-788 cache/audio/image/video 子项、801/802 路由字段、841 quota plan），**镜像字段建议使用 842-850 区间，落码前以 `bfe_access.proto` 实际空闲编号为准**：

| 建议编号 | 字段名 | 类型 | 说明 |
|---|---|---|---|
| 842 | `mirror_hit` | bool | 该请求是否被镜像（提交成功即记 true，含队列满前已置位语义见 7.2） |
| 843 | `mirror_cluster` | string | 镜像目标 cluster 名 |
| 844 | `mirror_status` | int32 | 保留位：镜像响应状态码。**一期异步结果不回写**，该字段置空，结果以 Prometheus 为准（见下） |
| 845-850 | `mirror_latency_us` / `mirror_ttfb_us` / `mirror_prompt_tokens` / `mirror_completion_tokens` / `mirror_finish_reason` / `mirror_error` | - | 同上，一期保留为空 |

**异步结果不回写的设计决策**：镜像读空完成时间通常晚于 `HandleRequestFinish`（访问日志写出点），回写大多是无效写入；且 goroutine 持有 `req` 引用 10 分钟会造成内存滞留。因此访问日志只记录同步可得字段（`mirror_hit` / `mirror_cluster` / 跳过原因计数），异步完成的 status/usage/TTFB 全部走 Prometheus 实时看板；离线 Doris 报表的完整镜像维度（二期）以独立"镜像完成事件"补齐（需求 6.2 / 开放问题 7）。

改动点：`bfe-access-pb/bfe_access.proto`（`sh build.sh` 重新生成，不得手改）→ `bfe/bfe_basic/request_ai_basic.go` 的 `AiBasicInfo` 新增 `MirrorHit` / `MirrorCluster` 字段 → `bfe/bfe_modules/mod_access_pb3/request_log.go` 的 `reqAiInfoGen` 加赋值。

### 10.2 Prometheus 指标（`mirror_state.go`，仿 `mod_ai_rate_limit` 范式）

```text
mirror_req_total{product, cluster, model}        # 被镜像请求数（提交成功）
mirror_submit_drop_total                          # 队列/semaphore 满丢弃
mirror_skip_total{reason}                         # 采样未命中 / body超限 / 改写失败 / 熔断
mirror_resp_status_total{cluster, status}         # 镜像响应状态码分布 ★多少条200
mirror_error_type_total{cluster, type, code}      # OpenAI error.type/code ★有没有报错
mirror_finish_reason_total{cluster, reason}       # stop/length/content_filter/error
mirror_ttfb_ms_bucket{cluster}                    # 首字节延迟直方图（TTFT 近似）★撑不撑得住
mirror_latency_ms_bucket{cluster}                 # 总延迟直方图
mirror_tokens_total{cluster, kind}                # prompt/completion token 量 ★成本
mirror_inflight{cluster}                          # 当前镜像并发
mirror_fail_total{cluster, reason}                # 发送/读空失败（熔断器消费）
mirror_circuit_open_total{cluster}                # 熔断丢弃数
mirror_resp_truncated_total{cluster}              # 响应读空被双上限截断数
```

module_state2 计数器与 Prometheus 指标并存（BFE 模块惯例，`/{mod}.prometheus` + `/{mod}.status`）。

## 11. 计费/鉴权隔离（FR-10）

- 镜像是网关内部发出的 fire-and-forget 子请求，**不经过** `mod_ai_token_auth` 的配额扣减、`mod_ai_rate_limit` 限流，不进任何客户账单；镜像 token 消耗归属内部成本中心（由 Prometheus token 指标 + 报表侧做成本估算）；
- 实现上天然隔离：镜像请求由模块内独立 `http.Client` 直接发往镜像后端，不进入 BFE 自身监听端口、不重建 `bfe_basic.Request`，整个模块管线（含计费）不会二次执行；
- 主请求 fallback 重试只镜像一次（4.2 once 标记）；
- 集成测试需专项验证：镜像流量不产生任何 token auth 扣减记录、不触发 rate limit 计数（WBS 任务 9）。

## 12. 非功能性设计

| 维度 | 设计 |
|---|---|
| 主链路性能 | 镜像动作 = 一次规则匹配 + 一次 body 拷贝 + 一次非阻塞提交，主链路零等待；目标开销增量 < 5%（CPU：一次内存拷贝 + map/Header 克隆） |
| 主链路保护 | 镜像构建/提交失败静默吞掉只计数；所有丢弃路径（采样未命中/body 超限/改写失败/队列满/熔断）均有独立计数可观测 |
| 故障隔离 | 镜像目标宕机/网络隔离/慢响应不影响主请求：semaphore 硬上限 + 队列满丢弃 + 熔断冷却 + 分层超时，goroutine 堆积有界 |
| 数据合规 | Header 黑名单默认剔除 Authorization/Cookie/X-Api-Key（FR-5）；请求体含对话历史与系统提示词，镜像目标集群需与生产同等级访问控制与审计，合规评审为上线前置条件（需求第 10 章） |
| 成本控制 | 比例采样默认保守；`bodyRewrites` 可将 model 指到 mock/平价模型；token 成本指标 + 报表归属内部成本中心 |
| 容量 | 镜像并发 ≈ 生产峰值 QPS × 比例 × 平均流时长；in-flight/TTFB/丢弃率指标告警 |

## 13. 开发任务拆分（WBS）

BFE 侧任务（本文档范围）：

| 编号 | 任务 | 主要改动文件 | 说明 |
|------|------|--------------|------|
| 1 | 模块骨架与注册 | `bfe/bfe_modules/mod_traffic_mirror/mod_traffic_mirror.go`、`bfe/bfe_modules/bfe_modules.go` | 注册 HandleForward 回调，更新顺序注释（第 3 节） |
| 2 | 配置与规则表 | `conf_mod_traffic_mirror.go`、`mirror_rule_table.go`、`mirror_rule_load.go`、`conf/mod_traffic_mirror/*` | 分层超时基础 conf + product/cond/percentage/bodyRewrites 规则，热加载（第 5 节） |
| 3 | 镜像发送器 | `mirror_sender.go`、`mirror_task.go` | http.Client（仿 mod_auth_request）、semaphore、提交队列、快照与克隆（4.3、7.3） |
| 4 | Header/Body 处理 | `mirror_task.go`、`mirror_body_rewrite.go` | hop-by-hop 剔除、黑名单、标识头、model 字段改写（GJSON）（5.3、7.2） |
| 5 | SSE 读空与解析 | `mirror_resp_read.go` | 读空至 [DONE]、usage/error/finish_reason 解析、字节/时长双上限（第 8 节） |
| 6 | 熔断器 | `mirror_breaker.go` | 按目标连续失败计数 + 冷却窗口（9.2） |
| 7 | 监控与状态 | `mirror_state.go` | module_state2 计数器 + Prometheus 指标（10.2） |
| 8 | 访问日志字段 | `bfe-access-pb/bfe_access.proto`（842 起）、`bfe_basic/request_ai_basic.go`、`mod_access_pb3/request_log.go` | `AiBasicInfo.MirrorHit/MirrorCluster` 及日志赋值（10.1） |
| 9 | 计费/鉴权隔离验证 | 单元 + 集成测试 | 镜像请求不触发 token auth 扣减、rate limit、不进客户账单（第 11 节） |
| 10 | 单元测试 | `*_test.go` | 规则匹配、采样、header/body 改写、SSE 读空（[DONE]/usage/error/截断）、熔断、sender（testing + testify） |
| 11 | 集成测试 | `integration-test/`（独立仓库） | 真实 BFE + mock 镜像集群：复制一致性（含 model 改写）、故障隔离、重试去重、SSE 完整读空、断连继续读完 |
| 12 | 文档与配置样例 | `bfe/docs/zh_cn/configuration/mod_traffic_mirror/`、`conf/mod_traffic_mirror/` | 配置说明、运维手册 |

配套仓库任务：`ai-gateway-api`（`model/imods/mod_traffic_mirror.go` 配置导出、镜像规则 CRUD 接口）、`ai-gateway-web`（规则管理页、开关、比例调节、成本卡片）、`ai-gateway-observability`（Grafana 镜像看板）、Doris 报表"流量镜像"页签。

依赖关系：任务 1 → 2/3 → 4/5/6 → 7/8 → 9/10/11/12。任务 8 依赖 `bfe-access-pb` 仓库先行合入。

## 14. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| 生产数据复制到镜像集群（合规） | 请求体含完整对话历史+系统提示词（业务机密指令），复制即数据出域 | Header 黑名单剔除鉴权/会话头（5.3）；低比例起步 + 合规评审前置；镜像目标集群与生产同等级访问控制与审计；二期支持 body 脱敏钩子 |
| 镜像流量产生真实模型成本 | 100% 镜像 ≈ 双倍推理成本 | 比例采样默认保守；镜像目标可配置为 mock server 或平价模型（`bodyRewrites` 指到便宜模型）；token 成本指标 + 内部成本中心归属（10.2） |
| SSE 长连接占用镜像资源 | 分钟级流使 in-flight 高、semaphore 易打满 | 独立并发池 + 双上限 + 丢弃/截断计数告警；容量按"峰值 QPS × 比例 × 平均流时长"评估（9.1） |
| 读空不完导致推理取消/统计失真 | 半途断开使镜像集群取消生成、usage 缺失 | FR-8 强制读空至 [DONE]；总时长兜底后记录 truncated；集成测试断言镜像集群侧取消率 ≈ 0 |
| 镜像风暴反压网关 | 镜像目标慢/挂导致 goroutine 堆积 | semaphore 硬上限 + 队列满丢弃 + 熔断冷却 + 分层超时；所有丢弃路径有计数（7.2/9） |
| 大 Body 请求（>2MB/8MB）漏镜像 | 多模态/长上下文请求不在验证样本内 | 跳过并计数，报表可见跳过占比；如需完整覆盖二期做流式 tee |
| fallback 重试导致重复镜像 | 镜像集群 QPS 虚高、成本翻倍 | once 标记（CtxMirrored）保证每请求仅镜像一次（4.2），集成测试覆盖 |
| model 改写引发目标路由错配 | 改写值在镜像集群不存在，大量 model_not_found | 控制面配置期校验（改写值需在镜像集群模型清单内）；运行时该错误单列指标 `mirror_error_type_total` |
| SvrDataConf / req 生命周期 | goroutine 访问已置 nil 字段或滞留请求内存 | 提交前快照全部信息，goroutine 不持 req 引用（4.3）；异步结果不回写 AiBasicInfo（10.1） |
| 异步结果晚于访问日志 | 日志中镜像状态/usage 大量为空 | 一期设计取舍：同步只记 mirror_hit/cluster，结果以 Prometheus 为准（10.1）；离线维度二期用镜像完成事件补齐 |

## 15. 后续阶段展望

一期闭环运行后，视使用情况评估二期演进（需求文档第 11 章开放问题）：

1. **响应 diff**：脱敏后抽样保留双方响应轻量特征（usage、finish_reason、首条 delta）做自动比对——用户当前要求"响应直接吃掉"，默认不做；
2. **改写前镜像**：验证 prompt 改写策略变更时需要"改写前"挂点（body 快照提前或第二挂点）；
3. **通用 GJSON PATH 改写**：一期仅 `model` 字段，temperature/messages 等通用改写有误配置风险，按需开放；
4. **WS 镜像**：改造 `websocketDataTransfer` 做 tee；
5. **大 Body 流式 tee**：解除 8MB 可访问 body 上限对镜像覆盖率的限制；
6. **镜像完成事件独立入 Doris**：补齐离线报表的镜像全维度（status/usage/TTFB 按天/集群/模型聚合）。
