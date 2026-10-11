# BFE Issue #1409 修复方案：非 2xx 响应的估算 token 残留污染访问日志

- Issue: https://github.com/bfenetworks/bfe/issues/1409
- 缺陷：pb3 访问日志中 `res_status_code=404` 的行带有非零 `ai_input_tokens` / `ai_output_tokens`，而同行的 `ai_total_tokens=0`、`ai_cost_value` 缺失；404 请求未成功调用上游，不应产生任何模型用量。计费侧无损失（`cost=0`），污染的是基于访问日志的用量统计 / 对账 / 报表。
- 代码库：`bfe/`，本文同时适用于 **v1.8.8 分支**（tip `8cd04c82`）与 **v1.8.9-dev 分支**（tip `1e27573d`）；正文行号以 v1.8.8 为准，v1.8.9-dev 的锚点、额外 404 来源与移植验证见 §九。部署侧对应镜像 `ghcr.io/rainway-ai-gateway/bfe:v1.8.8`（`ai-gateway/VERSIONS.yaml`），`EstimateToken = true`（`ai-gateway/conf/bfe.conf:32`、`bfe_config/bfe_conf/conf_basic.go:120` 出厂默认）。
- 关联：#1398（计费/日志视图隔离：守卫只清计费副本）、#1401（`-1` 哨兵在日志边界夹取为 0）、#1352（估算只在响应完成时可计费）、#1364。与 #1406 **无同一根因**（#1406 是 200 响应的 usage 采集链断裂，本文是**非 2xx** 的估算残留）。

## 一、口径定义（Expected 的来源）

### 1.1 三个字段的数据源

`bfe_modules/mod_access_pb3/request_log.go:431-443`，全部直读共享对象 `AiBasicInfo.TokenUsage`：

| 日志字段 | 数据源 | 写入条件 |
|---|---|---|
| `ai_input_tokens` | `TokenUsage.PromptTokens` | **始终写** |
| `ai_output_tokens` | `TokenUsage.CompletionTokens` | 始终写；负值在日志边界夹取为 0（`request_log.go:438-441`，issue #1401） |
| `ai_total_tokens` | `TokenUsage.UsedQuota` | **始终写** |
| `ai_cost_value` | `TokenUsage.UsedCost` | 仅 `> 0` 时写（`request_log.go:469`） |

### 1.2 仓库自身已声明的口径（本文的 Expected 依据）

`docs/zh_cn/sys_design/rmb_quota.md:649`（issue #1398 说明）：

> 无论何种场景，**访问日志与统计读共享 `TokenUsage`，保留真实解析值或批准口径的估算值**，不再被守卫抹零。

据此，非 2xx（未成功调用上游）行的 Expected 为：

- **A-01**：无真实 usage ⇒ `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` 为 0（或不记录），`ai_cost_value` 缺失；
- **A-02**：若该行报告了 token 用量，则必须是**同一口径下自洽的三元组**——`ai_total_tokens = ai_input_tokens + ai_output_tokens`（token 计费口径；图片/视频按次计费的行例外，见 §七）。

即：**日志的 token 字段只允许承载「真实解析值」或「已被计费口径批准的估算值」；两者都不是时，一律不输出。**

## 二、根因（四层，缺一不可）

### 第 1 层（必现、现场直接原因）：鉴权阶段无条件播种 `PromptTokens`

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`：

```go
408: promptToken := 0
409: if meta.IsAllowEstimateToken() {
410:     promptToken = int(GetPromptToken(req))      // req.ContentLength/4 或 len(body)/4
411: }
412: SetTokenAuthContext(req, tok, int64(promptToken), tok.Tags)
```

`SetTokenAuthContext`（`:564-576`）把该值写进共享 `TokenUsage.PromptTokens`，同时把 `CompletionTokens` 置为哨兵 `-1`。播种发生在 `HandleFoundProduct`（注册点 `:472`），**当时完全不知道最终响应状态**：

- 估算开关只看 `EstimateToken`（`bfe_config/bfe_conf/conf_basic.go:120` 默认 true，现场为 true）；
- `GetPromptToken`（`:588-603`）= 请求体长度 / 4。

⇒ 任何**通过鉴权但最终未成功调用上游**的请求（404 是典型），共享对象里都已经躺着 `PromptTokens = req_body_len/4`，且此后没有任何路径清理它。这正是 issue 现场"`ai_input_tokens ≈ req_body_len/4`"的来源，也与 issue 给出的部署侧行号 `mod_ai_token_auth.go:406`（本分支 tip 上为 `:409`）一致。

### 第 2 层（口径断裂的结构性根因）：估算只写 `CompletionTokens`，不写 `UsedQuota`

`bfe_modules/mod_body_process/content_quota_usage.go:107-113`：

```go
107: if tctx.UsedQuota <= 0 && caf.aiBasicInfo.IsAllowEstimateToken() {
108:     // still not got usage, estimate from content length
109:     if tctx.CompletionTokens == -1 {
110:         tctx.CompletionTokens = 0
111:     }
112:     // 累加事件的token数
113:     tctx.CompletionTokens += curCompletionToken     // 只累加 CompletionTokens！
114: }
```

`curCompletionToken` 来自事件字节数估算：`EstimateContentToken(val) = len(val)/4`（`bfe_model_protocol/utils/usage_parse.go:307-309`；调用点 `mod_body_process/llm_util.go:144`、`mod_body_process/body_process.go:461`）。对"整条 body 单事件"的非流式响应，累加值恰为 **响应体字节数 / 4**。

而 `UsedQuota`（= 计费与 `ai_total_tokens` 的口径）只在两条路径被写入：

1. 真实 usage 解析：`UpdateCtxByUsage`（`mod_ai_token_auth.go:123-183`，`used > 0` 或 `prompt/completion > 0` 时同步写 Prompt/Completion/UsedQuota）；
2. 被批准的估算：`tokenReadResponseHandler`（`:210-247`，条件 `res.StatusCode == 200 && isCompleteNonStreamBody(res)`）或请求结束时的估算兜底（`:329-331`）。

⇒ **只要估算值在"未批准"状态下留在共享对象，就必然产生 `in>0 / out>0 / total=0` 的断裂三元组**——这正是 A-02 观测到的现象，也是它的结构性根因（不是某一处的笔误）。

### 第 3 层（清理缺口）：请求结束守卫对非 2xx 直接返回，且只清"计费副本"

`mod_ai_token_auth.go:275-278`：

```go
275: if res == nil || res.StatusCode != bfe_http.StatusOK {
276:     // only count used quota for successful requests
277:     return bfe_module.BfeHandlerGoOn
278: }
```

非 2xx 连守卫都不进（计费正确性因此没问题），但**也没有任何清理**。对"不可计费的 200"（客户端中断、流式被截断、无 usage 且未完成），守卫也只作用于计费副本 `billingUsage := *tokenUsage`（`:314-328`），共享对象按 #1398 的设计刻意保留观测值/估算值（`:332-337` 仅把**已批准**的估算镜像回共享 `UsedQuota`）。

⇒ 共享对象 = 日志视图，**永远不会被清理**；清理只影响计费副本。

### 第 4 层（掩盖）：`-1` 哨兵被日志边界夹取为 0

`SetTokenAuthContext:570` 把 `CompletionTokens` 预置为 `bfe_basic.COMPLETION_TOKENS_UNKNOWN`（-1）。404 行从未被任何响应路径覆盖，因此在含 #1401 的构建里被夹取成 `0`（`request_log.go:438-441`），在不含 #1401 的构建（如 tag `v1.8.8`，2026-09-24）里直接记为 `-1`。

⇒ 该夹取把"从未获得 usage"伪装成一个合法的 0 值，使 `in>0 / out=0 / total=0` 看起来只是"输出为 0"，掩盖了第 1~3 层。

## 三、404 行的来源枚举，以及"输出 = 响应体长度/4"的取证结论

### 3.1 v1.8.8 分支上 AI 流量可能出现的 404

| # | 来源 | 位置 | 是否调用过上游 |
|---|---|---|---|
| (a) | **AI 路由未命中**（`AI route not found`，text/plain，BFE 本地生成） | `bfe_server/reverseproxy.go:1221-1232` | 否 |
| (b) | **上游 404 直通**（404 不在 fallback 白名单，不再降级） | `reverseproxy.go:1876-1883`（白名单）+ `:1885-1907`（判定） | 是，但失败 |

> 本分支不存在其它 404 生成点：`bfe_basic.ErrorCodeToStatusCode` 无 404 取值，`mod_ai_batch` / 上游错误归一化（`reverseproxy_ai_error.go`）均为 v1.8.9-dev 新增，不在 v1.8.8。故现场 404 必为 (a) 或 (b)。

`(a)` 尤其重要：`HandleFoundProduct`（`:1199`，鉴权在此执行）**先于** AI 路由检查（`:1221`），所以"路径没有 ai_route 规则、但产品/主机命中 token 规则"的请求会**先播种 `PromptTokens`、再拿到 404**，且全程没有上游调用——与现场"`in ≈ req_body_len/4`、`total=0`、`cost` 缺失"完全吻合。可用 `ai_route_rule_hits` 为空、`ai_cluster_key_names` 为空、`backend_first`/`backend_retry` 缺失来辨识这一子集。

### 3.2 "输出 ≈ res_body_len/4" 在本分支源码上不可达（需现场核对部署二进制）

按 §二第 2 层，产生"`out = 响应体字节数/4` 且 `total=0`"的唯一写入者是 `QuotaUsageProcessor.Process`（`content_quota_usage.go:107-113`），而该处理器的创建被**成功状态门槛**拦住：

```go
27: func NewQuotaUsageProcessor(req *bfe_basic.Request, res *bfe_http.Response) *QuotaUsageProcessor {
28:     if res.StatusCode != bfe_http.StatusOK {
29:         // only count used quota for successful requests
30:         return nil
31:     }
```

`DoResponseProcess`（`mod_body_process/body_process.go:309-377`）只接受 `ccq != nil` 或 `conf != nil`，而 `HandleReadResponse` 在整条链上**只被调用一次**（`reverseproxy.go:1428-1440`，在 fallback 循环收敛之后），所以过滤器看到的 `res` 与日志记录的 `res` 是同一对象、同一状态。结论：

- 在本仓库 **v1.8.8 分支**（tip `8cd04c82`、父提交、tag `c95df40a` 均已核对）上，非 2xx 响应的 `CompletionTokens` 只能是哨兵 `-1`（或被夹取的 `0`），**不可能**是正数；
- 现场出现"`out ≈ res_body_len/4` 且 `total=0`"，只能是**部署二进制与分支源码不一致**造成的，最可能是 `mod_body_process` 用了无状态码门槛的旧实现——本分支里还保留着它的注释版：`body_process.go:379-423`（`p := NewCalcCompletionQuota(req)`，**只接收 req，没有状态门槛**）。

**需要 owner 执行的最小取证（不阻塞修复落地）**：

1. 取部署镜像 `ghcr.io/rainway-ai-gateway/bfe:v1.8.8` 的实际构建 commit / 私有补丁，比对 `bfe_modules/mod_body_process/content_quota_usage.go` 是否含 `:28` 的 200 门槛、`body_process.go` 是否为 `NewCalcCompletionQuota(req)` 旧变体；
2. 复核 CSV（`tmp/bfe_ai_request_log_20261010.csv`）的 `ai_output_tokens` 列是否确实取自同一行的 `AiOutputTokens`（而非导出脚本派生/列错位）；
3. 抽样该 43 行的 `ai_route_rule_hits` / `ai_cluster_key_names` / `backend_retry`，确认落在 (a) 还是 (b)。

**本修复方案不依赖上述判别结果**：无论残留来自"鉴权播种"还是"响应侧累加"，步骤 1~3 都能保证它不再进入访问日志的 token 字段。

## 四、修复步骤

设计原则：

1. **日志口径 = 真实解析值 / 批准口径估算值，其余不输出**（与 `rmb_quota.md:649` 一致），非 2xx 行因此天然无 token 字段；
2. **不改计费行为**：非 2xx 本就零扣费，`#1398` 的"日志/计费视图隔离"保持不变；
3. **不依赖残留来源判定**：播种残留与累加残留都被覆盖；
4. 不变量由构造保证，并加断言兜底（发现新的口径断裂立即告警而非静默）。

### 步骤 1（必改，核心）：在请求结束阶段确定"usage 处置"，由日志按处置输出

**1.1 `bfe_basic/request_ai_basic.go`：新增 usage 处置状态**（`AiBasicInfo` 结构 `:105-121`）

```go
// Token-usage dispositions for the access log (issue #1409).
const (
    // UsageDispositionUnset is the zero value: no disposition was judged
    // (the request did not go through token authentication, or the module
    // was not loaded). The access-log fields keep their legacy behaviour.
    UsageDispositionUnset = ""
    // UsageDispositionNone means the request never successfully called the
    // upstream (non-2xx response), so no model usage can exist: the access
    // log must not report the token fields.
    UsageDispositionNone = "none"
    // UsageDispositionObserved means the upstream was successfully called
    // and the shared TokenUsage holds the real parsed usage or the
    // EstimateToken estimate of this request (issue #1398 keeps that view
    // observable); the access log reports it as-is.
    UsageDispositionObserved = "observed"
)

func (aiinfo *AiBasicInfo) SetUsageDisposition(d string) { aiinfo.usageDisposition = d }
func (aiinfo *AiBasicInfo) UsageDisposition() string     { return aiinfo.usageDisposition }
```

> 设计取舍：只有 `none`（未成功调用上游）会抑制输出；**200 但未获批准口径的估算值（客户端中断、流式被截断等）仍按 #1398 的观测契约输出**——`tests/integration/.../scenario-SC29-usage-snapshot-billing` 的 TC-03 明确断言该形态必须保留 `ai_input_tokens`（详见 §七）。

**1.2 `mod_ai_token_auth`：在请求结束阶段写下处置**（`mod_ai_token_auth.go`）

- 非 2xx / `res == nil` → `SetUsageDisposition(UsageDispositionNone)`，并顺手清理残留（步骤 2）；该判定**前置到 `/count_tokens` 跳过之前**，保证任何非 2xx 出口都被覆盖；
- 2xx 且 token 鉴权上下文存在（`ctx != nil`）→ `SetUsageDisposition(UsageDispositionObserved)`；
- `/count_tokens`、`ctx == nil`（未命中 token 规则）、非 token 鉴权流量 → 保持 `Unset`，行为与修复前一致（这些场景没有播种，也没有残留）。

**1.3 `bfe_modules/mod_access_pb3/request_log.go`：按处置输出 + 不变量断言**

```go
usage := aiInfo.GetTokenUsage()
if usage != nil && aiInfo.UsageDisposition() != bfe_basic.UsageDispositionNone {
    reqLog.AiInputTokens = proto.Int64(usage.PromptTokens)
    outputTokens := usage.CompletionTokens
    if outputTokens < 0 { // 哨兵兜底：#1401 的夹取保留
        outputTokens = 0
    }
    reqLog.AiOutputTokens = proto.Int64(outputTokens)
    reqLog.AiTotalTokens = proto.Int64(usage.UsedQuota)
    // cache/audio/image/video 字段保持现状条件（>0 才写）
    ...
    usageInconsistent = usageTripleInconsistent(usage, outputTokens)
} else if usage != nil {
    // 未成功调用上游：token 字段全部不写（proto optional ⇒ 字段缺席，
    // 与"0 用量"区分开），下游 SUM(COALESCE(...,0)) 聚合不受影响。
    usageSuppressed = true
}
```

要点：
- 字段**缺席**而非写 0，是刻意的：让"未产生用量"与"用量为 0"在日志层可区分（proto 是 `optional`，`bfe-access-pb` 无需改动）；
- `reqAiInfoGen` 保持包级函数，新增 `(usageSuppressed, usageInconsistent bool)` 返回值（既有调用点/单测不受影响，Go 允许忽略返回值），由 `requestLogGen` 计数：`ModuleAccessLogState` 新增 `AiUsageSuppressed` / `AiUsageInconsistent` 两个 `metrics.Counter`；
- `#1401` 的夹取保留为兜底，其单测（`request_log_test.go` 的负值哨兵用例）继续通过。

**不变量断言的边界**（`usageTripleInconsistent`）：仅当 `UsedQuota > 0` 且非图片/视频按次计费行时校验 `UsedQuota == PromptTokens + CompletionTokens`。`total=0` 而 `in/out>0` 的形态在 200 行上是 #1398 设计内的观测视图（SC29 TC-03 断言），不在此告警；非 2xx 行则由处置 `none` 整体抑制、计入 `AiUsageSuppressed`。

> 修复前的日志创建路径 `mod_access_pb3.go` 的模块顺序（v1.8.8 分支 `bfe_modules.go:150` < `:163`）保证 `mod_ai_token_auth` 的处置先于日志生成写入，无需调整注册顺序。

### 步骤 2（必改，源头止血）：非 2xx 且无真实 usage 时清理共享对象中的估算残留

在 `tokenRequestFinishHandler` 的**非 2xx 早返回分支内**（`mod_ai_token_auth.go`）调用新增的 helper `m.dropUnbilledEstimate(req, res)`：

```go
func (m *ModuleAITokenAuth) tokenRequestFinishHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
    if res == nil || res.StatusCode != bfe_http.StatusOK {
        // only count used quota for successful requests; a non-2xx response
        // means the upstream was never successfully called, so the request
        // must not report model usage (issue #1409).
        m.dropUnbilledEstimate(req, res)
        return bfe_module.BfeHandlerGoOn
    }
    // Skip token-count endpoints which should never be billed.
    if strings.Contains(req.HttpRequest.RequestURI, "/count_tokens") { ... }

    ctx := GetTokenAuthContext(req)
    if ctx == nil { ... }
    // 2xx：共享 TokenUsage 承载本请求的观测/估算值，可被访问日志输出
    ctx.aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionObserved)
    ...
}

// dropUnbilledEstimate: 非 2xx 出口统一处置
func (m *ModuleAITokenAuth) dropUnbilledEstimate(req *bfe_basic.Request, res *bfe_http.Response) {
    aiBasicInfo := req.GetAiBasicInfo()
    if aiBasicInfo == nil { return }
    aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionNone)

    if aiBasicInfo.IsFinalUsageSeen() {
        // 非 2xx 但确有真实 usage：保留观测值，仅计数 + 告警，
        // 不作为失败请求的用量报告。
        m.state.Non2xxRealUsageKept.Inc(1)
        log.Logger.Warn(...)
        return
    }
    usage := aiBasicInfo.GetTokenUsage()
    if usage.PromptTokens == 0 && usage.CompletionTokens <= 0 && usage.UsedQuota == 0 {
        return // 无残留可清
    }
    // 清 Prompt/Completion/cache/audio/image/video/UsedQuota（全部估算字段）
    ...
    m.state.Non2xxEstimateDropped.Inc(1)
    log.Logger.Warn("%s: drop estimated usage of non-2xx response: status=%d cluster=%s model=%s req_body_len=%d", ...)
}
```

要点：
- 清理只作用于**估算字段**，且以 `!IsFinalUsageSeen()` 为前提：真实观测值（#1398 的核心诉求）不受影响；
- 该清理**不改变计费**：非 2xx 本就不扣费（`:275-278`），且本模块的 finish 处理先于 `mod_ai_rate_limit`（`bfe_modules.go:150` < `:160`）、`mod_access_pb3`（`:163`），后者只读 `UsedQuota`（计费与限流对账），非 2xx 行该值本就是 0；
- 与步骤 1 的关系：步骤 1 保证"日志不会输出残留"，步骤 2 保证"共享对象本身不含残留"，二者互为纵深防御（也为将来新增的消费者兜底）。

### 步骤 3（必改，独立正确性修复）：补齐 / 固化 `QuotaUsageProcessor` 的"成功状态"门槛

现场"`out = res_body_len/4`"的必要条件是处理器在非 2xx 上被创建（§3.2）。请按取证结果二选一：

- **若部署二进制确无 `content_quota_usage.go:28` 的门槛**（私有补丁/旧变体）→ 立即补齐（该门槛在本分支与 tag 上均存在，属回归），并对私有补丁做全量 diff 复核；
- **无论来源如何**，都补单测把该语义固化（已实施）：扩展既有 `TestNewQuotaUsageProcessorNonOK` 覆盖 400/404/500，并新增 `TestDoResponseProcessNon2xxNoCompletionEstimate`（EstimateToken 开启下驱动 404 响应体通过 `DoResponseProcess`，断言 `CompletionTokens` 仍为哨兵、`UsedQuota` 仍为 0），避免后续改动再把估算器接到错误响应上。

### 步骤 4（加固，可选）：估算字段显式标注来源 —— 本次未实施

第 2 层的歧义根源是"估算值"与"真实值"共用同一批字段。原本建议在 `AiBasicInfo` 上区分 `real` / `estimate` 来源。**本次未实施**，原因：

- SC29 TC-03 明确要求「客户端中断（200、无最终 usage）」的日志保留鉴权播种的估算值（#1398 的观测契约），因此日志层无法按"真实/估算"分流；
- 本次修复只需要区分"是否成功调用上游"（`none` / `observed`），已在步骤 1 实现；
- 若后续需要监控"估算口径占比"，可在此基础上增加来源标记与计数，属独立改动。

### 步骤 5（可观测）

| 模块 | 计数器 | 语义 |
|---|---|---|
| `mod_ai_token_auth` | `Non2xxEstimateDropped` | 非 2xx 且无真实 usage，删除了估算残留 |
| `mod_ai_token_auth` | `Non2xxRealUsageKept` | 非 2xx 但确有真实 usage（应极少，出现即需排查上游语义） |
| `mod_access_pb3` | `AiUsageSuppressed` | 日志中 token 字段被按口径抑制的行数（用于验证与对账） |
| `mod_access_pb3` | `AiUsageInconsistent` | 三元组口径断裂（应为 0，非 0 立即告警） |

### 步骤 6（文档同步）

- `docs/zh_cn/sys_design/rmb_quota.md`：§7.4 补充"`QuotaUsageProcessor` 仅当响应为 200 时才创建，非 2xx 不产生任何用量口径"；§7.5 的判定表增加"非 2xx（未成功调用上游）⇒ 零扣费且日志 token 字段不输出"一行，并新增 issue #1409 说明段（已实施）。

## 五、回归测试

单元测试（已实施，放在被测代码旁）：

1. `bfe_modules/mod_access_pb3/request_log_test.go`
   - 新增 `TestReqAiInfoGenSuppressesUsageOnNon2xx`：`res.StatusCode=404` + 处置 `none` + 播种 `PromptTokens=100` + 残留 `CompletionTokens=4` → 断言 `AiInputTokens` / `AiOutputTokens` / `AiTotalTokens` / `AiCostValue` **均为 nil**（字段缺席）、返回 `usageSuppressed=true`，而 `ai_apikey_id` 等非用量字段照常输出；
   - 新增 `TestReqAiInfoGenDetectsInconsistentUsageTriple`：`total=200` 而 `in+out=104` → 返回 `usageInconsistent=true`；一致三元组、图片/视频按次行、`total=0` 的 #1398 观测行均不告警；
   - 既有 `TestReqAiInfoGenNegativeCompletionTokensClamped`（`-1` 哨兵夹取）等保持通过。
2. `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`
   - 新增 `TestTokenRequestFinishHandler_Non2xxDropsEstimateResidue`：404 + 播种 prompt + 残留 completion → 共享 `TokenUsage` 全零、处置 `none`、`Non2xxEstimateDropped` +1、无 Redis 扣减；
   - 新增 `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation`：500 + `MarkFinalUsageSeen()` → 观测值保留、处置 `none`、`Non2xxRealUsageKept` +1、无扣减；
   - 新增 `TestTokenRequestFinishHandler_ObservedDispositionKeepsLogView`：200 + 客户端中断（无最终 usage）→ 处置 `observed`、播种估算值保留在共享对象（#1398 日志视图）、零扣减。
3. `bfe_modules/mod_body_process/content_quota_usage_test.go`
   - 扩展 `TestNewQuotaUsageProcessorNonOK`：400/404/500 → 处理器为 nil；
   - 新增 `TestDoResponseProcessNon2xxNoCompletionEstimate`：EstimateToken 开启下把 404 响应体（`application/json`）驱动过 `DoResponseProcess` → `CompletionTokens` 仍为哨兵、`UsedQuota` 仍为 0（固化"非 2xx 不产生估算"的语义）。

集成测试（`bfe/tests/integration`，SC05 访问日志字段场景）：

4. 新增 `TestTC19_Non2xxNoUsageFields`（`sc05_access_log_ai_fields_test.go`，三臂）：
   - **臂 A（现场主形态）**：请求主机 `noroute.example.org`（`host_rule.data` 映射到 `ai_product`，但 `ai_route.data` 无对应规则）→ BFE 本地 404 `AI route not found`、后端 0 命中、token/cost 字段全部缺席；
   - **臂 B**：mock 后端返回 404（`MockBackend.Response/Body` 运行期改写）→ 404 直通、后端 1 命中、token/cost 字段全部缺席；
   - **臂 C（对照臂）**：同一后端返回 200 且响应体无 usage → 估算口径不变，`ai_total_tokens == ai_input_tokens + ai_output_tokens` 且两者 > 0；
   - 新增助手 `assertNoUsageFields`（字段缺席断言）与 `sendRequestNoKeepAlive`（逐请求独立连接，避免臂 A 的 `closeAfterReply` 造成连接复用伪失败）；testdata 仅新增一个主机名，不影响其它 TC。
5. 回归范围与实跑结果（v1.8.8 分支 + 本修复）：

| 范围 | 结果 |
|---|---|
| `go build ./...` / `go vet`（受影响包） | 通过 |
| `bfe_basic` / `bfe_server` / `mod_ai_token_auth` / `mod_access_pb3` / `mod_body_process` / `mod_ai_route` 单测 | 全部通过 |
| SC05（19 个 TC，含新增 TC-19） | 通过 |
| SC29（#1398 usage 快照 / 客户端中断日志视图） | 通过 |
| SC02（多 API-Key）、SC03（RMB 配额）、SC11（token auth 计费）、SC12（客户端中断） | 均通过 |

验证命令（本次实跑）：

```bash
cd bfe
go build ./... && go vet ./bfe_basic/ ./bfe_server/ ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_modules/mod_body_process/
go test -count=1 ./bfe_basic/ ./bfe_server/... ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_modules/mod_body_process/ ./bfe_modules/mod_ai_route/
go test -count=1 ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/
go test -count=1 ./tests/integration/implementation/scenario-SC29-usage-snapshot-billing/
go test -count=1 ./tests/integration/implementation/scenario-SC02-multi-api-key/ ./tests/integration/implementation/scenario-SC03-rmb-quota/ ./tests/integration/implementation/scenario-SC11-ai-token-auth-billing-fix/ ./tests/integration/implementation/scenario-SC12-client-abort-billing/
```

## 六、修复后目标行为

以 §3.1(a)（无 AI 路由的 404，现场主形态）为例：

1. `HandleFoundProduct`：鉴权通过，`PromptTokens` 播种为 `req_body_len/4`（本身是既有设计，用于限流预估，不变）；
2. AI 路由检查失败 → 404（`reverseproxy.go:1228`）；
3. `HandleReadResponse`：`mod_ai_token_auth` 因非 200 跳过，`mod_body_process` 不创建 `QuotaUsageProcessor` → `UsedQuota` 保持 0；
4. 请求结束：非 2xx 出口 → 处置 `None` + 清理估算残留（`Non2xxEstimateDropped` +1）+ 一条 Warn；
5. 访问日志：`res_status_code=404`，`ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` **全部缺席**，`ai_cost_value` 缺席，`ai_stream` 照常；
6. 200 且真实 usage/批准估算的行：三字段齐备，`total = in + out`（A-02 不变量成立）；
7. 计费与扣减完全不变（非 2xx 本就零扣费）。

## 七、边界与残余风险

- **抑制范围（已决策：仅非 2xx）**：修复后只有"未成功调用上游"（非 2xx / 无响应）的行不输出 token 字段。"200 但未获批准口径的估算值"（客户端中断、流式截断等）**保持输出**——`scenario-SC29-usage-snapshot-billing` 的 TC-03 明确断言该形态必须保留 `ai_input_tokens`（#1398 的观测契约），因此本修复没有采用"所有未批准估算一律抑制"的严格变体；这类行仍可能出现 `total=0` 而 `in/out>0`，不计入 `AiUsageInconsistent`（见步骤 1.3 的边界说明）。若后续产品口径要求连这类观测值也收窄，需同步修改 SC29 TC-03 与 `rmb_quota.md:649` 的口径表述。

- **聚合兼容**：字段缺席对 `SUM(COALESCE(ai_*,0))` 类聚合等价于 0（`ai-gateway/conf/doris/05_create_insert_job.sql:49-51`），报表口径不变；下游若用 `COUNT(ai_input_tokens)` 统计"有用量的行"，语义反而更正确。

- **不变量断言的例外**：图片/视频按次计费的行 `ai_total_tokens` 记的是张数（`ImageCount` / `VideoCount`），不满足 `total = in + out`，断言需排除（步骤 1.3 已体现）。

- **部署一致性风险（最高优先级）**：本分支任何版本都**不可能**在非 2xx 上产生正数 `ai_output_tokens`，现场证据（`out = res_body_len/4`）指向部署二进制与分支源码不一致（§3.2）。若部署确有私有补丁，务必同步修复补丁版本，否则现场可能继续产生其它绕过日志口径的写入路径。

- **未改动**：计费金额、Redis 扣减、fallback/重试策略、转发字节、模块注册顺序、`EstimateToken` 语义（限流预估仍使用播种值）均不变。

- **v1.8.9-dev 同步**：`develop` / `v1.8.9-dev` 上 `mod_access_pb3`、`mod_ai_token_auth`、`mod_body_process` 的行号不同（且新增了 `mod_ai_cache` / `mod_ai_batch` / 上游错误归一化等 404 生成点），本方案需在移植时按下述差异复核：缓存命中的 200（`goto send_response` 跳过 `HandleReadResponse`，属"无 usage"）、上游错误归一化把错误改写为 404（`reverseproxy_ai_error.go:155`）、`mod_ai_batch` 的 `CodeBatchFileForbidden=404`——三者都应被"处置 = None ⇒ 不输出 token 字段"的规则自然覆盖。

## 八、实施记录（2026-10-11，v1.8.8 分支）

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `usageDisposition` 字段 + `UsageDispositionUnset/None/Observed` 常量 + `SetUsageDisposition/UsageDisposition` |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 新增 `dropUnbilledEstimate`（非 2xx：处置 `none`、清除估算残留、真实 usage 只计数保留）；`tokenRequestFinishHandler` 非 2xx 分支前置到 `/count_tokens` 跳过之前，2xx 分支写入处置 `observed`；state 新增 `Non2xxEstimateDropped` / `Non2xxRealUsageKept` |
| `bfe_modules/mod_access_pb3/request_log.go` | `reqAiInfoGen` 返回 `(usageSuppressed, usageInconsistent)`；token 字段按处置输出（`none` ⇒ 字段缺席）；新增 `usageTripleInconsistent` 不变量校验 |
| `bfe_modules/mod_access_pb3/mod_access_pb3.go` | `ModuleAccessLogState` 新增 `AiUsageSuppressed` / `AiUsageInconsistent`，由 `requestLogGen` 计数 |
| `bfe_modules/mod_access_pb3/request_log_test.go` | 新增 2 例（非 2xx 抑制、三元组不变量） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 新增 3 例（残留清除、真实 usage 保留、2xx 观测处置保持日志视图） |
| `bfe_modules/mod_body_process/content_quota_usage_test.go` | 扩展非 2xx 门槛用例（400/404/500）+ 新增 `TestDoResponseProcessNon2xxNoCompletionEstimate` |
| `docs/zh_cn/sys_design/rmb_quota.md` | §7.4 补 `QuotaUsageProcessor` 的 200 门槛；§7.5 判定表新增非 2xx 行 + issue #1409 说明段 |
| `docs/zh_cn/modifications/2026-08-19-update-ai-access-log-fields/design-changes.md` | 新增 §10：非 2xx 响应的 token 字段语义与不变量 |
| `tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增 `TestTC19_Non2xxNoUsageFields`、`assertNoUsageFields`、`sendRequestNoKeepAlive`、`noRouteHost` |
| `tests/integration/.../scenario-SC05-access-log-ai-fields/testdata/server_data_conf/host_rule.data` | `ai_product` 主机列表新增 `noroute.example.org`（无 ai_route 规则，用于构造 BFE 本地 404） |
| `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/TC-19-非2xx响应不输出token字段.md` | 新增 TC-19 设计文档 |
| `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/场景说明.md` | TC 列表新增 TC-19 |
| `tests/integration/测试设计文档/测试场景总体说明.md` | SC05 场景清单（TC 数 16 → 19）与 TC 列表补齐 TC-17/TC-18/TC-19 |

**未实施项**：步骤 4（估算/真实来源标记，理由见步骤 4 小节）；上游错误归一化、`mod_ai_cache`、`mod_ai_batch` 等 v1.8.9-dev 才有的 404 生成点不在本分支，移植时按 §七 最后一条复核。

**未验证项**：现场 43 行 CSV 与部署镜像源码的一致性（§3.2 需 owner 取证）；部署侧若确有私有补丁，须同步修复补丁版本。

## 九、v1.8.9-dev 移植记录（2026-10-11）

同一修复已移植到 `v1.8.9-dev`（tip `1e27573d`），实现与 v1.8.8 完全一致，仅锚点行号不同。

### 9.1 锚点对照（v1.8.9-dev）

| 位置 | dev 行号 | 说明 |
|---|---|---|
| `bfe_basic/request_ai_basic.go` | `finalUsageSeen` `:198`、`IsFinalUsageSeen` `:231` | 处置字段与常量紧随其后新增 |
| `mod_ai_token_auth/mod_ai_token_auth.go` | `ModuleAITokenAuthState` `:54-63`、`tokenRequestFinishHandler` `:295` | 新增 2 个计数器；非 2xx 分支前置到 `/count_tokens` 跳过之前，`dropUnbilledEstimate` 紧随其后，2xx 置 `observed`（位置在 `ctx` 判空之后、batch/cache 早返回之前） |
| `mod_access_pb3/request_log.go` | `requestLogGen` 调用点 `:77`、`reqAiInfoGen` `:382`、token 段 `:430-471` | 返回 `(usageSuppressed, usageInconsistent)`；`usageTripleInconsistent` 置于文件末尾（rate-limit 段之后） |
| `mod_access_pb3/mod_access_pb3.go` | `ModuleAccessLogState` `:38-41` | 新增 `AiUsageSuppressed` / `AiUsageInconsistent` |
| `mod_body_process/content_quota_usage.go` | `NewQuotaUsageProcessor` `:27-31` | 200 门槛与 v1.8.8 一致（无需改动，由单测固化） |

### 9.2 dev 特有的 404 来源（均被"处置 = none ⇒ 不输出 token 字段"覆盖）

| 来源 | 位置 |
|---|---|
| 上游错误归一化把上游错误改写为 404（`CodeUpstreamModelNotFound`） | `bfe_basic/request_ai_basic.go:508` + `bfe_server/reverseproxy_ai_error.go:155` |
| `mod_ai_batch` 的本地拒绝 `CodeBatchFileForbidden = 404` | `bfe_basic/request_ai_basic.go:488` |
| 其余同 v1.8.8：AI 路由未命中的本地 404、上游 404 直通 | `bfe_server/reverseproxy.go`（AI 路由检查）、`aiFallbackStatusCodes` |

### 9.3 dev 上的边界说明

- **AI 缓存命中**（`mod_ai_cache`，`AiCacheHit`）也是"未调用上游"，但它返回 200，因此按本方案的 `observed` 口径照旧输出播种的估算值（与 #1398 的 2xx 观测口径一致）。如需一并收窄，可在 `tokenRequestFinishHandler` 的 `AiCacheHit` 早返回处同样置 `UsageDispositionNone`（一行改动），但会改变现有缓存命中行的日志语义，需 owner 决策，本次**未实施**；
- **batch 结算行**（`batchContractHandler` 早返回）同样保持 `observed`（2xx），行为与修复前一致；
- 部署侧镜像与源码一致性的取证仍适用 §3.2（dev 上 404 行的 `ai_output_tokens` 同样只可能是哨兵夹取值，不可能是正数）。

### 9.4 dev 上的验证结果（本机实跑）

| 范围 | 结果 |
|---|---|
| `go build ./...`、`go vet`（受影响包）、`gofmt` | 通过 / 干净 |
| `bfe_basic`、`bfe_server`、`mod_ai_token_auth`、`mod_access_pb3`、`mod_body_process` 单测 | 全部通过 |
| 集成 SC05 TC-19（`TestTC19_Non2xxNoUsageFields`，三臂） | 通过 |
| 集成 SC05 全量（19 TC）、SC29 | 通过 |
| 集成 SC02、SC03、SC11、SC12 | 通过 |
| dev 特有回归：SC20（缓存命中）、SC25（缓存前置查询）、SC27（上游错误归一化）、SC28（batch） | 通过 |