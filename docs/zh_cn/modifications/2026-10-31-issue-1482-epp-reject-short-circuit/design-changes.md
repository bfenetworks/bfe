# BFE Issue #1482 修复方案：EPP 过载拒绝的透传与短路

- Issue: https://github.com/bfenetworks/bfe/issues/1482
- 缺陷：后端实例都已满负荷时，EPP 集群上的流控设置不起作用，新请求全部放进后端排队（14~31 秒），没有请求被拒绝。
- 现状与分工：配置下发链（控制台 → ai-gateway-api `flow_control` → InnerAPI epp-data → EPP 热编译）是通的；**流控"不生效"由两侧共同造成**——
  1. 判定侧（不在本仓）：EPP 的饱和检测只按 KV cache 利用率/后端排队深度判，且 FC 的 occupancy 只统计 EPP 内排队，不看已派发在飞，因此后端 HTTP 饱和时请求被立即 admit；
  2. 出口侧（**本仓**）：EPP 对流控拒绝/驱逐使用 ext_proc `ImmediateResponse`（429/503），而 bfe 只读 `envoy.lb` 元数据、**丢弃 IR**；即使认出，`reverseproxy` 也会无条件回退本地负载均衡。**即 EPP 真拒了，客户端也看不到错误码。**
- 本卡范围：**只做出口侧**（识别 IR → 不重试、不回退、不计熔断 → 明确错误码 + 指标）。判定侧的"并发饱和检测可配"由 ai-gateway-api 侧同批落地（`epp_config.saturation_detector`，见跨仓说明）；EPP 侧零代码（`concurrency-detector` 与 `inflightload` 默认 producer 已注册）。
- 关联：#1480（非 2xx 行的 token/计费口径，本卡的拒绝行沿用它）、#1441（AiError 统一错误体与 `X-Bfe-Gw-Error`）、#1453（`BK_NO_BACKEND` 业务码先例）、#1475（`drainResponses` 为长流死锁修复，本卡不得破坏）、#1474/#1478（非 2xx 计费边界）。
- 跨仓：ai-gateway-api 新增 `epp_config.saturation_detector`（mode/`max_concurrency`/`headroom`/`concurrency_mode`）；ai-gateway-epp 需 ≥ 支持 `concurrency-detector` 的版本（`llm-d-router v0.11.0-rc.2` 起，现有 `8417262` 已注册）。bfe 不感知这些字段。

## 一、现状盘点与缺口

### 1.1 EPP 的拒绝是 ext_proc `ImmediateResponse`

llm-d-router `pkg/epp/handlers/server.go:205` 注释写明"The state machine sends an ImmediateResponse(429) to the proxy"；`:719-742` 是驱逐路径（同样 429）。`pkg/epp/flowcontrol/types/outcomes.go` 区分"背压 429"与"池缩容到零 503"。

### 1.2 bfe 丢弃 IR

`bfe_balance/bal_gslb/bal_gslb.go:359-471 callEPP`：

- `RecvTimeout(callTimeout)`（`:428`、`:453`）拿到的 `ProcessingResponse` 只用于读 `dynamic metadata → envoy.lb → x-gateway-destination-endpoint`（`:434-451`）；
- **从未检查 `resp.GetImmediateResponse()`**；拿不到 endpoint 就 `return "", nil, fmt.Errorf("no endpoint from epp")`（`:466-467`）。

### 1.3 即便识别出来，当前也会被本地回退吞掉

`bfe_server/reverseproxy.go:345-372`：

```go
if bal.BalanceMode == cluster_conf.BalanceModeEPP {
    clusterBackend, err = bal.BalanceEpp(request)
}
if clusterBackend == nil {              // ← 无条件回退
    clusterBackend, err = bal.Balance(request)   // 本地 WRR
    ...
}
```

`BalanceEpp`（`bal_gslb.go:714-771`）在 EPP 无决策时返回 `nil, fmt.Errorf("EPP no decision: %v", err)` 且**不设置 `req.ErrCode`**，于是流程继续走本地均衡——过载保护被绕过。

### 1.4 拒绝会被误记成 EPP 故障

- `bal_gslb.go:758-760`：`if breaker != nil { breaker.record(err == nil) }` ——任何 EPP 错误都算失败，持续过载会把熔断打开，进一步退化成"本地均衡"，与保护意图相反；
- `bal_gslb.go:755-757`：`ErrEppFallbackLocal` / `recordEPPFallbackLocal` 也会把"拒绝"算成"回退"，混淆两种语义；
- `bfe_balance/bal_gslb/epp_metrics.go:33-37, 107-141`：结果分类只有 `ok/no_pool/unknown_pool/draining/transport`，没有"拒绝"。

### 1.5 响应阶段 IR 同样不可见（P1）

`bfe_util/epp/epp_client.go:299-307 drainResponses` 把响应阶段所有消息 `Recv()` 后丢弃（这是 #1475 修长流死锁的前提：必须持续读）。因此"在飞驱逐"（response 阶段 IR 429）不可见——本卡不改，留 P1，且改造时**不能**改成"识别后停止读"。

### 1.6 失败出口已有统一映射，缺"拒绝"一支

`bfe_server/reverseproxy.go:1452-1480` 在 `err != nil || res == nil` 时调用 `aiGatewayFailureResponse`（`:2302-2330`），按 `basicReq.ErrCode` 分派，目前只有 `ErrBkNoBackend`（`BK_NO_BACKEND` 500）与 `ErrBkBodyProcess`（`CONTENT_REJECTED` 500）两个特例，其余落 `BACKEND_UNAVAILABLE`（502）。另 `:1426-1429` 有"`ErrAiRateLimit` 不触发集群 fallback"的先例可复用。

## 二、修复方案

### 步骤 1（必改）：新增拒绝类型与哨兵

`bfe_basic/error_code.go`（现有 `ErrBkNoBackend = ...`、`ErrBkRetryTooMany = ...` 见 `:40-48`）：

```go
// ErrEppRejected marks an EPP admission decision (ext_proc ImmediateResponse),
// not a transport/infra failure: the request must not be retried on another EPP
// address, must not fall back to local balancing, and must not count against the
// EPP circuit breaker (issue #1482).
ErrEppRejected = errors.New("EPP_REJECTED")
```

`bfe_balance/bal_gslb`（新文件 `epp_reject.go` 或 `bal_gslb.go` 内）：

```go
// eppRejected carries the EPP admission decision.
type eppRejected struct {
    StatusCode int    // 429 (backpressure) or 503 (no capacity)
    Code       string // optional x-error-code from the IR headers
    RetryAfter string // optional retry-after from the IR headers
    Body       string // truncated EPP body, logs only (never echoed to clients)
}

func (e *eppRejected) Error() string
// IsEPPRejected reports whether err is an EPP admission decision.
func IsEPPRejected(err error) bool
```

### 步骤 2（必改）：`callEPP` 解析 `ImmediateResponse`

在两处 `RecvTimeout` 后（`bal_gslb.go:428` 首个往返、`:453` 带 body 的第二个往返）插入同一段处理：

```go
if ir := resp.GetImmediateResponse(); ir != nil {
    eppClient.Close()
    rej := &eppRejected{Body: truncateString(string(ir.GetBody()), 512)}
    if st := ir.GetStatus(); st != nil {
        rej.StatusCode = int(st.GetCode())
        for _, h := range st.GetHeaders().GetSetHeaders() {
            switch {
            case strings.EqualFold(h.GetHeader().GetKey(), "retry-after"):
                rej.RetryAfter = string(h.GetHeader().GetRawValue())
            case strings.EqualFold(h.GetHeader().GetKey(), "x-error-code"):
                rej.Code = string(h.GetHeader().GetRawValue())
            }
        }
    }
    if rej.StatusCode == 0 {
        rej.StatusCode = bfe_http.StatusTooManyRequests
    }
    return "", nil, rej
}
```

要点：

- IR 可能出现在**第一次**往返（准入拒绝）或**带 body 的第二次**，两处都要拦；
- 只把 **429/503** 当"拒绝"；其它状态码视为异常响应，走既有 transport 错误路径（不改变原有重试/回退行为，避免新路径放大故障面）；
- **EPP 的 body 不回传客户端**（可能含池/地址等内部信息），只截断入日志。

### 步骤 3（必改）：不重试、不回退、不计熔断

1. `isEPPRetryable`（`bal_gslb.go:473-491`）：显式对 `*eppRejected` 返回 false（虽然 `status.FromError` 对非 grpc 错误天然 false，仍要加分支 + 注释，防止将来并入 transport 分支）；
2. `BalanceEpp`（`:714-771`）：在 `chooseBackendFromEPP` 失败后加分支——

```go
var rej *eppRejected
if errors.As(err, &rej) {
    bal.recordEPPRejected(rej)              // 指标（Prometheus + monitor 计数）
    req.ErrCode = bfe_basic.ErrEppRejected  // 失败出口据此映射
    req.SetContext(REQ_CTX_EPP_REJECT, rej) // 承载状态码/Retry-After
    return nil, err                          // 不落 fallback 计数、不 recordEPPFallbackLocal
}
```

3. 熔断：`if breaker != nil { breaker.record(err == nil) }` 改为拒绝不参与：

```go
if breaker != nil && !IsEPPRejected(err) {
    breaker.record(err == nil)
}
```

4. `ErrEppFallbackLocal` / `recordEPPFallbackLocal`：拒绝**不递增**（拒绝不是"回退"）。

### 步骤 4（必改）：`reverseproxy` 短路

`bfe_server/reverseproxy.go:345-372` 的平衡循环：

```go
if bal.BalanceMode == cluster_conf.BalanceModeEPP {
    clusterBackend, err = bal.BalanceEpp(request)
    if bal_gslb.IsEPPRejected(err) {
        // EPP 的准入决策：直接失败，绝不回退本地均衡
        break
    }
}
if clusterBackend == nil && err == nil {   // 仅"EPP 无决策且非拒绝"才回退
    clusterBackend, err = bal.Balance(request)
    ...
}
```

`break` 后走既有失败出口（`err != nil || res == nil` → `aiGatewayFailureResponse`），无需新增出口。

### 步骤 5（必改）：明确错误码与 `Retry-After`

`bfe_basic/request_ai_basic.go`（错误目录 `:660-664` 的 `CodePriceNotConfigured` 是同类先例；状态码表 `:755`、类型表 `:813`）：

```go
// CodeEppOverload: EPP refused admission because the pool is saturated
// (backpressure). A gateway capacity decision, retryable later.
CodeEppOverload = "EPP_OVERLOAD"
// CodeEppNoCapacity: EPP has no capacity for this request (pool scaled to
// zero / no endpoint can serve it).
CodeEppNoCapacity = "EPP_NO_CAPACITY"
```

- `ErrorCodeToStatusCode`：`CodeEppOverload: 429`、`CodeEppNoCapacity: 503`；
- `ErrorCodeToType`：`CodeEppOverload: TypeRateLimitError`、`CodeEppNoCapacity: TypeInternalError`。

`aiGatewayFailureResponse`（`reverseproxy.go:2302-2330`）新增一支：

```go
if basicReq.ErrCode == bfe_basic.ErrEppRejected {
    rej := eppRejectDetail(basicReq) // 从 REQ_CTX_EPP_REJECT 取出
    code, typ := bfe_basic.CodeEppOverload, bfe_basic.TypeRateLimitError
    if rej != nil && rej.StatusCode == bfe_http.StatusServiceUnavailable {
        code, typ = bfe_basic.CodeEppNoCapacity, bfe_basic.TypeInternalError
    }
    res := bfe_basic.NewAiError(code, typ, "gateway capacity exhausted, retry later").CreateErrorResponse(basicReq)
    if rej != nil && rej.RetryAfter != "" {
        res.Header.Set("Retry-After", rej.RetryAfter)
    }
    return res
}
```

配套：

- **不触发集群 fallback**：与 `ErrAiRateLimit` 同款（`reverseproxy.go:1426-1429`）——在 AI fallback 判定处把 `ErrEppRejected` 加入"不 fallback"名单；
- **不写 key 惩罚**：`aiKeyPenaltyTTL429`（`:2296-2300`）是"上游限流"语义，网关自身过载不得惩罚 provider key；
- `CreateErrorResponse` 自带 `X-Bfe-Gw-Error`，`normalizeUpstreamError` 不会二次改写。

### 步骤 6（必改）：指标与日志

`bfe_balance/bal_gslb/epp_metrics.go`：

- 新增结果标签 `eppResultRejected = "rejected"`（`callsTotal{cluster, result}`，`:33-37`）；
- 新增 `eppRejectedTotal{cluster, status}`（Prometheus）；
- `classifyEPPError`（`:107-141`）增加 `errors.As(err, &rej)` → `eppResultRejected`；
- `bal_gslb` state 增加 `EPPRejected *metrics.Counter`（web monitor 页可见）；
- 日志：`Warn("EPP rejected request: cluster=%s status=%d code=%s retryAfter=%s body=%s")`，body 截断 512 字节。

## 三、兼容性与发布顺序

- **旧 EPP（不发 IR）**：解析分支不进入，行为与现状逐字节一致，可先发 bfe。
- **异常 IR**：只认 429/503；其余状态码仍按 transport 处理并保留原有重试/回退，避免新路径放大故障面。
- **与 #1480 的口径**：被拒请求是网关生成的非 2xx → `keepNon2xxUsage` 生效：`ai_usage_source=failed`（有鉴权播种则上报估算 token，`UsedQuota` 保持 0），`ai_cost_value` 缺席；报表可按 `err_code=EPP_OVERLOAD` 或 `res_status_code=429` 统计，**不要**用"是否有 token"判断。
- **重试/熔断**：`req.RetryTime` 不递增、`ErrBkRetryTooMany` 不触发；拒绝不推高熔断失败率。
- **发布顺序**：bfe 先发（对新 IR 无副作用）→ ai-gateway-api 开放 `saturation_detector`（并发检测）→ 端到端验收。api 侧字段对旧 EPP 无影响（旧 EPP 不认识 `concurrency-detector` 时，该集群会保留上一代引擎，不中断服务）。

## 四、回归测试

### 单元测试

| 文件 | 用例 |
| --- | --- |
| `bfe_balance/bal_gslb/`（新增 `epp_reject_test.go`） | `TestCallEPPImmediateResponse429`（首个往返 IR 429 + `Retry-After: 1` → 返回 `*eppRejected{429, RetryAfter:"1"}`、client 已关闭）；`TestCallEPPImmediateResponse503`；`TestCallEPPImmediateResponseOnBodyRound`（带 body 的第二次往返）；`TestCallEPPIgnoresNonRejectImmediateResponse`（IR 500 → 视为 transport，走原路径） |
| 同上 | `TestChooseBackendFromEPPNoRetryOnReject`（双 EPP 地址，首个拒绝 → 不试第二个）；`TestBalanceEppRejectKeepsBreakerClosed`（连续拒绝后熔断仍 closed）；`TestBalanceEppRejectNoFallbackMetric`（`ErrEppFallbackLocal` 不增、`req.ErrCode=ErrEppRejected`） |
| `bfe_balance/bal_gslb/epp_metrics_test.go` | `classifyEPPError(rej)` → `rejected`；`eppRejectedTotal` 标签断言 |
| `bfe_server/reverseproxy_test.go`（若无则靠 SC 覆盖） | `aiGatewayFailureResponse`：`ErrEppRejected`+429 → `EPP_OVERLOAD`/429/`rate_limit_error`/`Retry-After` 透传；+503 → `EPP_NO_CAPACITY`/503 |

### 集成测试（`bfe/tests/integration`）

1. **扩展 `common/mock_epp.go`**：新增 option `WithImmediateResponse(status int, headers map[string]string, body string, afterMessages int)`，在 `Process` 中返回 `ProcessingResponse_ImmediateResponse`（`afterMessages=0` 即首个往返）。
2. **SC14（EPP scheduling）新增用例**：
   - `TestTCxx_EPPRejectReturns429`：mock EPP 返回 IR 429 → 客户端 **429 `EPP_OVERLOAD`**、`X-Bfe-Gw-Error` 存在、**后端命中 0**、`epp_calls_total{result="rejected"}` +1；
   - `TestTCxx_EPPReject503`：IR 503 → **503 `EPP_NO_CAPACITY`**、后端 0 命中、`Retry-After` 透传；
   - `TestTCxx_EPPRejectNoLocalFallback`：集群同时配本地后端且 EPP 拒绝 → 后端命中 **0**（旧实现会命中 1 次，此用例即红→绿证明）；
   - `TestTCxx_EPPRejectKeepsBreakerClosed`：连续 N 次拒绝后 EPP 仍被调用（熔断未打开）。
3. **回归**：SC14 既有用例（正常调度、长流、EPP 故障回退、熔断开/半开）、SC36（SSE 长连接）、SC29（usage 快照与上游断流臂）全量回归，确认 IR 解析不影响响应阶段流。（跨仓的上游断流场景 SC46 在 integration-test 仓库，不在本仓。）

## 五、边界与残余风险

1. **误拒**：阈值由 ai-gateway-api 侧配置（`max_concurrency` 等），bfe 无法判断 EPP 是否误判；只能靠 `EPPRejected` 比例与客户端 429 监控发现。**逃生方案**：实例级回滚 bfe 版本即可（旧版本忽略 IR，恢复"忽略 + 本地回退"）；若要集群级开关（如 `GslbBasic.EPPRejectShortCircuit`），需 ai-gateway-api 同步导出一个 `GslbBasic` 字段，属跨仓小改动，**本卡不默认实现**（列为开放问题）。
2. **body 泄露**：方案明确"EPP 的 IR body 不回传"，只截断入日志；若产品希望透传 EPP 标准错误体，需先确认 EPP body 已脱敏。
3. **`Retry-After` 缺失**：EPP 目前不保证下发，bfe 缺省不带（不假装有建议）。
4. **response 阶段 IR（P1）**：在飞驱逐不可见；改造 `drainResponses` 时必须"持续读 + 识别"，不能识别后停止读（#1475 的长流死锁会回归）。
5. **pipeline 交互**：`mod_ai_token_auth` 的鉴权在路由前完成，被拒请求已扣"鉴权期检查"但不计费；若将来在 EPP 拒绝后补做"负优先级驱逐"，需重新审视计费门控。

## 六、部署与配置说明

- 无新增配置项（bfe 侧零配置）；行为由 EPP 是否下发 IR 决定。
- 监控：`epp_calls_total{result="rejected"}` 与 `eppRejectedTotal{status}` 上线后应出现；`epp_fallback_local_total` 在过载期间应**不**随拒绝增长（这是本卡与旧行为的关键区别）。
- 报表：被拒行 `err_code=EPP_OVERLOAD|EPP_NO_CAPACITY`、`res_status_code=429|503`、`ai_cost_value` 缺席、`ai_usage_source=failed`。

## 七、实施记录（2026-10-31）

### 7.1 改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/error_code.go` | 新增 `ErrEppRejected` 哨兵 |
| `bfe_basic/request_ai_basic.go` | 新增 `CodeEppOverload`(429, rate_limit_error) / `CodeEppNoCapacity`(503, internal_error) 并登记 `ErrorCodeToStatusCode` / `ErrorCodeToErrorType` |
| `bfe_http/status.go` | 导出 `StatusTooManyRequests = 429`（原为未导出的 `statusTooManyRequests`，同 map 同步） |
| `bfe_balance/bal_gslb/epp_reject.go`（新增） | `EPPRejected` 决策类型（StatusCode/Code/RetryAfter/Body）、`newEppRejected`（只认 429/503；`Retry-After`/`x-error-code` 取自 IR 的 HeaderMutation，缺省用 `Details` 作 code；body 截断 512 字节）、`IsEPPRejected` / `rejectionOf` / `EPPRejectedDetail` |
| `bfe_balance/bal_gslb/bal_gslb.go` | 新增 `REQ_CTX_EPP_REJECT`；`callEPP` 在两处 `RecvTimeout` 后解析 IR；`isEPPRetryable` 对拒绝返回 false；`BalanceEpp` 拒绝分支（计数、置 `ErrCode`、存上下文、Warn 日志，**不落 fallback 计数、不 record 熔断**）；`BalErrState` 新增 `ErrEppRejected` |
| `bfe_balance/bal_gslb/epp_metrics.go` | `epp_calls_total` 增 `rejected` 分类；新增 `epp_rejected_total{cluster,status}`；`classifyEPPError` 增加拒绝分支 |
| `bfe_server/reverseproxy.go` | 平衡循环对 `IsEPPRejected(err)` 直接 `break`（跳过本地均衡回退）；AI fallback 判定排除 `ErrEppRejected`；`aiGatewayFailureResponse` 增加 429/503 映射 + `Retry-After` 透传 + 固定文案（不回显 EPP body） |
| `docs/zh_cn/sys_design/epp_integration.md` | §4.1 错误语义表补 IR 两行与说明；§4.4 补拒绝分支；§4.7 指标补 `rejected` / `epp_rejected_total` / `ErrEppRejected`；§5 补"拒绝不算故障、不降级"与旧 EPP 兼容；§6 补 `epp_reject.go` |
| `tests/integration/common/mock_epp.go` | 新增 `SetImmediateResponse(ir, onBody)`：mock 以 IR 代替调度决策（`onBody=true` 延迟到请求体往返） |
| `tests/integration/.../scenario-SC14-epp-scheduling/` | 新增 TC-09~TC-12；`postWithHeaders` 辅助 |
| `tests/integration/测试设计文档/scenario-SC14-EPP调度对接/场景说明.md` | 用例清单与断言方式同步 |

### 7.2 验证结果

单测：

```
go test ./bfe_balance/... ./bfe_server/... ./bfe_basic/... ./bfe_http/... -count=1  → 全部通过
新增：TestNewEppRejected / TestBalanceEppRejectedNoRetryNoFallback /
      TestBalanceEppRejectedOnBodyRound / TestBalanceEppRejectedKeepsBreakerClosed
      （bfe_balance/bal_gslb），TestAIGatewayFailureResponseEppRejected（bfe_server）
```

集成（`bfe/tests/integration`）：

| 场景 | 结果 |
| --- | --- |
| SC14（12 个用例，含新增 TC-09 429 透传 / TC-10 503 / TC-11 不回退本地 / TC-12 不打开熔断） | ok（13.5s） |
| SC36 SSE 长连接 | ok（9.5s） |
| SC29 usage 快照（含上游断流臂） | ok（19.4s） |

新用例的红/绿依据：TC-11 断言"拒绝时 `epp_fallback_local_total` 不增长、EPP 侧计数 == 请求数"——改动前该计数会随每次拒绝增长且请求被本地均衡转发成功（200），用例必红。

### 7.3 实施期间与初稿的偏差

1. **`Retry-After` 的解析位置**：方案写"从 `ImmediateResponse.Status` 的 headers 取"，实测 go-control-plane 的 `HttpStatus` 只带 `Code`，头在 `ImmediateResponse.Headers`（HeaderMutation）。已改为读 HeaderMutation，并用 `Details` 兜底 `code`。
2. **`bfe_http` 缺 429 常量**：仓库只有未导出的 `statusTooManyRequests`，已导出为 `StatusTooManyRequests`（同文件状态文本表同步），避免业务代码写裸数字。
3. **`BalErrState.ErrEppRejected` 的可见性**：该扁平计数器由 `NewServerStatus()` 的反射 `Init` 建立，单测里为 nil（`metrics.Counter` 的 `Inc/Get` 对 nil 安全）；因此单测的"不落 fallback / 拒绝计数"断言改走 `EppMetricsText()` 的 Prometheus 序列（包内全局 registry），并按"前后差值"断言以容忍同包其他用例的累计。
4. **拒绝类型的可见性**：为让 `bfe_server` 的失败响应测试能构造决策，`EPPRejected` 与 `EPPRejectedDetail` 导出（原方案为包内私有）。
5. **回归清单修正**：方案里误把 integration-test 仓的场景 SC46 列为 bfe 侧回归项，实际 bfe 侧对应 SC29（含上游断流臂）+ SC36。