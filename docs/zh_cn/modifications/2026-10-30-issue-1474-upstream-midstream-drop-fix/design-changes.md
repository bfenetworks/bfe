# BFE Issue #1474 修复方案：上游流式输出途中故障时不计费且客户端无感知

- Issue: https://github.com/bfenetworks/bfe/issues/1474
- 缺陷：流式输出途中后端进程异常退出（如 `kill -9` vLLM）时，chunked 流无终止 chunk，① 网关对这次请求**零扣费**；② 客户端收到 HTTP 200、流直接结束，**没有 `[DONE]`、没有错误事件**，无法判断回答是否完整。
- 代码库：`bfe/`（行号基于当前 HEAD `db122312`，2026-10-30，逐条核实）
- 关联：#1352（客户端中断零扣费，有意设计，本方案不动）、#1410/#1414（零计费可观测信号）、#1415（clean-EOF 完成标记）、`2026-10-06-upstream-error-normalization`（上游错误归一，含流内截断标记 `StreamTruncated`）。**本方案只补「上游异常断流」一环：错误归因、估算计费、客户端错误事件注入。**

## 一、根因

### 1.1 拷贝错误不区分来源，上游读错误被归因为"客户端写错误"

`sendResponse` → `copyResponse`（`bfe_server/reverseproxy.go:998-1043`）用 `io.CopyBuffer(dst, src, buf)`（`:1041`）一条语句同时完成"读上游"和"写客户端"，返回值不区分错误来自哪一侧。`send_response` 块对**任意**拷贝错误统一处理（`reverseproxy.go:1487-1497`）：

```go
err = p.sendResponse(rw, res, resFlushInterval, cancelOnClientClose)
if err != nil {
    // ...
    basicReq.ErrCode = bfe_basic.ErrClientWrite   // :1494 一刀切
    p.proxyState.ErrClientWrite.Inc(1)            // :1497
}
```

`kill -9` 后端 ⇒ 上游连接中断 ⇒ chunked 流无终止 chunk ⇒ **读错误** ⇒ 被标成 `CLIENT_WRITE`。于是上游断流与客户端主动中断（真的写失败）在 `err_code` 上完全同路径。

### 1.2 收尾计费：上游断流落入"客户端中断零扣费"分支

`tokenRequestFinishHandler`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:407-423`）：

```go
aborted := isClientAbortErr(req.ErrCode)   // ErrClientWrite/ErrClientClose/ErrClientReset
if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
    m.state.AbortBillingSkipped.Inc(1)                       // 计入"中断跳过"
    ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceAborted)
    // ... RMB 计 BillingZeroCost ...
    ctx.deducted = true                                       // 零扣费
    return
}
```

`kill -9` 的断流 `err_code=CLIENT_WRITE` 命中 `isClientAbortErr` ⇒ 与客户端主动中断完全同分支：零扣费、计入 `AbortBillingSkipped`（污染了客户端中断率指标）、`ai_usage_source=aborted`（污染了中断行级标记），连 `UsageUnrecognized` 信号都没有。而 `shouldMarkAIStreamCompleted`（`reverseproxy.go:1521-1525`）要求 `copyErr == nil`，断流时恒 false ⇒ `responseCompleted` 不置位 ⇒ 即使走到 `:442` 的 `estimateBillable = allow && completed` 也为假。两条路都到不了计费。

### 1.3 客户端无错误事件：截断检测只打标记，不注入事件，且默认不安装

上游错误归一的流过滤器（`bfe_server/reverseproxy_ai_error.go`）：

- **安装门控**：`normalizeUpstreamError`（`:49-88`）仅在集群开启 `NormalizeUpstreamError.StreamEnabled` 时才为 SSE 响应包装 `aiErrorStreamFilter`（`:70-81`）；默认关闭 ⇒ 绝大多数集群根本没有截断检测。
- **只标记不注入**：`finalize()`（`:451-466`）在 EOF 且未见终止事件时只置 `aiMeta.StreamTruncated = true` + `ErrStreamTruncated` 计数，**不向客户端流中写入任何错误事件**；且 `fill()`（`:324-354`）在源返回非 EOF 读错误时直接 `return err` 放弃，连 finalize 都不执行。
- 于是客户端看到的就是测试实测形态：HTTP 200、流静默结束、无 `[DONE]`、无错误事件。

### 1.4 现有 TC 为何不覆盖

SC12（中断计费）只覆盖客户端主动中断；SC27（截断标记）依赖 `StreamEnabled` 开启且只断言 `ai_stream_truncated` 标记；SC29（计费快照）的 mock 后端均干净收尾。「上游读错误中断流 + 计费 + 客户端错误事件」三件套无任何用例触达。

## 二、修复方案

设计原则：**"客户端写失败"与"上游读失败"是两类不同的事实，必须在传输层分开归因**；归因修正后，计费门控与观测信号自然各归其位；客户端错误事件按协议形态补发。修复不依赖 `NormalizeUpstreamError` 开关，不改动 #1352 的客户端中断语义。

### 步骤 1（必改）：copyResponse 错误归因

文件：`bfe_server/reverseproxy.go`。

为 `copyResponse` 增加错误侧分类。实现要点：

1. 用包装 writer 记录写侧错误：`dst = &writeTracker{w: dst, err: &writeErr}`，三条拷贝路径（`CopyWithoutBuffer` `:1024`、MaxLatencyWriter `:1031-1035`、默认 `io.CopyBuffer` `:1041`）都经同一 tracker。
2. 拷贝返回错误时按下表分类：

| 条件 | 归类 | ErrCode |
| --- | --- | --- |
| `cancelOnClientClose` 的 CloseWatcher 已触发（客户端先断，回调里 `src.Close()`，`reverseproxy.go:1005-1015`） | 客户端中断（**即使最终表现为读错误**） | `ErrClientWrite`（现状语义） |
| 写侧错误非空（`writeErr != nil`，或返回错误与 `writeErr` 同源） | 客户端写失败 | `ErrClientWrite`（现状语义） |
| 其余读错误 | **上游断流** | 新增 `ErrBkReadRespBody`（`bfe_basic/error_code.go`，与 `ErrBkReadRespHeader` 同族：`BK_READ_RESP_BODY`，"read response body error"） |

CloseWatcher 优先级最高是关键：**SSE 响应恒 `cancelOnClientClose=true`（`reverseproxy.go:1427`），客户端中断的典型形态正是 watcher 关闭 src 后的读错误**——不归因 watcher 会把客户端中断误判成上游断流，直接破坏 #1352 语义。跟踪标志需在 watcher 回调中置位。

`send_response` 块（`:1487-1497`）按分类打 `ErrCode` 与计数器：上游断流走新增的 `proxyState.ErrBkReadRespBody` 计数，不再污染 `ErrClientWrite` / `AbortBillingSkipped`。

### 步骤 2（必改）：上游断流按已转发内容估算计费

文件：`bfe_basic/request_ai_basic.go`、`bfe_server/reverseproxy.go`、`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`。

1. `AiBasicInfo` 新增标记 `upstreamDropped` + `MarkUpstreamDropped()` / `IsUpstreamDropped()`，语义：上游在响应体未完整交付时异常断开（区别于 `responseCompleted` 的"完整交付"与 `StreamTruncated` 的协议层截断观测）。
2. 置位点：`send_response` 块中，拷贝错误归类为上游断流 且 `res.StatusCode == 200` 且非 `ModeFile`/`ModeBatch` 时 `aiMeta.MarkUpstreamDropped()`（与 `shouldMarkAIStreamCompleted` 并列的纯判定函数，便于单测）。
3. 计费门控（`mod_ai_token_auth.go:442`）：

```go
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() &&
    (ctx.aiBasicInfo.IsResponseCompleted() || ctx.aiBasicInfo.IsUpstreamDropped())
```

   断流时共享 `TokenUsage` 上已有鉴权期 prompt 估算（`tokenFoundProductHandler`）与逐 chunk 的 completion 累加（`content_quota_usage.go:111-122`，只统计**已转发**内容，天然符合测试建议"按已转发的输出估算"）⇒ 走既有 `:486-494` 估算兜底与镜像链路，**不需要新的计费数学**，只放宽门控。客户端中断不受影响：其 `err_code` 仍是 `CLIENT_WRITE`，在 `:407` abort 分支提前返回，根本到不了门控。

4. 观测信号（与 #1410/#1414 同一模式）：
   - 新 `UsageSourceUpstreamDropped = "upstream_dropped"`（`request_ai_basic.go` UsageSource 常量组，`:332-347`），在 `:475-482` 的 source 分支中优先于 estimated/parsed 命中，访问日志 `ai_usage_source` 可区分；
   - `EstimateToken=false` 时断流请求不可静默：走 unrecognized 清零分支，但 `ZeroCostReason` 置新增的 `ZeroCostReasonUpstreamDropped = "upstream_dropped"`（区别于客户端中断的 `aborted`），计数仍入 `UsageUnrecognized` + `BillingZeroCost`；
   - 估算计费成功时新计数 `UpstreamDropBilled`（alertable），与 `AbortBillingSkipped` 彻底分账。

### 步骤 3（必改）：向上游断流的 SSE 响应注入协议错误事件

目标：客户端能明确知道"响应不完整"，而非静默 EOF。

**推荐方案 A：轻量透传注入过滤器（新增，独立于 aiErrorStreamFilter）**

文件：`bfe_server/reverseproxy_ai_error.go`（或新文件 `reverseproxy_stream_inject.go`）。

1. 新过滤器 `streamTruncationInjector`：平时**逐 Read 原样透传**（零缓冲、零块重组，无 TTFT/flush 时序副作用——这是与 `aiErrorStreamFilter` 的本质区别，后者为做错误事件改写需要按 SSE 块缓冲，不适合默认全量安装）；仅在源 `Read` 返回**非 EOF 错误**时：若该响应未见协议终止标记，则把预构造的协议错误事件帧追加到透传字节之后返回，随后向上返回原错误。
2. 终止标记判定：透传过程中按协议做廉价子串扫描（openai：`data: [DONE]`；anthropic：`event: message_stop` 或 `message_stop`；命中即抑制注入——上游已正常收尾、纯连接损坏时不多发一帧）；适配器声明 `StreamEndsAtEOF` 的协议（gemini，`bfe_model_protocol/gemini/errors.go:38-43`）整体豁免，与 `finalize()` 的豁免口径一致。
3. 错误事件构造：协议适配器新增能力接口（`bfe_model_protocol/utils`，能力位风格同 `StreamEndsAtEOFReporter`），各协议实现：
   - openai：`data: {"error":{"message":"...","type":"server_error","code":"upstream_stream_truncated"}}\n\n`（无 `event:` 行，OpenAI chat 流错误事件的既定格式）；
   - anthropic：`event: error\ndata: {"type":"error","error":{"type":"api_error","message":"..."}}\n\n`；
   - gemini：豁免，不实现。
   消息体复用统一 AiError 目录的 `CodeModelInternalError`/`CodeUpstreamUnknown` 语义；开启 `StreamEnabled` 的集群沿用既有归一 body，未开启的集群用默认 message（如 "upstream stream terminated abnormally."）。
4. 安装点：`normalizeUpstreamError`（`reverseproxy_ai_error.go:49-88`）之后、EPP body 过滤器（`reverseproxy.go:1431-1437`）之前，对 AI 模式 + `res.IsSse` + 200 响应包装。与 `aiErrorStreamFilter` 可叠放（injector 在内层，先看到原始读错误）。
5. 配置：`AIConf.NormalizeUpstreamError` 下新增 `StreamTruncatedError *bool`（指针语义，缺省 nil = **开启**，使修复对存量集群开箱生效；显式 false 关闭）；安装决策放宽为"`StreamEnabled` 或 `StreamTruncatedError` 任一开启"，但**错误事件改写**仍只受 `StreamEnabled` 控制，注入行为只受 `StreamTruncatedError` 控制，两个开关互不越权。

| 备选方案 | 排除原因 |
| --- | --- |
| B：扩展 `aiErrorStreamFilter.fill()` 在读错误路径注入（`:324-354` 现在直接 `return err`） | 需把过滤器默认全量安装，引入按 SSE 块缓冲的时序副作用（部分块延迟到 blank line 才透传），且截断标记/错误归一/注入三种语义挤在一个过滤器里，耦合错误归一开关 |
| C：在 `copyResponse` 里直接写字节 | copyResponse 是通用传输层，不认识 SSE/协议形态；且与 EPP 过滤器、错误归一过滤器的叠放顺序失控 |

**非流式边界**：200 非流式响应（`Content-Length` 或 chunked JSON）头已发出，状态码无法回改为 502，属 HTTP 固有限制。该形态下本方案交付：① 正确的 `err_code=BK_READ_RESP_BODY`；② 按已转发内容估算计费（同步骤 2，拷贝错误归因不区分流式/非流式）；③ BFE 不在拷贝错误后补发终止 chunk/定长收尾，客户端 HTTP 栈会得到截断传输错误而非"完整但截短的 200"。是否在拷贝错误时显式确保连接异常关闭（而非依赖各协议栈默认行为）在实施时核实并固定。

## 三、与既有语义的兼容性（安全边界）

- **#1352 客户端中断零扣费**：不受影响。客户端中断的判定链是"写失败 或 CloseWatcher 触发"（步骤 1 优先级表前两行），abort 分支（`mod_ai_token_auth.go:407-423`）照旧零扣费、计 `AbortBillingSkipped`、`ai_usage_source=aborted`。
- **#1415 clean-EOF 完成标记**：`shouldMarkAIStreamCompleted` 要求 `copyErr == nil`，本方案不改动；上游断流时完成标记不置位，`upstreamDropped` 是并列标记而非替代。
- **SC27 截断标记**：`aiErrorStreamFilter.finalize()` 语义不变；方案 A 的注入决策独立做终止标记扫描，不读不写 `StreamTruncated`。
- **#1410/#1414 信号链**：unrecognized 分支、BillingZeroCost 检查点、`ai_zero_cost_reason` 全部保留，仅新增枚举值。
- **EPP 集群**：`streamTruncationInjector` 安装在 EPP body 过滤器（`reverseproxy.go:1435`）之前，注入帧会随 EPP 链转发；EPP 侧 `EppClient.Close` 阻塞（问题5）不在本方案范围，但断流时 EPP 的 `donech` 等不到 EndOfStream 的收尾阻塞仍需问题5的修复配合——本方案解决计费与客户端感知，不解决 EPP 连接挂起。
- **观测迁移影响**：`err_code` 分布将发生变化——存量集群中原先记为 `CLIENT_WRITE` 的上游断流行改记 `BK_READ_RESP_BODY`，`ErrClientWrite` 计数与 `AbortBillingSkipped` 计数会回落（这是修正而非丢失）；依赖 `CLIENT_WRITE` 做客户端中断率监控的看板需同步调整口径。

## 四、回归测试

### 单元测试

1. `bfe_server`：拷贝错误归因矩阵——纯写错误、纯读错误、CloseWatcher 触发后的读错误（必须归客户端）、读写同时失败、非 SSE 路径、tracker 在 MaxLatencyWriter/CopyWithoutBuffer 分支同样生效（将分类逻辑抽为小函数直测）。
2. 置位判定函数（`shouldMarkUpstreamDropped`）：200+SSE+断流 ⇒ true；2xx 非 200、非 SSE、`ModeFile`/`ModeBatch`、`aiMeta==nil` ⇒ false；copy 成功 ⇒ false。
3. `streamTruncationInjector`：读错误 + 未见终止标记 ⇒ 透传字节后追加协议错误帧；读错误 + 已见 `data: [DONE]` ⇒ 不注入；`event: message_stop`（anthropic）⇒ 不注入；`StreamEndsAtEOF` 适配器 ⇒ 不注入；干净 EOF ⇒ 零干预逐字节透传；注入后原错误照常上抛。
4. 适配器能力接口：openai/anthropic 错误事件帧格式快照测试（`event:` 行有无、data 载荷 JSON）；gemini 豁免。
5. `mod_ai_token_auth`：计费门控矩阵——断流 + EstimateToken=true ⇒ 按已转发估算扣减、`ai_usage_source=upstream_dropped`、计 `UpstreamDropBilled`；断流 + EstimateToken=false ⇒ 零扣费 + `unrecognized` 信号 + `ai_zero_cost_reason=upstream_dropped`；客户端中断（含 watcher 触发形态）⇒ 仍零扣费 + `aborted`，**不**计 `UpstreamDropBilled`。

### 集成测试（SC29 扩展 + 新场景）

mock 能力：现有 `common/mock_backend.go` 的 `SSERaw` 是 handler 返回式（干净 chunked EOF），**需要新增**"写 N 帧后强制关闭连接、不发送终止 chunk"的 mock 端点（net/http 下 hijack 或直接返回后由服务端断连），这是本测试形态的关键前置。

- **TC-07 上游 kill -9 复刻**（OpenAI 流式，`ignore_eos` 语义下 mock 持续发 content chunk，`stream_options.include_usage=true`）：第 N 帧后 mock 断连 ⇒ 断言 ① RMB 按已转发内容估算扣减（promptEst + 前 N 帧 data 字节/4）；② pb3 `ai_total_tokens>0`、`ai_cost_value>0`、`ai_usage_source=upstream_dropped`；③ `err_code=BK_READ_RESP_BODY`、非 `CLIENT_WRITE`；④ 客户端在流内收到协议错误事件帧（openai 形态 `data: {"error":...}`）；⑤ `AbortBillingSkipped` 不增长。
- **TC-08 Anthropic 断流**：`message_start` + content deltas 后断连（无 `message_stop`）⇒ 客户端收到 `event: error` 帧；计费同 TC-07。
- **TC-09 断流 + EstimateToken=false** ⇒ 零扣费，`ai_usage_source=unrecognized` + `ai_zero_cost_reason=upstream_dropped` + warn。
- **回归**：SC12（客户端中断零扣费，重点回归 CloseWatcher 形态不被误判）、SC27（截断标记）、SC29 TC-01~06、SC03（RMB 扣减）、SC05（日志字段）。

验证命令：`cd bfe && go test ./bfe_server/ ./bfe_modules/... ./bfe_basic/` 与 `go test ./tests/integration/implementation/scenario-SC29-usage-snapshot-billing/ -v`；提交前 `make test`。

## 五、修复后正确行为（目标链路）

`kill -9` 上游、流已转发 2014 帧、无 `[DONE]`、无 usage：

1. 上游连接中断 ⇒ `copyResponse` 读错误 ⇒ 归类**上游断流**（CloseWatcher 未触发、写侧无错误）⇒ `err_code=BK_READ_RESP_BODY`，计 `ErrBkReadRespBody`，**不**计 `ErrClientWrite`；
2. 同时 `aiMeta.MarkUpstreamDropped()`；
3. 透传过滤器在源读错误处向客户端补发协议错误事件帧（SSE error 事件），客户端明确得知响应不完整；
4. 收尾：`isClientAbortErr(BK_READ_RESP_BODY)` 为 false ⇒ 跳过 abort 零扣费分支；`estimateBillable = true && (false || true)` ⇒ 不清零 ⇒ `CalcReqUsedQuota(prompt估算, 已转发completion累加)` ⇒ RMB 扣减发生；
5. pb3 日志：`ai_total_tokens>0`、`ai_cost_value>0`、`ai_usage_source=upstream_dropped`、`err_code=BK_READ_RESP_BODY`；`UpstreamDropBilled` +1。

与问题1（客户端主动中断）的最终形态对比：

| 形态 | err_code | 计费 | ai_usage_source |
| --- | --- | --- | --- |
| 客户端主动中断（#1352 语义，不变） | CLIENT_WRITE | 零扣费，计 AbortBillingSkipped | aborted |
| 上游异常断流（本方案） | BK_READ_RESP_BODY | 按已转发内容估算扣费，计 UpstreamDropBilled | upstream_dropped |
| clean-EOF 无终止事件（#1415 语义，不变） | 空 | 完整交付按估算扣费 | estimated |

## 六、边界与残余风险

- **双端同时断开**（客户端恰在 kill -9 同时点"停止"）：CloseWatcher/写错误优先归客户端中断，零扣费。这是保守选择：内容未送达客户端的部分不向用户收费；已在转发中的内容因无法证明送达而不计。与 #1352 口径一致。
- **终止标记扫描的误判窗口**：上游发了终止标记但分帧极特殊（如终止标记被拆在两次 Read 边界且连接随即损坏）——扫描是对透传字节流做的，命中判定不受 Read 边界影响；真正未命中（上游未发终止事件）正是要注入的场景。残余风险可忽略。
- **注入帧对老客户端的影响**：遵循各协议既有的流内错误事件格式，主流 OpenAI/Anthropic SDK 均会将其 surfaces 为错误而非静默；这比现状（静默截断）严格更可诊断。
- **`NormalizeUpstreamError.StreamTruncatedError` 默认 true 是行为变更**：未开归一的存量集群将从"静默截断"变为"注入错误事件"，属本修复的目的本身；如需灰度可先将默认置 false 分批打开，但建议直接默认 true（缺陷形态对客户端是不可诊断的）。
- **无 DDL、无模块顺序调整、无转发字节变化**（注入仅发生在断流路径）；回滚 = revert 三个步骤的置点。

## 七、实施与部署说明

- 升级 bfe 即可生效，无配置迁移；`StreamTruncatedError` 为 `*bool`，缺省（nil）= 开启，旧 conf 无需改动，显式 `"StreamTruncatedError": false` 才关闭。
- 部署后验收：重跑 issue 所述用例（kill -9 vLLM 流式场景），断言扣费非 0、客户端收到错误事件；核对 `ErrClientWrite`/`AbortBillingSkipped` 回落、`ErrBkReadRespBody`/`UpstreamDropBilled` 出现的看板口径。
- 遗留事项（代码外）：问题5（EPP 长流式挂起）修复前，经 EPP 集群的断流请求仍可能因 `EppClient.Close` 阻塞到不了收尾计费点——两个问题需分别验证；报表/控制台侧可基于 `ai_usage_source=upstream_dropped` 提供"上游断流请求"统计页。

## 八、实施记录（2026-10-30）

已按本方案实施，与方案的偏差均在下方标注。

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/error_code.go` | 新增 `ErrBkReadRespBody = "BK_READ_RESP_BODY"`（上游响应体读取失败，与 `ErrBkReadRespHeader` 同族） |
| `bfe_basic/request_ai_basic.go` | 新增 `upstreamDropped` 标记 + `MarkUpstreamDropped`/`IsUpstreamDropped`；新增 `UsageSourceUpstreamDropped`、`ZeroCostReasonUpstreamDropped` 枚举 |
| `bfe_server/reverseproxy.go` | `copyResponse` 新增 `writeErrorTracker` 记录写侧错误、`responseCopyError` 分类包装（readSide/clientClosed）、CloseWatcher 触发置 `clientClosed`；三条拷贝路径统一经 tracker；AI 路径 `send_response` 按 `isUpstreamDropErr` 分派 `ErrBkReadRespBody`/`ErrClientWrite` 并置 `MarkUpstreamDropped`；新增 `shouldMarkUpstreamDropped`（200 + 非 file/batch + 未完成） |
| `bfe_server/proxy_state.go` | 新增计数 `ErrBkReadRespBody`、`ErrStreamTruncationInjected`（反射初始化，无需注册） |
| `bfe_server/reverseproxy_ai_error.go` | `normalizeUpstreamError` 安装条件放宽为 `Enabled || StreamEnabled || StreamTruncatedError`（默认开）；stream 分支：仅截断开→只挂注入器，归一+截断开→注入器外套 `aiErrorStreamFilter`（偏差：安装点就在 `normalizeUpstreamError` 内而非其后的独立位置——`adapter` 在此已解析，效果等同且少一次查表） |
| `bfe_server/reverseproxy_stream_inject.go`（新增） | `streamTruncationInjector`：零缓冲透传 + 子串终止标记扫描 + 源读错误时注入协议错误事件（unified 形态用 `wrapUnified`，否则原生 envelope）；`StreamEndsAtEOF` 适配器豁免；注入时置 `StreamTruncated` 并计 `ErrStreamTruncationInjected` |
| `bfe_model_protocol/utils/errors.go` | 新增可选能力接口 `StreamErrorEventBuilder`（`NativeStreamErrorEvent`/`WrapStreamErrorPayload`）、`StreamTerminalMarkersReporter` |
| `bfe_model_protocol/openai/stream_error_event.go`（新增） | openai 错误事件：`data: {"error":{...,"type":"server_error","code":"upstream_stream_truncated"}}`；终止标记 `data: [DONE]`/`data:[DONE]` |
| `bfe_model_protocol/anthropic/stream_error_event.go`（新增） | anthropic 错误事件：`event: error` + `{"type":"error","error":{"type":"api_error",...}}`；终止标记 `event: message_stop` + `[DONE]` 两种形态 |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 新增 `UpstreamErrorNormalizeConf.StreamTruncatedError *bool` + `StreamTruncatedErrorEnabled()`（nil=开启） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 计费门控放宽为 `allow && (completed \|\| upstreamDropped)`；unrecognized 分支按 drop 置 `upstream_dropped` 原因；usage source 增加 `upstream_dropped` 分支并计 `UpstreamDropBilled`；checkpoint 中 `unrecognized && upstreamDropped` 时强制 `upstream_dropped` 原因（防止 `calcCostUnits` 的定价缺口原因掩盖真实成因） |
| `tests/integration/common/mock_backend.go` | 新增 `SSEAbortAfter`：写完 N 帧后 `panic(http.ErrAbortHandler)` 模拟 kill -9（无终止 chunk / gzip trailer） |

验证结果（2026-10-30）：

| 范围 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 全量单元测试（`go list ./... \| grep -v tests/integration`） | 全部通过 |
| 新增单测 | `TestShouldMarkUpstreamDropped`（8 形态）、`TestIsUpstreamDropErr`（5 形态）、`TestCopyResponseErrorAttribution`（读/写侧 + tracker）、`TestStreamTruncationInjector`（7 形态：透传/两协议注入/两种终止标记抑制/gemini 豁免/unified 包裹）、`TestStreamErrorEventBuilder`×2（线上格式）、`TestTokenRequestFinishHandler_UpstreamDrop*`×2（估算计费 / 关闭估算的信号） |
| 集成 SC29 | 9/9 通过：新增 TC-07（OpenAI 断流：精确金额 + `ai_usage_source=upstream_dropped` + `err_code=BK_READ_RESP_BODY` + 客户端收到注入错误事件）、TC-08（Anthropic 断流：`event: error` + 精确金额）、TC-09（`EstimateToken=false`：零扣费 + `unrecognized` + `ai_zero_cost_reason=upstream_dropped`）；TC-01~06 回归通过 |
| 回归 SC03 / SC05 / SC12 / SC27 | 全部通过（客户端中断零扣费、归一流截断标记、日志字段、RMB 扣减均未受影响） |

实施中发现：注入的错误事件帧会经过 `mod_body_process` 的事件估算（`EstimateContentToken(data)` = `len(data)/4`），因此断流请求的完成 token 估算会把这一帧计入（约 20 token）。这是"按转发字节估算"口径的自然结果（既有实现同样计入 role/chunk 等非内容事件），TC-07/08 的精确金额断言已按此口径编写；如需剔除，属估算器语义调整，不在本卡范围。
