# mod_ai_route 对应 BFE 主程序修改方案

## 1. 背景与目标

### 1.1 背景

`mod_ai_route` 已在 `HandleFoundProduct` 阶段完成 AI 网关路由查找，并将结果写入请求上下文（`AiRouteResult`）。但与原有 BFE 转发流程相比，AI 网关需要：

- 不再走原有 `findCluster()` 的租户内路由；
- 按 `targets` 加权选择的目标进行转发；
- 在目标转发失败时，按 `fallbacks` 顺序降级；
- 支持模型字段覆盖与透传。

原有 `ReverseProxy.ServeHTTP()` 是为传统 BFE 转发设计的，直接修改它会引入较大复杂度和风险。实际实现新增了独立的 `ServeHTTPForAI()`，并在 `http_conn.go` 中根据 `EnableAiGateway` 开关进行分发。

### 1.2 目标

1. 在 `bfe_server/reverseproxy.go` 中新增 `ServeHTTPForAI()`，专门处理 AI 网关请求转发；
2. 在 `bfe_server/http_conn.go` 中根据 `EnableAiGateway` 决定调用 `ServeHTTP()` 或 `ServeHTTPForAI()`；
3. 复用现有 `clusterInvoke()`、`sendResponse()` 等核心转发能力；
4. 实现 target 命中后的模型覆盖/透传；
5. 实现 fallback 顺序降级机制；
6. 尽量降低对原有 `ServeHTTP()` 的侵入。

## 2. 设计原则

- **独立路径**：AI 网关和传统七层负载均衡使用不同的入口函数，互不干扰；
- **复用优先**：回调处理、集群查找、后端转发、响应发送尽量复用现有函数；
- **失败隔离**：fallback 只针对后端不可用场景，4xx/限流/鉴权失败不触发；
- **状态一致**：每次 fallback 重置 `OutRequest` 和相关上下文，避免污染下一次尝试。

## 3. 总体架构

### 3.1 请求处理路径

```
HTTP 请求接入
    │
    ▼
bfe_server/http_conn.go
    │
    ├── EnableAiGateway = false ──► ReverseProxy.ServeHTTP()
    │                                    (原有路径)
    │
    └── EnableAiGateway = true ───► ReverseProxy.ServeHTTPForAI()
                                         (新增路径)
```

### 3.2 ServeHTTPForAI 内部流程

```
┌─────────────────────────────────────┐
│  setClientAddr()                    │
│  HandleBeforeLocation               │
│  findProduct()                      │
│  HandleFoundProduct                 │
│    (mod_ai_route 写入 AiRouteResult) │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  未获取 AiRouteResult？              │
│  → 返回 404（AI 网关模式无默认路由）  │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  HandleAfterLocation                │
│  (mod_body_process 等）              │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  加权随机选择 target                  │
│  构建尝试列表：                       │
│  [selected target] + fallbacks       │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  prepareRequestBodyForRetry()        │
│  确保请求体可回退（Rewindable）       │
│  不可回退时禁用 fallback              │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  对每个尝试目标循环：                  │
│  1. resetRequestForRetry()（非首次）  │
│  2. ClusterTable.Lookup(ClusterName) │
│  3. 准备 OutRequest                  │
│  4. 模型覆盖 / cluster.AIConf 映射     │
│  5. clusterInvoke()                  │
│  6. 成功则跳出；失败且满足 fallback    │
│     条件则继续下一个 fallback          │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  response_got / send_response       │
│  HandleReadResponse                 │
└─────────────────────────────────────┘
```

## 4. 详细修改方案

### 4.1 修改 bfe_server/http_conn.go

在 `conn.serveRequest()` 中，根据 `EnableAiGateway` 决定调用哪个 `ServeHTTP`：

```go
// serve the request
var ret1 int
if c.server.Config.Server.EnableAiGateway {
    ret1 = c.server.ReverseProxy.ServeHTTPForAI(w, request)
} else {
    ret1 = c.server.ReverseProxy.ServeHTTP(w, request)
}
```

当前代码位置：`bfe_server/http_conn.go:558-559`

```go
// 原代码
ret1 := c.server.ReverseProxy.ServeHTTP(w, request)
```

替换为：

```go
var ret1 int
if c.server.Config.Server.EnableAiGateway {
    ret1 = c.server.ReverseProxy.ServeHTTPForAI(w, request)
} else {
    ret1 = c.server.ReverseProxy.ServeHTTP(w, request)
}
```

### 4.2 新增 ReverseProxy.ServeHTTPForAI()

在 `bfe_server/reverseproxy.go` 中新增 `ServeHTTPForAI()`，位于 `ServeHTTP()` 之后，便于复用内部辅助函数。

#### 4.2.1 函数签名

```go
// ServeHTTPForAI processes AI gateway http request and send http response.
func (p *ReverseProxy) ServeHTTPForAI(rw bfe_http.ResponseWriter, basicReq *bfe_basic.Request) (action int) {
    // implementation
}
```

#### 4.2.2 实现结构

```go
func (p *ReverseProxy) ServeHTTPForAI(rw bfe_http.ResponseWriter, basicReq *bfe_basic.Request) (action int) {
    var err error
    var res *bfe_http.Response
    var hl *bfe_module.HandlerList
    var retVal int
    var req *bfe_http.Request = basicReq.HttpRequest
    var serverConf *bfe_route.ServerDataConf
    var writeTimer *time.Timer
    var eppClient *epp.EppClient
    var ok bool

    // declare ai-related vars at top to avoid goto jumping over declarations
    var aiResult *bfe_basic.AiRouteResult
    var aiMeta *bfe_basic.AiBasicInfo
    var selectedTarget bfe_basic.AiRouteTarget
    var attempts []aiForwardAttempt
    var lastCluster *bfe_cluster.BfeCluster
    var invokeErr error

    isRedirect := false
    resFlushInterval := time.Duration(0)
    cancelOnClientClose := false

    timeoutReadClient := time.Duration(cluster_conf.DefaultReadClientTimeout) * time.Millisecond
    timeoutWriteClient := time.Duration(cluster_conf.DefaultWriteClientTimeout) * time.Millisecond
    timeoutReadClientAgain := time.Duration(cluster_conf.DefaultReadClientAgainTimeout) * time.Millisecond

    // get instance of BfeServer
    srv := p.server

    // set clientip of original user for request
    setClientAddr(basicReq)

    // ========== HandleBeforeLocation ==========
    hl = srv.CallBacks.GetHandlerList(bfe_module.HandleBeforeLocation)
    if hl != nil {
        retVal, res = hl.FilterRequest(basicReq)
        basicReq.HttpResponse = res
        switch retVal {
        case bfe_module.BfeHandlerClose:
            action = closeDirectly
            return
        case bfe_module.BfeHandlerFinish:
            action = closeAfterReply
            basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
            goto send_response
        case bfe_module.BfeHandlerRedirect:
            Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
            isRedirect = true
            basicReq.BfeStatusCode = basicReq.Redirect.Code
            goto send_response
        case bfe_module.BfeHandlerResponse:
            goto response_got
        }
    }

    // ========== findProduct ==========
    if err := srv.findProduct(basicReq); err != nil {
        basicReq.ErrCode = bfe_basic.ErrBkFindProduct
        basicReq.ErrMsg = err.Error()
        p.proxyState.ErrBkFindProduct.Inc(1)
        res = bfe_basic.CreateInternalSrvErrResp(basicReq)
        action = closeAfterReply
        goto response_got
    }

    // ========== HandleFoundProduct ==========
    hl = srv.CallBacks.GetHandlerList(bfe_module.HandleFoundProduct)
    if hl != nil {
        retVal, res = hl.FilterRequest(basicReq)
        basicReq.HttpResponse = res
        switch retVal {
        case bfe_module.BfeHandlerClose:
            action = closeDirectly
            return
        case bfe_module.BfeHandlerFinish:
            action = closeAfterReply
            basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
            goto send_response
        case bfe_module.BfeHandlerRedirect:
            Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
            isRedirect = true
            basicReq.BfeStatusCode = basicReq.Redirect.Code
            goto send_response
        case bfe_module.BfeHandlerResponse:
            goto response_got
        }
    }

    // ========== AI Route Result Check ==========
    aiResult = basicReq.GetAiRouteResult()
    if aiResult == nil {
        // AI gateway mode: no route hit, return 404
        basicReq.ErrCode = bfe_basic.ErrBkFindLocation
        basicReq.ErrMsg = "no ai route found"
        p.proxyState.ErrBkFindLocation.Inc(1)
        res = bfe_basic.CreateSpecifiedContentResp(basicReq, bfe_http.StatusNotFound,
            "text/plain", "AI route not found")
        action = closeAfterReply
        goto response_got
    }

    aiMeta = basicReq.GetAiBasicInfo()

    // ========== HandleAfterLocation ==========
    hl = srv.CallBacks.GetHandlerList(bfe_module.HandleAfterLocation)
    if hl != nil {
        retVal, res = hl.FilterRequest(basicReq)
        basicReq.HttpResponse = res
        switch retVal {
        case bfe_module.BfeHandlerClose:
            action = closeDirectly
            return
        case bfe_module.BfeHandlerFinish:
            action = closeAfterReply
            basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
            goto send_response
        case bfe_module.BfeHandlerRedirect:
            Redirect(rw, req, basicReq.Redirect.Url, basicReq.Redirect.Code, basicReq.Redirect.Header)
            isRedirect = true
            basicReq.BfeStatusCode = basicReq.Redirect.Code
            goto send_response
        case bfe_module.BfeHandlerResponse:
            goto response_got
        }
    }

    // ========== AI Forward Loop ==========
    serverConf = basicReq.SvrDataConf.(*bfe_route.ServerDataConf)

    // weighted random select target
    if len(aiResult.Targets) > 0 {
        selectedTarget = SelectTarget(aiResult.Targets)
    }

    // build attempt list: selected target + fallbacks
    attempts = make([]aiForwardAttempt, 0, 1+len(aiResult.Fallbacks))
    if selectedTarget.ClusterName != "" {
        attempts = append(attempts, aiForwardAttempt{
            ClusterName: selectedTarget.ClusterName,
            Model:       selectedTarget.Model,
            IsFallback:  false,
        })
    }
    for _, fb := range aiResult.Fallbacks {
        attempts = append(attempts, aiForwardAttempt{
            ClusterName: fb.ClusterName,
            Model:       fb.Model,
            IsFallback:  true,
        })
    }

    // ensure request body is rewindable before attempting fallbacks
    if len(attempts) > 1 && basicReq.HttpRequest.Body != nil {
        if !prepareRequestBodyForRetry(basicReq.HttpRequest) {
            log.Logger.Warn("ServeHTTPForAI: request body is not rewindable, disable fallback")
            attempts = attempts[:1]
        }
    }

    for i, attempt := range attempts {
        if i > 0 {
            // fallback attempt: reset request state
            if !p.resetRequestForRetry(basicReq) {
                log.Logger.Warn("ServeHTTPForAI: fallback aborted, request body cannot be rewound")
                break
            }
        }

        res, action, lastCluster, invokeErr = p.aiClusterInvoke(srv, serverConf, basicReq, rw, attempt, aiMeta)
        if invokeErr == nil && res != nil && res.StatusCode < 500 {
            // success or 4xx (client error, do not fallback)
            break
        }

        // decide whether to try next fallback
        if i == len(attempts)-1 {
            // last attempt
            break
        }
        if !shouldTriggerFallback(res, invokeErr) {
            break
        }

        // log fallback
        log.Logger.Info("ServeHTTPForAI: fallback triggered, cluster[%s] err[%v] status[%d]",
            attempt.ClusterName, invokeErr, getResponseStatus(res))

        if res != nil {
            res.Body.Close()
        }
    }

    basicReq.HttpResponse = res
    basicReq.SvrDataConf = nil

    if err != nil || res == nil {
        basicReq.Stat.ResponseStart = time.Now()
        basicReq.BfeStatusCode = bfe_http.StatusInternalServerError
        res = bfe_basic.CreateInternalSrvErrResp(basicReq)
        goto response_got
    }

    // set response-phase timeouts based on the last cluster used
    if lastCluster != nil {
        resFlushInterval = lastCluster.ResFlushInterval()
        cancelOnClientClose = lastCluster.CancelOnClientClose()
        timeoutWriteClient = lastCluster.TimeoutWriteClient()
        timeoutReadClientAgain = lastCluster.TimeoutReadClientAgain()
    }
    if resFlushInterval == 0 && basicReq.HttpRequest.Header.Get("Accept") == "text/event-stream" {
        if lastCluster != nil {
            resFlushInterval = lastCluster.DefaultSSEFlushInterval()
        }
    }

    // ========== response_got / send_response (same as ServeHTTP) ==========
    // ... reuse existing response handling code ...

send_response:
    // send http response to client
    // ... same as ServeHTTP ...
    return
}
```

### 4.3 新增辅助类型和函数

#### 4.3.1 aiForwardAttempt

```go
type aiForwardAttempt struct {
    ClusterName string
    Model       string
    IsFallback  bool
}
```

#### 4.3.2 aiClusterInvoke()

封装一次 AI 目标转发，复用 `clusterInvoke()`：

```go
func (p *ReverseProxy) aiClusterInvoke(srv *BfeServer, serverConf *bfe_route.ServerDataConf,
    basicReq *bfe_basic.Request, rw bfe_http.ResponseWriter,
    attempt aiForwardAttempt, aiMeta *bfe_basic.AiBasicInfo) (
    res *bfe_http.Response, action int, cluster *bfe_cluster.BfeCluster, err error) {

    req := basicReq.HttpRequest

    // update route info
    basicReq.Route.ClusterName = attempt.ClusterName
    basicReq.Backend.ClusterName = attempt.ClusterName

    // look up cluster
    cluster, err = serverConf.ClusterTable.Lookup(attempt.ClusterName)
    if err != nil {
        log.Logger.Warn("no cluster for %s", attempt.ClusterName)
        basicReq.Stat.ResponseStart = time.Now()
        basicReq.ErrCode = bfe_basic.ErrBkNoCluster
        basicReq.ErrMsg = err.Error()
        p.proxyState.ErrBkNoCluster.Inc(1)
        return nil, closeAfterReply, nil, err
    }

    // set deadline to finish read client request body
    timeoutReadClient := cluster.TimeoutReadClient()
    if basicReq.IsSse {
        timeoutReadClient = -1
    }
    p.setTimeout(bfe_basic.StageReadReqBody, basicReq.Connection, req, timeoutReadClient)

    // prepare out request
    outreq := new(bfe_http.Request)
    *outreq = *req // includes shallow copies of maps, but okay
    basicReq.OutRequest = outreq

    httpProtoSet(outreq)
    hopByHopHeaderRemove(outreq, req)

    if cluster.DisableHostHeader {
        outreq.Host = ""
    }

    // apply model override from ai route target/fallback
    if attempt.Model != "" && aiMeta != nil {
        if err := condition.ReqBodyJsonSet(basicReq, "model", attempt.Model); err != nil {
            log.Logger.Warn("Failed to set model in request body: %s", err)
        } else {
            if outreq.ContentLength >= 0 {
                outreq.ContentLength = -1
                outreq.Header.Del("Content-Length")
            }
            aiMeta.TargetModel = attempt.Model
        }
    }

    // apply cluster.AIConf (api key, model mapping)
    if cluster.AIConf != nil && aiMeta != nil {
        if cluster.AIConf.Key != nil {
            mod_ai_token_auth.SetApiKey(outreq, *cluster.AIConf.Key)
        }
        if cluster.AIConf.ModelMapping != nil {
            model := aiMeta.ClientModel
            if aiMeta.TargetModel != "" {
                model = aiMeta.TargetModel
            }
            if model != "" {
                if newModel, ok := (*cluster.AIConf.ModelMapping)[model]; ok {
                    if err := condition.ReqBodyJsonSet(basicReq, "model", newModel); err != nil {
                        log.Logger.Warn("Failed to set model in request body: %s", err)
                    } else {
                        if outreq.ContentLength >= 0 {
                            outreq.ContentLength = -1
                            outreq.Header.Del("Content-Length")
                        }
                        aiMeta.TargetModel = newModel
                    }
                }
            }
        }
    }

    // invoke cluster
    res, action, err = p.clusterInvoke(srv, cluster, basicReq, rw)
    return res, action, cluster, err
}
```

#### 4.3.3 shouldTriggerFallback()

```go
func shouldTriggerFallback(res *bfe_http.Response, err error) bool {
    if err != nil {
        return true
    }
    if res != nil && res.StatusCode >= 500 {
        return true
    }
    return false
}
```

#### 4.3.4 resetRequestForRetry()

```go
func (p *ReverseProxy) resetRequestForRetry(basicReq *bfe_basic.Request) bool {
    // clear previous backend connection
    if basicReq.Trans.Backend != nil {
        basicReq.Trans.Backend.DecConnNum()
        basicReq.Trans.Backend = nil
    }
    basicReq.Trans.Transport = nil
    basicReq.RetryTime = 0

    // reset out request so body can be re-read
    basicReq.OutRequest = nil

    // rewind request body for next fallback attempt
    if !rewindRequestBody(basicReq.HttpRequest) {
        return false
    }

    // clear error info from previous attempt
    basicReq.ErrCode = nil
    basicReq.ErrMsg = ""
    return true
}
```

#### 4.3.5 prepareRequestBodyForRetry()

在尝试 fallback 之前，先确保请求体可回退。若当前 body 已实现 `bfe_http.Rewindable` 接口，则直接返回成功；否则通过 `GetBodyAccessor()` 尝试将其转换为 `bytes_body`。当全局 bytes_body 缓冲区大小达到 `TotalBodyBufferSizeLimit()` 限制时，不再包装，fallback 被禁用。

```go
func prepareRequestBodyForRetry(req *bfe_http.Request) bool {
    // if total buffer size already reaches the limit, do not wrap (no retry)
    if limit := bfe_http.TotalBodyBufferSizeLimit(); limit > 0 {
        if bfe_http.TotalBytesBodyBuffer() >= limit {
            return false
        }
    }
    if req.Body == nil {
        return true
    }
    if _, ok := req.Body.(bfe_http.Rewindable); ok {
        return true
    }
    if _, err := req.GetBodyAccessor(); err != nil {
        return false
    }
    _, ok := req.Body.(bfe_http.Rewindable)
    return ok
}
```

#### 4.3.6 rewindRequestBody()

`resetRequestForRetry()` 内部调用，将已支持 `Rewindable` 的请求体重置到起始位置。

```go
func rewindRequestBody(req *bfe_http.Request) bool {
    if req.Body == nil {
        return true
    }
    rewindable, ok := req.Body.(bfe_http.Rewindable)
    if !ok {
        return false
    }
    return rewindable.Rewind()
}
```

## 5. 与 ServeHTTP() 的共用逻辑

以下逻辑在 `ServeHTTPForAI()` 中直接调用或复用，与 `ServeHTTP()` 保持一致：

| 逻辑 | 复用方式 |
|------|----------|
| `setClientAddr()` | 直接调用 |
| `HandleBeforeLocation` | 直接调用 |
| `findProduct()` | 直接调用 |
| `HandleFoundProduct` | 直接调用 |
| `HandleAfterLocation` | 直接调用 |
| `httpProtoSet()` | 直接调用 |
| `hopByHopHeaderRemove()` | 直接调用 |
| `clusterInvoke()` | 直接调用 |
| `sendResponse()` | 直接调用 |
| `HandleReadResponse` | 直接调用 |
| `prepareRequestBodyForRetry()` | 直接调用 |
| `rewindRequestBody()` | 直接调用 |
| SSE/EPP/超时处理 | 直接复用 `response_got` 后代码 |

## 6. Target 选择器

`mod_ai_route` 不执行 target 选择，加权随机选择逻辑在 `ServeHTTPForAI()` 中通过 `bfe_server/reverseproxy.go` 中的 `SelectTarget()` 实现：

```go
import (
    "math/rand"
    "time"

    "github.com/bfenetworks/bfe/bfe_basic"
)

var aiTargetRand = rand.New(rand.NewSource(time.Now().UnixNano()))

func SelectTarget(targets []bfe_basic.AiRouteTarget) bfe_basic.AiRouteTarget {
    if len(targets) == 1 {
        return targets[0]
    }

    r := aiTargetRand.Intn(100)
    sum := 0
    for _, target := range targets {
        sum += target.Weight
        if r < sum {
            return target
        }
    }
    return targets[len(targets)-1]
}
```

> 说明：`SelectTarget()` 位于 `bfe_server/reverseproxy.go` 中，供 `ServeHTTPForAI()` 使用。

## 7. 模型覆盖逻辑

### 7.1 覆盖优先级

1. **target/fallback.Model 非空**：覆盖请求体中的 `model` 字段；
2. **cluster.AIConf.ModelMapping**：将当前 `model`（可能是原始 model 或 target 覆盖后的 model）映射为后端模型；
3. **均空**：透传原始 `model`。

### 7.2 请求体处理

每次调用 `condition.ReqBodyJsonSet()` 后，必须：

```go
if outreq.ContentLength >= 0 {
    outreq.ContentLength = -1
    outreq.Header.Del("Content-Length")
}
```

以避免 `Content-Length` 与实际 body 长度不一致。

## 8. Fallback 机制

### 8.1 触发条件

触发 fallback：

- `clusterInvoke()` 返回 `err != nil`（连接失败、超时、读写错误等）；
- `clusterInvoke()` 返回的响应状态码 `>= 500`。

不触发 fallback：

- 后端返回 `4xx`（视为客户端错误）；
- 请求被限流、鉴权失败等（`HandleFoundProduct` 阶段已处理，不会进入转发）。

### 8.2 行为

- 按 `fallbacks` 列表顺序依次尝试；
- 第一个成功（`err == nil` 且状态码 `< 500`）即停止；
- 所有 fallback 均失败后，返回最后一个 fallback 的响应或错误；
- 每次 fallback 前重置 `OutRequest`、backend 连接、retry 计数等状态。

### 8.3 请求体重用

fallback 时需确保请求体可重新读取。实现要点：

- 在转发前调用 `prepareRequestBodyForRetry()`，将非 `Rewindable` 的 body 通过 `GetBodyAccessor()` 包装为可重复读取的 `bytes_body`；
- 若 body 已实现 `bfe_http.Rewindable` 接口，则直接复用；
- 每次 fallback 前由 `resetRequestForRetry()` 调用 `rewindRequestBody()` 将 body 重置到起始位置；
- `aiClusterInvoke()` 每次从 `basicReq.HttpRequest` 重新构造 `OutRequest`。

全局 bytes_body 缓冲区受 `bfe_http.TotalBodyBufferSizeLimit()` 限制，达到上限后不再包装 body，fallback 会被禁用。单个请求可访问/缓冲的最大 body 大小由 `ConfigBasic.AccessibleBodySize` 控制（默认取自 `bfe_http.DefaultAccessibleBodySize`），超过该大小的请求体无法通过 `ReqBodyJsonSet` 等接口修改。

> 注意：请求体必须在首次消费前具备可回退能力，否则 fallback 将被禁用或失败。

## 9. 错误处理

| 场景 | 处理方式 |
|------|----------|
| AI 路由未命中 | 返回 404 Not Found |
| 集群不存在 | 复用 `ErrBkNoCluster`，返回 500 |
| target 转发失败 | 触发 fallback |
| 所有 fallback 失败 | 返回最后一个 fallback 的响应；无响应则返回 500 |
| 模型覆盖失败 | 记录 warn 日志，继续转发 |

## 10. 日志

当前实现通过 `log.Logger` 输出以下关键日志：

- target 命中与选择结果；
- fallback 触发原因（错误类型/状态码）；
- 每次 fallback 尝试的集群名和结果。

## 11. 测试覆盖

### 11.1 单元测试

`bfe_server/reverseproxy_ai_test.go` 已新增，覆盖以下场景：

1. `ServeHTTPForAI()` 正常转发命中；
2. AI 路由未命中返回 404；
3. `SelectTarget()` 加权随机分布符合预期；
4. target 模型覆盖生效；
5. cluster.AIConf.ModelMapping 生效；
6. 后端 5xx 触发 fallback；
7. 后端 4xx 不触发 fallback；
8. 所有 fallback 失败后返回正确响应；
9. `shouldTriggerFallback()` 边界条件。

### 11.2 集成验证

- 启用 `EnableAiGateway = true`，配置 `mod_ai_route`，验证完整请求链路；
- 启用 `EnableAiGateway = false`，验证原有 `ServeHTTP()` 不受影响；
- 热加载 `ai_route.data` 后，新请求按新规则转发。

## 12. 已完成的修改

1. 在 `bfe_server/reverseproxy.go` 中新增 `SelectTarget()`、`aiForwardAttempt`、`aiClusterInvoke()`、`shouldTriggerFallback()`、`resetRequestForRetry()`、`prepareRequestBodyForRetry()`、`rewindRequestBody()` 等辅助函数；
2. 新增 `ServeHTTPForAI()`，复用现有回调和转发逻辑；
3. 修改 `bfe_server/http_conn.go` 中的请求分发逻辑；
4. 新增单元测试 `bfe_server/reverseproxy_ai_test.go`；
5. 编译验证并通过测试。

## 13. 注意事项

1. **与原 `ServeHTTP()` 的隔离**：`ServeHTTPForAI()` 为独立实现，不修改 `ServeHTTP()` 的状态机，避免影响传统转发；
2. **请求体重复读取**：fallback 依赖请求体可回退能力，`prepareRequestBodyForRetry()` 会提前将 body 包装为 `Rewindable`；
3. **超时设置**：每次 `aiClusterInvoke()` 根据目标集群配置重新设置读请求体超时；
4. **连接数统计**：`resetRequestForRetry()` 递减 backend 连接计数并清空 `Trans.Backend`；
5. **SSE 流式响应**：fallback 仅在首次 target 转发前决策，已开始发送的 SSE 响应不再切换；
6. **EPP 处理**：`ServeHTTPForAI()` 复用了 `ServeHTTP()` 中的 EPP 清理逻辑。

## 14. 附录

### 14.1 修改文件清单

| 文件 | 修改类型 |
|------|----------|
| `bfe_server/reverseproxy.go` | 新增 `ServeHTTPForAI()`、`SelectTarget()` 及辅助函数 |
| `bfe_server/http_conn.go` | 修改请求分发逻辑 |
| `bfe_server/reverseproxy_ai_test.go` | 新增单元测试 |

### 14.2 关键函数调用关系

```
http_conn.serveRequest()
    │
    ├── EnableAiGateway=false
    │   └── ReverseProxy.ServeHTTP()
    │
    └── EnableAiGateway=true
        └── ReverseProxy.ServeHTTPForAI()
            ├── setClientAddr()
            ├── callbacks: HandleBeforeLocation / findProduct / HandleFoundProduct
            ├── GetAiRouteResult()
            ├── SelectTarget()  ← 新增
            ├── callback: HandleAfterLocation
            ├── prepareRequestBodyForRetry()  ← 新增
            └── aiClusterInvoke() × (1 + N fallbacks)
                ├── resetRequestForRetry()（非首次） ← 新增
                ├── ClusterTable.Lookup()
                ├── model override
                ├── cluster.AIConf handling
                └── clusterInvoke()
            └── response handling
```
