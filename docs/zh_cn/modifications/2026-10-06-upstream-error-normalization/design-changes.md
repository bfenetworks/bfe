# 上游错误体归一（统一错误码）

## 1. 背景

BFE 作为 AI 网关，自生成错误（认证/限流/配额/路由拒绝）已统一为 OpenAI 风格
`{error:{code,type,message,param,details}}`（`bfe_basic/request_ai_basic.go:300-463`，
总表 `docs/zh_cn/sys_design/ai_error_codes.md`）。但**上游厂商错误体原样透传**：
fallback 循环耗尽后，上游 4xx/5xx 响应经 `sendResponse` 直写客户端
（`bfe_server/reverseproxy.go:1345,1448-1449`），body 未做任何改写。

由此产生四个问题：

1. **同一错误多形态**：OpenAI / Anthropic / Gemini 三类 error envelope 直透，
   "多后端同一错误"承诺不成立，客户端需按厂商分支适配；
2. **状态码语义混淆**：上游 401（cluster key 失效）与客户端自身 key 失效都
   表现为 401，排障方向错误；
3. **无法统一监控**：错误码按厂商各异，告警/对账规则无法统一；
4. **凭证泄漏面**：上游错误消息可能回显 cluster key（原文/base64/URL 编码/
   JSON 转义形态），随错误体或日志外泄。

一期协议适配（`2026-09-03-model-protocol-adapter/design-changes.md`）已预埋
`ErrorNormalizer` seam（`bfe_model_protocol/protocol.go:65-68`、
`bfe_server/reverseproxy.go:1830`），但默认实现恒返回 nil
（`bfe_model_protocol/utils/errors.go:40-48`），且该 seam 只服务内部容错决策，
没有面向客户端的归一动作。

**目标**：

- 上游错误（三协议 envelope + SSE 流内错误事件）归一为统一错误目录与 OpenAI
  兼容响应体，状态码按统一映射表重映射（非流式）；
- 流式：错误事件载荷归一改写、流截断检测入访问日志，不改状态码、不驱动重试；
- 归一链路内嵌 cluster key 脱敏（客户端响应 + 访问日志两个出口）；
- per-cluster 配置开关，默认关闭，可灰度可回退。

**非目标**：

- 不改内部容错决策：`shouldTriggerFallback` 白名单、key 罚分保持现状；
  `ErrorNormalizer()` 恒返回 default（属"错误归一与容错增强"方案，独立评审）；
- 不做首事件探针容错（未提交字节时整体替换 4xx/5xx 响应、key 轮换/fallback）；
- 不做协议间错误格式互转（Anthropic 客户端同样收到 OpenAI 风格错误体）；
- 不重写 2xx/3xx 成功内容，不触碰计费链路。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 协议层类型 | `ProtocolError` 扩展字段；新增 `ErrorParser` / `StreamErrorParser` 接口 | `bfe/bfe_model_protocol/utils/errors.go`（新增） |
| 协议适配器 | openai/anthropic/gemini 实现 `ParseError` + `ParseStreamError`；映射表内置 | `bfe/bfe_model_protocol/{openai,anthropic,gemini}/*.go`、`protocol.go` |
| 错误目录 | 新增 `UPSTREAM_*` 错误码；激活 4 个预留码；`ErrorCodeToStatusCode` 扩充；`AiErrorDetail` 加 `upstream_status`/`upstream_code` | `bfe/bfe_basic/request_ai_basic.go` |
| 转发主链路 | fallback 循环结束后插入归一拦截点：非流式 body 重写 + SSE 过滤器挂接 | `bfe/bfe_server/reverseproxy.go` |
| 归一实现（新文件） | `normalizeUpstreamError`（非流式）、SSE 事件过滤器、凭证脱敏 | `bfe/bfe_server/reverseproxy_ai_error.go`（新增） |
| 集群配置 | `AIConf.NormalizeUpstreamError`（`Enabled`/`StreamEnabled`/`UnrecognizedAction`/`MaxBodyBytes`/`RedactSecrets`）+ `AIConfCheck` 校验 | `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` |
| 访问日志 | 新字段：`ai_upstream_status`/`ai_upstream_err_code`/`ai_err_normalized`/`ai_err_normalize_miss`/`ai_stream_error_rewritten`/`ai_stream_truncated` | `bfe-access-pb`（proto 扩展）→ `bfe/bfe_modules/mod_access_pb3/request_log.go` |
| 文档/样例 | 错误码总表更新；`conf/` 集群配置样例同步 | `bfe/docs/zh_cn/sys_design/ai_error_codes.md`、`conf/` |

## 3. 现状时序与问题定位

`ServeHTTPForAI` 的响应路径（`bfe_server/reverseproxy.go`）：

```text
for i, attempt := range attempts {              // :1299 fallback 循环
    res, ..., invokeErr = p.aiClusterInvoke(...) // :1308
    if ... < 400 { break }                       // 成功
    if !shouldTriggerFallback(res, invokeErr, aiMeta.AuthStyle) { break }  // :1332
}
basicReq.HttpResponse = res                     // :1345 —— 本改动拦截点
...
response_got:                                     // :1378
    eppClient.ProcRespHeader / NewEppResponseBodyFilter(res.Body)  // :1393-1399
    ...
send_response:
    p.sendResponse(rw, res, ...)                  // :1448 写出（不可再改）
```

`shouldTriggerFallback` 内的容错 seam（`:1819-1841`）：

```go
// 现状（保持不动）：phase-1 default normalizer 恒返回 nil，
// 决策仍走下方状态码白名单
if perr := modelprotocol.Get(authStyle).ErrorNormalizer().Normalize(code, nil, nil); perr != nil {
    return perr.IsUpstream && (perr.SwapKey || perr.Retryable)
}
```

问题定位：归一动作必须在"最终响应确定、字节未写出"的窗口内完成——即
`:1345` 之前、EPP 包裹（`:1393-1399`）之前。这是唯一同时满足
①不再影响 fallback 决策、②响应未提交、③模块链路（EPP/`HandleReadResponse`/
访问日志）看到最终形态的插入点。

## 4. 详细改动

### 4.1 协议层：`ErrorParser` / `StreamErrorParser` 接口（utils 叶子包）

**不复用 `ErrorNormalizer()`**，新增两个并行接口（seam 隔离，见 §5 拒绝方案①）：

```go
// bfe/bfe_model_protocol/utils/errors.go（新增，紧邻既有 ErrorNormalizer）

// ProtocolError 扩展（既有字段保留）：
type ProtocolError struct {
    Code         string // 归一后的网关错误码（UPSTREAM_* / 激活的预留码）
    StatusCode   int    // 上游原始 HTTP 状态码
    IsUpstream   bool
    Retryable    bool
    SwapKey      bool
    MarkDead     bool
    Message      string
    UpstreamCode string  // 新增：上游原始错误码（OpenAI error.code /
                          // Anthropic error.type / Gemini error.status）
    Param        *string // 新增：透传上游 param
}

// ErrorParser 解析非流式错误 envelope；nil = 未识别。
type ErrorParser interface {
    ParseError(statusCode int, body []byte, header http.Header) *ProtocolError
}

// StreamErrorParser 解析单个 SSE 事件；nil = 非错误事件。
type StreamErrorParser interface {
    ParseStreamError(ev StreamEvent) *ProtocolError
}
```

`ProtocolAdapter` 相应新增 `ErrorParser()` / `StreamErrorParser()` 两个方法
（`bfe_model_protocol/protocol.go`），与 `ErrorNormalizer()` 并存——
**`ErrorNormalizer()` 保持返回 `DefaultErrorNormalizer`**，容错语义零改动。

### 4.2 三协议适配器实现

每个适配器实现真实解析（宽容 JSON 解码，非 JSON / 未识别一律返回 nil）：

- **openai**：解析 `{"error":{message,type,param,code}}`（code 兼容字符串与嵌套
  对象两种形态）；429 额外读 `Retry-After` 头填 `details.retry_after_seconds`；
  SSE 侧识别 `event: error` 或 data 为 `{"error":...}` 形态；
- **anthropic**：解析 `{"type":"error","error":{type,message}}`；SSE 侧
  `event: error`；
- **gemini**：解析 `{"error":{code,message,status}}`，以 `status` 枚举
  （google.rpc.Status）为主映射键；SSE 侧 data 含 `error` 字段（Gemini 流无
  终止事件，以 HTTP EOF 正常结束）。

协议 → 网关错误码映射表内置于各适配器（常量表，纯数据），总表为
`docs/zh_cn/sys_design/ai_error_codes.md`（随本改动同步更新）。核心映射：

| 上游信号 | 归一 code | HTTP |
|---|---|---|
| 400/422 请求类错误 | `UPSTREAM_INVALID_REQUEST` | 400 |
| context_length_exceeded / request_too_large | `CONTEXT_LENGTH_EXCEEDED`（激活预留） | 400 |
| 内容审核类 | `CONTENT_FILTERED`（激活预留） | 400 |
| 上游 401/402/403 | `UPSTREAM_AUTH_ERROR` | 502 |
| model_not_found / not_found_error | `UPSTREAM_MODEL_NOT_FOUND` | 404 |
| 408 / 504 | `BACKEND_TIMEOUT`（激活预留） | 504 |
| 429 限流 / 配额耗尽 | `UPSTREAM_RATE_LIMITED` / `UPSTREAM_QUOTA_EXHAUSTED` | 429 |
| overloaded_error / 529 | `UPSTREAM_OVERLOADED` | 503 |
| 上游其余 5xx | `MODEL_INTERNAL_ERROR`（激活预留） | 500 |
| 识别为错误但无映射（仅 rewrite_generic） | `UPSTREAM_UNKNOWN` | 502 |

### 4.3 错误目录扩充（`bfe_basic/request_ai_basic.go`）

- 新增 `UPSTREAM_INVALID_REQUEST`/`UPSTREAM_RATE_LIMITED`/
  `UPSTREAM_QUOTA_EXHAUSTED`/`UPSTREAM_AUTH_ERROR`/
  `UPSTREAM_MODEL_NOT_FOUND`/`UPSTREAM_OVERLOADED`/`UPSTREAM_UNKNOWN` 七个
  code 常量，与 `CONTEXT_LENGTH_EXCEEDED`/`CONTENT_FILTERED`/
  `MODEL_INTERNAL_ERROR`/`BACKEND_TIMEOUT` 四个预留码一并登记
  `ErrorCodeToStatusCode`（`:349-388`）；
- `AiErrorDetail`（`:398-405`）新增 `UpstreamStatus *int` /
  `UpstreamCode string` 两个字段（omitempty，旧序列化形态不变）；
- 归一响应复用 `AiError.CreateErrorResponse`（`:454-463`）生成，与网关自生成
  错误同一构造路径。

### 4.4 非流式归一拦截点（`reverseproxy.go` + 新文件）

`ServeHTTPForAI` fallback 循环结束、`basicReq.HttpResponse = res`（`:1345`）之前
插入：

```go
// bfe/bfe_server/reverseproxy_ai_error.go（新增文件骨架）

// normalizeUpstreamError rewrites a final upstream error response into the
// unified AiError catalog. Called once after the fallback loop settles and
// before any byte is written / before the EPP filter wraps res.Body.
// No-op unless the cluster enables NormalizeUpstreamError.
func normalizeUpstreamError(basicReq *bfe_basic.Request, res *bfe_http.Response,
    aiConf *cluster_conf.AIConf, authStyle string, clusterKey string)

// 判定与流程：
// 1. cfg := aiConf.NormalizeUpstreamError; cfg == nil || !cfg.Enabled → return
// 2. res.StatusCode < 400 → return（成功响应不动）
// 3. 网关自生成标记检查（见下）→ return
// 4. body, _ := io.ReadAll(io.LimitReader(res.Body, cfg.MaxBodyBytes)); res.Body.Close()
// 5. perr := adapter(authStyle).ErrorParser().ParseError(res.StatusCode, body, res.Header)
// 6. perr == nil（未识别）：
//      passthrough（默认）→ 仅 RedactSecrets 时脱敏后回写 body，透传原状态码
//      rewrite_generic → 生成 UPSTREAM_UNKNOWN 统一响应
// 7. 识别成功 → redactSecretMaterial(perr.Message, clusterKey)
//      → NewAiError(perr.Code, typeOf(perr.Code), msg, details{upstream_status,
//        upstream_code, retry_after...}) → CreateErrorResponse 同款重建 res
// 8. 全程 fail-open：任何 error 只记 warn，res 保持可读、请求不中断
```

**网关自生成响应标记**：`AiError.CreateErrorResponse` 与
`CreateInternalSrvErrResp` 等本地错误路径统一加私有响应头标记
（如 `X-Bfe-Gw-Error: 1`，写出前剥除），归一判定前置检查，避免对已是统一
格式的本地错误二次改写。

### 4.5 SSE 事件过滤器（流内归一 + 截断检测）

同一拦截点，对 SSE 响应（`res.IsSse` / `Content-Type: text/event-stream`）且
`cfg.StreamEnabled` 时，包裹 `res.Body`：

```go
// bfe/bfe_server/reverseproxy_ai_error.go

type aiErrorStreamFilter struct {
    src      io.ReadCloser
    parser   utils.StreamErrorParser // 由 authStyle 取适配器
    redact   func(string) string     // cluster key 脱敏闭包
    onTrunc  func()                  // EOF 且无终止事件 → 置 basicReq 截断标记
    sawTerminal bool
}

func (f *aiErrorStreamFilter) Read(p []byte) (int, error)
// 内部按 SSE 事件增量解析（复用 utils.StreamEvent 形态，零/近零分配）：
//   非错误事件 → 原样透传（含注释行/心跳/usage 事件，计费不受影响）
//   错误事件   → ParseStreamError 非 nil → 脱敏 + 映射 → 事件 data 载荷
//                重写为 {"error":{...统一格式...}}，事件名/id/结构不变
//   终止事件   → 记 sawTerminal（adapter.IsStreamTerminal）
//   EOF        → !sawTerminal 且协议有终止语义 → onTrunc()
// 未识别事件：透传（RedactSecrets 时先脱敏），流式不支持 rewrite_generic
```

挂载位置必须在 EPP 包裹之前（`:1393-1399`），保证 EPP/日志读到最终事件流。
Gemini 适配器 `IsStreamTerminal` 恒 false（`protocol.go:70-75`），天然不误报截断。

### 4.6 凭证脱敏

```go
// redactSecretMaterial 展开 cluster key 的 N 种形态（原文 / base64 /
// URL-encode / JSON 转义）为子串模式集，命中替换为 "••••••••"。
// fail-open：扫描异常只记 warn，绝不阻塞错误返回。
```

覆盖范围（关键决策）：**集群归一开关开启后，脱敏作用于所有外发上游错误内容**——
归一重写路径（message 嵌入上游原文之前）、passthrough 路径（body 已缓冲，含 key
命中时脱敏写回、不含 key 逐字节不变）、流式错误事件、访问日志
`ai_upstream_err_code` 字段。开关关闭的集群维持现状（不脱敏 = 存量行为）。

### 4.7 集群配置（`AIConf`）

```go
// bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go
// AIConf 新增（json 键与 Go 字段同名，PascalCase，与 ModelProtocols 等一致）：

type UpstreamErrorNormalizeConf struct {
    Enabled              bool   // 非流式归一开关，默认 false
    StreamEnabled        bool   // 流内归一开关，独立灰度，默认 false
    UnrecognizedAction   string // passthrough（默认）/ rewrite_generic
    MaxBodyBytes         int64  // 默认 65536；<=0 取默认
    RedactSecrets        bool   // 默认 true
}

// AIConf 内新增字段：
NormalizeUpstreamError *UpstreamErrorNormalizeConf
```

`AIConfCheck`（`:1178`）增加校验：`UnrecognizedAction` 非法值报错；
`MaxBodyBytes` 上限保护（如 ≤ 4MB）。零值/缺省 = 关闭，旧 cluster_table.data
无需变更即兼容。`conf/` 样例配置同步（AGENTS.md 要求）。

### 4.8 访问日志字段

新字段经 `bfe-access-pb` proto 扩展后由 `mod_access_pb3/request_log.go` 填充，
取值挂在 `basicReq` 请求级状态（归一拦截点写入，日志模块读取，与
`AiAuthInfo.RejectReason` 同模式）：

| 字段 | 来源 |
|---|---|
| `ai_upstream_status` | 归一拦截点记录的上游原始状态码 |
| `ai_upstream_err_code` | 上游原始错误码（脱敏后） |
| `ai_err_normalized` | 1 重写 / 0 透传（含未识别、开关关闭） |
| `ai_err_normalize_miss` | 未识别标记（补映射表样本） |
| `ai_stream_error_rewritten` | 本流至少一个错误事件被归一 |
| `ai_stream_truncated` | EOF 缺失协议终止事件 |

指标（`proxyState` counter）：`ErrNormalizeHit`/`ErrNormalizeMiss`/
`ErrStreamErrorRewritten`/`ErrStreamTruncated`/`ErrNormalizeRedact`。

## 5. 行为变化与兼容性

| 项 | 现状 | 新行为（开关开启后） | 兼容性 |
|---|---|---|---|
| 开关缺省 | — | `Enabled`/`StreamEnabled` 均 false | **存量集群逐字节不变**；现有集成测试不加配置全绿即证明 |
| 上游 4xx/5xx 非流式错误 | 原样透传（状态码+body） | 统一错误体 + 映射状态码（上游 401→502 等） | 客户端可见变化，需 release note 公告；行内客户端先行验证 |
| 上游 5xx 状态 | 500/502/503 直透 | 归一为 500/504 | 同上，灰度观察 |
| SSE 流内错误事件 | 厂商原生事件直透 | data 载荷统一为 OpenAI 格式（事件名/status 不变） | status 恒 200 不变，仅载荷变化 |
| 流中断 | 与正常结束不可区分 | `ai_stream_truncated` 标记 + counter | 纯新增可观测，无行为变化 |
| fallback 重试 | 状态码白名单 | **不变** | 归一仅在循环结束后介入 |
| key 池罚分/容错 | 状态码分支 | **不变** | `ErrorNormalizer()` 恒 default |
| 计费/配额 | 预扣 + 按 usage 结算 | 不变（错误无 usage；过滤器透传 usage 事件） | 无 |
| passthrough 错误体 | 逐字节透传 | 不含 key 时逐字节不变；含 key 时脱敏写回 | 语义不变，安全增强 |

**不变量清单**（回归锚点）：`shouldTriggerFallback` 及其白名单、key 选择/罚分
逻辑、fallback 尝试序列、`mod_body_process` usage 提取与审核钩子、EPP 过滤、
`AiError` 序列化字段（新增字段均 omitempty）、`ErrorCodeToStatusCode` 既有条目。

**拒绝的备选方案**：

1. **在 `ErrorNormalizer()` seam 上做归一**——该 seam 位于容错决策路径，换成
   真实解析会立刻改变 fallback 行为（sibling 方案明确需全量回归+评审），
   与"客户端归一低风险独立灰度"目标冲突；故新增并行接口、seam 保持 default；
2. **归一逻辑放 `mod_body_process` 模块**——该模块可不被加载，且归一是转发
   语义而非 body 加工语义；放 `HandleReadResponse` 还会晚于 EPP 包裹点；
3. **流式首事件缓冲、整体替换 4xx/5xx 响应**——要求延迟响应头发出，与
   `sendResponse` 时序强耦合，且其价值依赖容错探针（二期）才有意义；
   一期事件级改写无"已提交"边界问题；
4. **全局单开关而非 per-cluster**——多厂商集群灰度节奏不同，且 StreamEnabled
   需要独立于 Enabled 灰度。

## 6. 测试

1. **单测**（`make test`）：
   - 三协议 `ParseError`/`ParseStreamError`：envelope 正反例、畸形 JSON、
     code 对象形态、空 body、Retry-After、各协议错误事件判定（table-driven）；
   - 映射表全用例；
   - 脱敏：key 四形态命中、fail-open、passthrough 含 key 写回/不含 key
     逐字节不变；
   - 拦截点：开关开/关、status 边界（399/400）、网关自生成标记跳过、
     超限截断、unrecognized 两分支；
   - SSE 过滤器：首事件错误/流中错误/`[DONE]` 正常结束/截断/心跳透传/
     多行 data/畸形事件、零分配基准；
2. **集成测试**（`tests/integration/implementation/scenario-SC27-upstream-error-normalization/`，
   设计文档 `tests/integration/测试设计文档/scenario-SC27-上游错误体归一/`）：mock 上游三协议
   envelope 错误与 SSE 四类场景，断言客户端错误体、状态码、截断标记、usage 计费不受影响；
   fallback 场景断言重试中不归一、最终结果归一（含配置归属镜像对照）；
3. **回归**：`make test` 全绿（不加配置 = 现状行为）；SC02/SC05/SC08/SC25 集成场景回归；
   `mod_body_process` 流式 usage 回归；开启过滤器前后 TTFT/TPOT 对比。

## 7. 灰度与回滚

| 阶段 | 动作 | 观察 |
|---|---|---|
| 1 | 测试集群开 `Enabled` + `UnrecognizedAction=passthrough` | `ai_err_normalize_miss` 分布，补全映射表 |
| 2 | 单厂商生产镜像集群开 `Enabled` | 归一命中率为 expected；客户端错误处理兼容（行内对接方确认） |
| 3 | 全量集群开 `Enabled`；镜像集群追加 `StreamEnabled` | `ErrStreamTruncated` 基线与误报面 |
| 4 | 全量开 `StreamEnabled` | TTFT/TPOT 对比、截断告警阈值校准 |

回退：热加载撤销 `Enabled`/`StreamEnabled` 即恢复（配置零值=关闭，无状态
残留）；异常时回滚镜像同样安全（未开启配置的集群行为与旧版本逐字节一致）。

## 8. 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| 状态码重映射影响行内存量客户端/拨测 | 误告警、重试逻辑错配 | §5 兼容公告；灰度前与行内对接方确认；必要时评估"5xx 保留上游状态码"兼容档位（仅归一 body） |
| 上游错误形态漂移导致识别率不足 | miss 样本积累 | `ai_err_normalize_miss` 标记 + passthrough 兜底（永不误改）；映射表迭代机制 |
| SSE 过滤器热路径开销 | TTFT/TPOT 劣化 | 零/近零分配实现；过滤器仅在开关开启时挂接；阶段 3 强制性能对比门槛 |
| 脱敏误伤良性内容 | 错误消息可读性下降 | fail-open + 掩码只作用于 key 形态子串；"宁可过度"是显式接受的权衡 |
| 网关自生成标记遗漏某条本地错误路径 | 本地错误被二次归一 | 标记加在 `CreateErrorResponse`/`CreateInternalSrvErrResp` 等公共构造点（非各调用点）；单测覆盖全部本地错误路径断言带标记 |
| 控制面（ai-gateway-api）未配套导致配置无法下发 | 功能空转 | 配置链路透明：控制面先于或后于数据面发布均安全（零值=关闭）；BFE 侧 `conf/` 手工样例可先行验证 |
