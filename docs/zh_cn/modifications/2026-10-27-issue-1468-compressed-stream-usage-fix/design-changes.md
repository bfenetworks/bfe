# BFE Issue #1468 修复方案：压缩（gzip/zstd）流式响应 usage 采集失效

- Issue: https://github.com/bfenetworks/bfe/issues/1468
- 缺陷：上游按 `Accept-Encoding` 压缩 SSE 流（`/chat/completions`、`/v1/messages`）时，`mod_body_process` 把压缩字节原样转交 `LineDecoder`，usage 解析必然落空，请求回退到「字节数÷4」估算：`cache_read_tokens` 恒为 0、`prompt_tokens` 由请求体长度估算、`completion_tokens` 由**压缩后**字节估算。RMB 计费因此按「全量输入 × input 单价」扣减，与上游实际 usage 严重不符（现场抓包命中率 96%）。
- 代码库：`bfe/`（行号基于当前 HEAD，2026-10-27 逐条核实）
- 本期范围：步骤 1 + 2 + 3 + 4 + 5（回源协商收窄到 `gzip`）；步骤 6（zstd/br 解码）转 backlog（见「七、实施顺序与本期决策」）
- 副作用与实测：见「八、副作用与实测数据」
- 关联：#1406（非流式 gzip usage 失效，其「步骤 4」记录的扩展点正是本 issue）、#1364（非流式 chunked 解析）、#1398（`stream_options.include_usage` 注入）、#1414（usage 存在性判定）、#1415（clean EOF 完成判定）、#1447（`outreq.Header` 别名必须 Clone 的先例）

## 一、根因

### 数据流前提：BFE 对 AI 回源响应不做透明解压

反向代理 transport 显式关闭压缩（`bfe_server/reverseproxy.go:285` `DisableCompression: true`），`bfe_http` 的透明解压只在 transport 自己添加 `Accept-Encoding: gzip` 时生效（`bfe_http/transport.go:1044-1054`）。因此客户端自带的 `Accept-Encoding: gzip, deflate, br, zstd` 会原样透传给上游（`hopByHopHeaderRemove` 不清除它，`reverseproxy.go:1680`），上游（现场为 openresty 中继）据此返回 `Content-Encoding: zstd|gzip` + `Transfer-Encoding: chunked` 的 SSE，压缩字节连同响应头一起进入模块层，并被 `sendResponse`（`reverseproxy.go:535`）原样转给客户端。

### 第 1 层（触发条件）：`DoResponseProcess` 遇到 `Content-Encoding` 直接旁路

`bfe_modules/mod_body_process/body_process.go:338`：

```go
default:
	if hasContentEncoding(res.Header.Get("Content-Encoding")) {
		// ... a JSON/SSE decoder would fail on the compressed bytes and
		// truncate the forwarded body. Pass the bytes through untouched
		// (LineDecoder preserves them verbatim); for non-stream responses
		// mod_ai_token_auth parses the decoded copy instead (issue #1406).
		bp.CreateEventDecoder(NewLineDecoder)
		break
	}
```

这段是 #1406 为「非流式 gzip」加的止血旁路：保证转发不被严格解码器截断，代价是**流式场景彻底不解析**。#1406 把非流式交给了 `mod_ai_token_auth.tokenReadResponseHandler` 的「解码副本」路径（`mod_ai_token_auth.go:293`，门槛 `isCompleteNonStreamBody` = 定长或 chunked 非 SSE JSON，`mod_ai_token_auth.go:286`），而压缩 SSE 既不在该门槛内，也在 `mod_body_process` 里被旁路，**两条路同时不通**。

### 第 2 层（治本）：usage 解析跑在压缩字节上

`LineDecoder` 在压缩字节里寻找 `0x0A`，吐出的「事件」是压缩数据片段；`QuotaUsageProcessor.Process`（`content_quota_usage.go:38`）对每个事件调用 `RawEvent.GetQuotaUsage`（`body_process.go:452`），后者把压缩字节交给 `utils.ParseUsageFieldsCrossProtocol`（`bfe_model_protocol/utils/usage_parse.go:209`）→ 必然 `UsagePresent=false` → `isguess=true`、`curtoken=EstimateContentToken(压缩片段)`，且 `IsFinalUsage` / `IsTermination` 全为 false。于是 `MarkFinalUsageSeen()` 永不生效，`tctx.UsedQuota` 始终为 0。

### 第 3 层（计费口径双重失真）

请求收尾时 `IsFinalUsageSeen()` 为假，`TokenUsage` 里只有估算值：

- `prompt_tokens` 来自鉴权阶段的 `GetPromptToken`（`mod_ai_token_auth.go:853`）：`ContentLength/4` 或 peek body 长度/4，即**请求体字节数**，与上游真实 `prompt_tokens`、尤其与 `cached_tokens` 无关；
- `completion_tokens` 来自压缩字节长度/4，被压缩比**低估**；
- `cache_read_tokens` / `cache_write_tokens` 恒为 0。

`calcChatCost` 因此把全部 `prompt_tokens` 当「未命中输入」按 input 单价扣减（input 1.0 元/M vs cache_read 0.02 元/M 量级差 50 倍），同时把 `ai_usage_source` 落为 `estimated`（`mod_ai_token_auth.go:481`，常量见 `bfe_basic/request_ai_basic.go:328-347`）。这直接违反 `rmb_quota.md` §1.1/§1.2 第 6 条（cache 子项必须拆分并按 cache_read 价计费）。

### 第 4 层（次生与放大）

- `bfe_util/content_encoding.go:35` 的 `DecodeContentEncoding` 只支持 `gzip`/`x-gzip`（`:44`），`zstd`/`br` 直接返回 `unsupported content-encoding`（`:53`）。现场 zstd 占多数，因此**即便补一条「流式解码副本」链路，zstd 仍解不开**；非流式副本路径（#1406）对 zstd 同样失效。
- `bfe_server/reverseproxy.go:1581` `maybeInjectStreamUsage` 要求 body 可完整缓冲（`:1599` `body, all := bodyAccessor.GetBytes(); if !all || len(body) == 0 { return false }`，`all=false` 由 `bfe_http/transfer.go:950-973` 在 body 超过 `AccessibleBodySize` 时置位）。大上下文请求静默跳过注入 → 上游本就不带 usage 的概率升高，与压缩缺陷叠加。
- 旁路与降级全程无观测信号：解码失败/旁路发生时不打点、不告警，只能靠事后对账发现。

> 结论：#1468 与 #1406 是同一族缺陷的两半——#1406 修的是「非流式 + 整体可缓冲」，本 issue 修的是「流式 + 只能边转边解析」。issue 的证据索引（抓包 vs `bfe_ai_request_log` 同窗口端点级分化、明文 `/responses` 正常）已排除「上游未命中缓存」，源码给出确切跳过分叉，判定为 `FAILED_PRODUCT` 成立。

## 二、修复方案

### 总体思路

`mod_body_process` 的既有结构是「decode → processors → encode → 转发」（`body_process.go:186` `fillBuffer` 写 buffer，`Read` 从此吐出给 `copyResponse`）。压缩流式场景需要**同时满足**三个目标，缺一不可：

1. **转发逐字节不变**：客户端收到的仍是上游那串压缩字节（含 gzip 尾帧），`Content-Encoding` 头保留 → 客户端解压器不受影响；
2. **解析走解压旁路**：usage 从解压后的明文中提取，与明文 SSE 路径语义完全一致；
3. **失败必须 fail-open**：任何解压/解析异常都退化为「原样透传」，绝不截断转发、绝不阻断计费链路。

据此引入一个 **tap 解码器**（`CompressedStreamDecoder`）：用一个 `io.TeeReader` 把源字节旁路给解压器，解码器读的是明文（喂给既有 `SSEEventDecoder` 得到 `SSEEvent`，`GetQuotaUsage` 走同一条协议适配链，`llm_util.go:143`），而吐给 encoder 的事件携带的是**被解压器消费掉的那段原始压缩字节**。每个上游字节恰好转发一次、顺序不变；明文只用于解析。

选择 tap（而非 goroutine + `io.Pipe`）的理由：全流程仍在 `copyResponse` 的读循环内（`reverseproxy.go:997`），不新增 goroutine、不引入管道背压与死锁面，与现有 `BodyProcessor` 的单线程语义一致。

**解码范围只做 `gzip`/`deflate`**：配合步骤 5 把回源协商收窄到同一集合，`zstd`/`br` 不进 tap（理由见步骤 6 与「八、副作用与实测数据」：zstd 无带宽收益但每流约 5 MB 内存，br 窗口不可外部限幅）。

### 步骤 1（必改）：`bfe_util` 补流式解码器（仅 gzip / deflate）

文件：`bfe_util/content_encoding.go`（现有 `DecodeContentEncoding` 保留为兼容入口，内部委托）。

```go
// NewContentDecoder returns a streaming decoder for an HTTP Content-Encoding
// header value. A multi-value list is decoded in reverse application order.
// An unsupported coding returns an error so the caller can fall back to
// passing the encoded bytes through untouched.
func NewContentDecoder(encoding string, r io.Reader) (io.ReadCloser, error)

// CanDecodeContentEncoding reports whether every coding in the header value
// is supported. Used both to select the stream tap and to narrow the outbound
// Accept-Encoding (step 5).
func CanDecodeContentEncoding(encoding string) bool
```

单值分派（`newSingleContentDecoder`）：

| coding | 实现 | 备注 |
| --- | --- | --- |
| `""` / `identity` | `io.NopCloser(src)` | 恒等 |
| `gzip` / `x-gzip` | `gzip.NewReader` | 保持 `Multistream(true)`；错误由上层 fail-open |
| `deflate` | `zlib.NewReader` / `flate.NewReader` | 先 `bufio.Peek(2)` 嗅探 zlib 头（`hdr[0]&0x0f==8 && (b0<<8\|b1)%31==0`），不合规则按裸 DEFLATE 处理（部分实现不符合 RFC 9110） |
| `br` / `zstd` / 其它 | 返回 `unsupported content-encoding %q` | **本期刻意不支持**：靠步骤 5 把它们排除在协商之外；出现即 fail-open → 估算 + 告警计数（见步骤 6 的触发条件） |

`DecodeContentEncoding(encoding, data)` 改为：`NewContentDecoder` + `io.ReadAll(io.LimitReader(dec, bfe_http.GetAccessibleBodySize()+1))`，保留现有「超限/失败原样返回」的 fail-open 语义。这样**非流式路径（#1406）顺带获得 `deflate` 支持**；`zstd`/`br` 的非流式副本路径同样维持「原样返回 + 告警」，不在本期扩展（与步骤 6 一并立项）。

### 步骤 2（必改，治本）：`mod_body_process` 增 tap 解码器

文件：`bfe_modules/mod_body_process/body_process.go`（新增 `compressed_stream_decoder.go` 更清晰）。

数据结构：

```go
// tapEvent carries two views of one stream position: raw is the compressed
// byte range read from the upstream and forwarded verbatim; plain is the
// plaintext event produced by decompressing that range and used only for
// usage parsing. It is never re-encoded into the forwarded stream.
type tapEvent struct {
	raw   []byte
	plain Event // nil for flush / raw-only events
}

func (e *tapEvent) ToBytes() []byte { return e.raw }

func (e *tapEvent) GetQuotaUsage(authStyle string) QuotaUsage {
	if e.plain == nil {
		return QuotaUsage{IsGuess: true} // 不参与估算，也不构成 final usage
	}
	return e.plain.GetQuotaUsage(authStyle)
}
```

解码器：

```go
type CompressedStreamDecoder struct {
	src     io.Reader        // 上游原始（压缩）body
	raw     bytes.Buffer     // tee 落点：已从 src 读走、尚未转发的字节
	dec     io.ReadCloser    // 解压器，读取 io.TeeReader(src, &raw)
	inner   EventDecoder     // NewSSEEventDecoder(dec)
	rawOnly bool             // 解压失败后切换为逐字节透传
	eof     bool
	budget  int64            // 解明文预算（见「边界与残余风险」）
}

func NewCompressedStreamDecoder(source io.Reader, encoding string, fac EventDecoderFac) (EventDecoder, error)
```

`Decode()` 契约（每条都是测试可断言的不变量）：

1. `inner.Decode()` 取明文事件 → `drainRaw()` 取出这一步被解压器消费的压缩字节 → 若明文事件非空，把 raw 挂到第一个事件上返回（raw 可能为空：同一压缩块内的后续事件）；
2. 明文事件为空且 `err == nil`（宽容解码器可能出现）→ 若 raw 非空，先返回 raw-only 事件，避免客户端停摆；否则继续循环；
3. 明文 `err == io.EOF` → **先排空解压器**（`io.Copy(io.Discard, d.dec)`，把其内部缓冲的压缩字节经 tee 冲出），**再直接读 `src` 到 EOF**，把剩余 raw 一并以 raw-only 事件返回（保证 gzip 尾帧完整送达）；下一次 `Decode` 返回空事件 → `bp.err = io.EOF`；
4. 解压/解码出错 → `log.Logger.Warn`（含 `Content-Encoding`、错误）+ 计数器 → `rawOnly = true`，冲刷已积累 raw，此后按 `LineDecoder` 语义直接读 `src` 转发，**不再解析**；
5. 字节守恒：任一时刻 `已转发字节 == src 已被读走的字节`；raw 只在被返回时清空，从不重复挂载。

`BodyProcessor` 需要的配套改动：

- 新增 `func (bp *BodyProcessor) SetEventDecoder(dec EventDecoder)`（现有 `CreateEventDecoder` 在失败时写 `bp.err`，tap 构造失败需要走旁路而不是报错）；
- `Close()` 除 `bp.source.Close()` 外，若 `bp.decoder` 实现 `io.Closer` 则一并关闭（`gzip.Reader`、`zlib.Reader` 都实现，统一在 tap 的 `Close()` 里释放）。

接入点：`DoResponseProcess`（`body_process.go:338`）的压缩分支增加一层判定——**仅当存在 `QuotaUsageProcessor` 且响应是 SSE 且编码可解码时**才建 tap：

```go
	if hasContentEncoding(encoding) {
		// Compressed body: the forwarded bytes must stay exactly what the
		// upstream sent. For a streaming (SSE) response the usage is parsed
		// from a decompressed side channel (issue #1468); every other shape
		// keeps the pass-through of issue #1406.
		if ccq != nil && res.IsSse {
			if dec, err := NewCompressedStreamDecoder(res.Body, encoding, NewSSEEventDecoder); err == nil {
				bp.SetEventDecoder(dec)
				m.state.ResStreamDecoded.Inc(1)
				break
			} else {
				m.state.ResStreamDecodeSkipped.Inc(1)
				log.Logger.Warn("%s: stream usage decode unavailable, content-encoding %q: %v", m.name, encoding, err)
			}
		}
		bp.CreateEventDecoder(NewLineDecoder)
		break
	}
```

要点：

- `res.IsSse` 由 `bfe_http/response.go:214`（`isSSEResponse`，已处理 `; charset=utf-8` 参数）在读完响应头时置位，此处可用；
- `ccq != nil` 之外的场景（只有内容审计规则、无 AI 计费）不建 tap，避免为审查白白解压；
- 非 SSE 的压缩响应（压缩 JSON）仍走 #1406 的整体副本路径，行为不变；
- `res.ContentLength = -1` / `res.Header.Del("Content-Length")` 是既有行为（`body_process.go:374-376`），本次不改；
- 转发切分粒度会从「压缩字节行边界」变为「SSE 事件边界」。今日的切分点本就是压缩数据里偶然出现的 `0x0A`，无语义；**转发字节序列本身逐字节不变**（见验收判据 3 的落实方式）。

### 步骤 3（自动生效，须固化测试）：估算兜底改用解明文字节

`RawEvent.GetQuotaUsage`（`body_process.go:452`）对 guess 事件按 `EstimateContentToken(压缩片段)` 累加，压缩比会低估计费量。tap 路径下 `tapEvent` 转发的是 `plain.GetQuotaUsage(...)`，其 `CurrentTokens` 已是 `EstimateContentToken(明文)`（`llm_util.go:152-155`），因此**无需改动即自动修正**；需在测试中固化该断言（压缩 SSE 无 usage ⇒ 估算量 ≈ 明文长度/4，而非压缩长度/4）。

`GetPromptToken`（`mod_ai_token_auth.go:853`）保持现状：它只是鉴权阶段为「配额预检查」准备的估值，一旦解析到 final usage 就会被 `QuotaUsageProcessor` 覆盖（`content_quota_usage.go:57-101`）。本 issue 不改其口径，但需在文档/日志里保持「parsed 覆盖 estimated」的既有优先级。

### 步骤 4（必改）：可观测性 + `maybeInjectStreamUsage` 不再静默

1. `mod_body_process` 的 `ModuleBodyProcessState`（`mod_body_process.go:40`）新增计数：`ResStreamCompressed`（压缩响应数）、`ResStreamDecoded`（tap 生效数）、`ResStreamDecodeSkipped`（编码不可解码/构造失败）、`ResStreamRawOnly`（解压中途失败降级数）。`ResStreamRawOnly` 持续增长即代表「计费正在估算」，是本次缺陷的直接告警面。
2. `maybeInjectStreamUsage`（`reverseproxy.go:1581`）在 `!all` 处增加 `log.Logger.Warn` + 计数器（`stream_options.include_usage` 注入被跳过，`AccessibleBodySize` 不足），让「上游不带 usage」这一放大因素可被观测；运维侧按最大请求体调整 `AccessibleBodySize`（`conf/bfe.conf:41`，上限 8 MB）。
3. 不做「新增访问日志字段」：`ai_usage_source=estimated` 已能表达口径（验收判据 5 要求估算不得冒充 parsed），新增字段只会增加下游报表改动面。

### 步骤 5（必改，本期执行）：回源 `Accept-Encoding` 收窄到「网关可解码集合」

> **决策（2026-10-27，二次评审修订）**：本步**由 backlog 提回本期、改为必做**。理由：解压侧只保 `gzip`/`deflate` 时，必须保证上游不会发 `zstd`/`br`；把协商收窄到同一集合，才能把上游的编码集合关进可测范围（这也是「为什么不做 zstd 解码」成立的前提，见步骤 6）。本步同时是步骤 2 的运维级开关：出问题可切 `identity` 立即回到明文路径。

作用：让**网关解不开的编码不再进入协商**，同时几乎不损失带宽（实测 gzip 与 zstd 在 SSE 上体积相当，见「八、副作用与实测数据」）。

配置（沿用 `[Server]` 布尔/枚举风格，`bfe_config/bfe_conf/conf_basic.go:55-59` 已有先例）：

```ini
# how Accept-Encoding is forwarded to an AI upstream:
#   auto     - keep only the codings the gateway can decode (default)
#   identity - strip compression (diagnostics / troubleshooting)
#   off      - forward the client value unchanged
AIUpstreamAcceptEncoding = auto
```

实现落点：`doSingleAIForward` 中 `hopByHopHeaderRemove`（`reverseproxy.go:1680`）之后调用 `applyAIUpstreamAcceptEncoding(outreq, mode)`。

关键约束（#1447 已踩过的坑，务必照抄）：`outreq` 是 `*req` 的浅拷贝（`reverseproxy.go:1667`），`Header` **与入站请求共用同一个 map**，改动前必须 `outreq.Header = outreq.Header.Clone()`，否则会污染入站请求（鉴权、访问日志、`mod_compress`、客户端可见头）。**只改 outreq，不改入站请求**。

`auto` 语义（三条硬规则，缺一即 bug）：

1. 解析客户端列表为 `(coding, q)`，保留 q 值；
2. 剔除 `!CanDecodeContentEncoding(coding)` 的项（`*` 视为无约束，展开为网关可解码集合 `gzip, deflate`）；
3. 若剔除后**为空**（例如客户端只声明 `zstd`），**不修改**该头——不能因为网关解不开就回一个客户端没接受的编码（同理，客户端带 `identity;q=0` 时不得强制 identity）；
4. 与客户端原值相同时不写回（避免无意义差异）。

**对外可见变化（需签核）**：原来拿到 `zstd` 的客户端此后会拿到 `gzip`。这合法（客户端声明了 gzip）、体积几乎不变（实测 145.8 KB vs 147.0 KB）、客户端解压器无感，但确实是**对验收判据 3 的显式偏离**（判据原文要求「客户端可见字节流与修复前一致」），发布前需运维/产品确认。

**运维预案（不改代码也能生效，用于 hotfix 或灰度前的临时处置）**：`mod_header` 已支持 `REQ_HEADER_SET`（`bfe_modules/mod_header/action.go:52`），请求侧回调注册在 `HandleAfterLocation`（`bfe_modules/mod_header/mod_header.go:187`），早于 `doSingleAIForward` 里的 `hopByHopHeaderRemove`：

```json
{
    "cmd": "REQ_HEADER_SET",
    "params": ["Accept-Encoding", "gzip, deflate"]
}
```

预案的代价：它改的是**入站**头（会影响访问日志、`mod_compress`、`mod_ai_cache` 的可见行为），且不支持与客户端列表求交，因此只作应急处置，正式路径走上面的代码方案。

### 步骤 6（backlog，本期不实施）：`zstd` / `br` 解码

**本期刻意不支持** `zstd`/`br`：由步骤 5 把它们排除在协商之外。转入 backlog，触发条件（满足其一即立项）：

1. 现场出现**不遵守 `Accept-Encoding`、强制 `zstd`/`br`** 的上游，且该流量占比显著（判据：`ResStreamDecodeSkipped` 占比 > 1%，或对账显示仍有可观比例落 `estimated`）；
2. 业务要求保留 zstd/br 的带宽收益（需先按「八、副作用与实测数据」复核，实测收益 ≈ 0）；
3. 客户端只声明 `zstd`（`auto` 因求交为空而不改写）的场景普遍存在。

落地时的前置工作与约束（留档）：

- 引入 `github.com/klauspost/compress`，**版本钉 `v1.18.0`**（声明 `go 1.22`；`v1.18.5`/`v1.19.x` 声明 `go 1.24`，会迫使 `go.mod` 提升语言版本）；License BSD-3-Clause，须实跑 `make license-check`；二进制约 +1.0 MB；
- **必须先把现场 pcap 里 zstd 帧头声明的 window 量出来，再定 `WithDecoderMaxWindow`**：解码器按帧声明的窗口分配内存（实测窗口 4 MB ⇒ 每流约 5.2 MB 常驻），上限设小了会让帧直接解码失败（fail-open），设大了会撑爆内存；
- `WithDecoderConcurrency(1)`（`WithDecoderLowmem(true)` 已是默认值）+ `BodyProcessor.Close()` 释放；
- `br` 的 ring buffer 上限来自流内部（`1<<window_bits`，标准最大 16 MB、large-window 更大），库无法外部限幅，属内存 DoS 面——即使做 zstd 也建议**不把 br 放进 tap**。

### 被否决的方案

| 方案 | 否决理由 |
| --- | --- |
| `io.Pipe` + 解析协程（push 式喂解压器） | 每请求新增 goroutine 与管道背压，慢速/中断路径的泄漏与死锁面变大；需要在数据面新增生命周期管理 |
| 先整体缓冲解压再解析 | SSE 无法缓冲（破坏流式、内存不可控），且大上下文请求正是本缺陷的现场形态 |
| 仅回源 `Accept-Encoding: identity`（方案 B） | 改动最小但让所有 AI 流失去压缩（实测这类流量压缩比约 4.2x，egress 成本显著上升）；`identity` 仅作为步骤 5 的排障模式保留 |
| 只补 `DecodeContentEncoding` 支持 zstd | 流式不走该函数（`body_process.go:338` 旁路），单补非流式无法解决 issue 场景 |
| 只改 `Accept-Encoding` 不做 gzip 解码 | 例外只有 `identity`；若收窄到 `gzip` 而不实现解码，压缩 SSE 仍会落回估算，等于没改。故步骤 5 必须与步骤 1/2 成对交付 |
| 支持全部编码（zstd/br）而不收窄协商 | 实测 zstd 无带宽收益却带来约 5 MB/流的解码内存，br 窗口不可外部限幅；把不可控维度留在数据面不如在协商层关掉 |

## 三、回归测试

### 单元测试

1. `bfe_util/content_encoding_test.go`（在现有 5 例上扩展）：
   - `TestNewContentDecoderRoundTrip`：`gzip` / `x-gzip` / `deflate`（zlib 头）/ `deflate`（裸 flate）往返；
   - `TestCanDecodeContentEncoding`：`gzip`、`gzip, deflate` 为真；`br`、`zstd`、`compress` 为假（本期刻意不支持，配合步骤 5）；
   - `TestNewContentDecoderUnsupported`：未知编码返回 error 且不消费源；
   - `TestDecodeContentEncodingZstdFailsOpen`：`zstd` 输入原样返回 + error 非空（非流式路径的既有 fail-open 语义，防止将来误以为已支持）。
2. `bfe_modules/mod_body_process/compressed_stream_decoder_test.go`（新增）：
   - `TestCompressedStreamDecoderGzipSSEUsage`：gzip(OpenAI SSE，末帧含 `usage.prompt_tokens_details.cached_tokens`) + `QuotaUsageProcessor` → 断言 `PromptTokens` / `CompletionTokens` / `CacheReadTokens` 等于上游值、`IsFinalUsageSeen()` 为真；**同时**断言所有事件 `ToBytes()` 顺序拼接后与输入压缩字节逐字节相等；
   - `TestCompressedStreamDecoderAnthropicCacheRead`：`message_start`/`message_delta` + `cache_read_input_tokens`（对应 issue 的 `/v1/messages`）→ cache 字段正确、`IsTermination` 由 `message_stop` 置位；
   - `TestCompressedStreamDecoderSplitReads`：源每次 `Read` 只返回 1 字节（模拟慢速分块到达）→ 解析与转发断言同上（覆盖跨帧增量解压边界）；
   - `TestCompressedStreamDecoderDeflateSSEUsage`：`deflate`（zlib 头）与裸 `flate` 两种形态；
   - `TestCompressedStreamDecoderCorruptFailsOpen`：截断 gzip（去 trailer）/ 非压缩字节 + `Content-Encoding: gzip` → 无错误返回、转发字节完整、不产生 final usage；
   - `TestCompressedStreamDecoderEstimateUsesPlaintext`：无可解析 usage 的压缩 SSE + `SetAllowEstimateToken(true)` → `CompletionTokens ≈ 明文长度/4`（而非压缩长度/4）；
   - `TestCompressedStreamDecoderCloseReleasesDecoder`：`BodyProcessor.Close()` 会关闭 tap。
3. `bfe_modules/mod_body_process/body_process_test.go`（扩展）：
   - `TestDoResponseProcessCompressedSSE`：`IsSse=true` + `Content-Encoding: gzip` → 选 tap；`IsSse=false`（压缩 JSON）→ 仍 `LineDecoder`（#1406 回归）；`ccq == nil`（仅审计规则）→ 仍 `LineDecoder`；`Content-Encoding: zstd` → 仍 `LineDecoder` + `ResStreamDecodeSkipped`。
4. `bfe_server/reverseproxy_accept_encoding_test.go`（**本期必写**，对应步骤 5）：
   - `auto`：客户端 `gzip, deflate, br, zstd` → 回源被收窄为 `gzip, deflate`；客户端仅 `zstd` → **头原样不变**；客户端 `gzip;q=0.5, zstd` → 保留 q 值；
   - `identity` → 回源头为 `identity`；`off` → 原样；非 AI 请求 → 一律原样（不受开关影响）；
   - **入站 `req.Header` 未被修改**（别名隔离，对齐 #1447 的断言风格）。

### 集成测试

`tests/integration/common/mock_backend.go` 需新增两个能力：

- `SSECompress string`（本期只需 `"gzip"`）：在 `SSEEvents`/`SSERaw` 模式下把每帧经**流式压缩器 + Flush** 写出，并写出对应的 `Content-Encoding` 头、使用 chunked 传输。现有的 `ResponseHeaders + Body`（TC22 用法，`sc03_rmb_quota_test.go:1300-1345`）只能模拟「一次性压缩的整块」，无法覆盖跨帧增量解压，因此该能力是本方案集成验证的前提。
- 让 mock 记录并**断言收到的 `Accept-Encoding`**（`headerCopies` 已有），用于步骤 5 的「回源协商确实被收窄」断言。

> 注意：`zstd` 帧在测试里**无法现场生成**（标准库无 zstd 编码器，本期也不引入 `klauspost/compress`）。需要模拟「上游不遵守协商、强发 zstd」时，用 `Content-Encoding: compress`（或其它未支持编码）配任意字节即可达到同一 fail-open 分支；若要精确复现 zstd，只能在 `testdata/` 里放一个预置的 zstd 帧二进制 fixture。

1. `scenario-SC03-rmb-quota`：
   - `TestTC29_RMBQuotaDeduction_OpenAI_Stream_CompressedGzip`（设计文档：TC-29）：请求显式带 `Accept-Encoding: gzip`；上游返回 gzip SSE（末帧含 `usage.prompt_tokens_details.cached_tokens`）。断言：① 客户端 `Content-Encoding: gzip`，`gunzip(body)` 与上游明文逐字节一致（转发不变）；② 命中 1 次；③ `quota:plan_rmb` 扣减 == `(prompt−cacheRead−cacheWrite)×input + cacheRead×cache_read_input_token_cost + cacheWrite×cache_creation_input_token_cost + completion×output`（定点整数，口径同 SC1301-TC022，不使用容差）。
   - `TestTC30_RMBQuotaDeduction_AcceptEncodingNarrowed`（**必做**，设计文档：TC-30）：客户端发 `Accept-Encoding: gzip, deflate, br, zstd`，断言上游实际收到的是被收窄的值（`headerCopies`），且响应正常计费（与 TC-29 同判据）。
   - `TestTC31_RMBQuotaDeduction_Stream_UnsupportedEncodingFailsOpen`（设计文档：TC-31）：上游无视协商返回 `Content-Encoding: compress`，断言转发字节不变、`ai_usage_source=estimated`、`ResStreamDecodeSkipped` 计数 +1（验收判据 5）。
2. `scenario-SC05-access-log-ai-fields`：新增压缩 SSE 用例，断言 `ai_cache_read_tokens == cached_tokens`、`ai_usage_source == "parsed"`（判据 1）。
3. `scenario-SC35-body-process-content-audit` / `scenario-SC36-sse-long-connection`：真回归——明文 SSE 的字节序列、事件时序、客户端中断不计费（#1352）行为不变；`scenario-SC29-usage-snapshot-billing`、`scenario-SC30-nested-choice-usage-billing` 保持通过。

### 本地验证命令

```bash
cd bfe
go build ./...
go vet ./bfe_util/ ./bfe_modules/mod_body_process/ ./bfe_modules/mod_ai_token_auth/ ./bfe_server/
go test ./bfe_util/ ./bfe_modules/mod_body_process/ ./bfe_modules/mod_ai_token_auth/ ./bfe_server/ ./bfe_config/bfe_conf/
go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ -v
go test ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/ -v
go test ./tests/integration/implementation/scenario-SC36-sse-long-connection/ -v
```

## 四、修复后正确路径（压缩 SSE + gzip）

1. transport 原样收到 gzip body（`DisableCompression: true`），`res.IsSse=true`、`ContentLength=-1`；
2. `mod_body_process.DoResponseProcess`：`ccq != nil && res.IsSse && CanDecodeContentEncoding("gzip")` → 建 tap；`res.Body` 换成 `BodyProcessor`；
3. `copyResponse` 读循环：tap 每次 `Decode()` 从解压流拿到一个 SSE 事件 → `QuotaUsageProcessor` 解析出该事件的 usage（含 `cached_tokens`）→ 末帧命中 `IsFinalUsage` → `MarkFinalUsageSeen()`；同一事件携带的压缩字节原样写入 buffer → 客户端拿到与上游一致的 gzip 流；
4. `tokenRequestFinishHandler`：`IsFinalUsageSeen()` 为真 → 守卫不清零 → `calcChatCost`：`normalInput = prompt − cacheRead − cacheWrite` 按 input 价、`cacheRead × cache_read_input_token_cost`、`cacheWrite × cache_creation_input_token_cost`、`completion × output` 逐项计费，与供应商台账一致；
5. 访问日志 `ai_input_tokens` / `ai_output_tokens` / `ai_cache_read_tokens` 反映上游真实 usage，`ai_usage_source = parsed`；
6. 客户端可见字节流、`Content-Encoding` 头与修复前**逐字节一致**（解压只服务解析）。唯一例外是步骤 5 的编码协商：客户端原本会拿到 `zstd`，收窄后拿到 `gzip`（仍在其 `Accept-Encoding` 之内，体积差 <1%，见「八」）。

## 五、边界与残余风险

- **上游不遵守协商**：若上游无视被收窄的 `Accept-Encoding`，强行返回 `zstd`/`br` 或其它未支持编码，则不建 tap 或中途降级 → 转发不受影响、计费落 `estimated`，`ResStreamDecodeSkipped` / `ResStreamRawOnly` 计数 + 告警可见。这是本方案**唯一**会静默退回估算的路径，必须配告警（占比 > 1% 即触发步骤 6 的立项条件）。
- **只声明 `zstd` 的客户端**：`auto` 因求交为空而不改写其 `Accept-Encoding`，若上游因此返回 `zstd`，仍走上面的估算降级。此类客户端极罕见（主流客户端都同时声明 gzip）。
- **解明文预算**：压缩流的膨胀比不可控（zip bomb）。tap 需设「单事件缓冲上限 + 每响应解明文预算」（建议单事件 8 MiB、每响应 64 MiB，常量置于 `bfe_util` 并附注释）；超限即停止解析（fail-open → 估算）。该保护不影响转发。
- **内容审计（`textfilter`）**：压缩响应上 tap 事件 `ToBytes()` 仍是压缩字节，内容审计看到的仍是压缩数据——与今日行为一致，本次不改变。若需按明文审查，应另立 issue（涉及「审查可以改写事件」与「转发字节不可变」的冲突）。
- **转发切分粒度**：字节序列不变，但写入客户端的边界从压缩行边界改为 SSE 事件边界（见步骤 2 说明）。客户端按字节流解析 SSE，无语义影响；极端情形（上游发出半个事件后停住）最后一帧会延后到事件补齐或 EOF，建议在 SC36 增加帧间隔时延断言。
- **非流式压缩 JSON**：受益于步骤 1 获得 `deflate` 支持（`zstd`/`br` 仍原样返回），且受 `AccessibleBodySize`（`conf/bfe.conf:41`，默认 2 MiB）限制——超限时只有前缀可解析。此为 #1406 的既有边界，非本次引入。
- **额外 CPU / 内存**：仅对「AI 请求 + SSE + 压缩 + 编码可解码」的响应解压一次，转发路径不重复解压；gzip 实测约 40 KB/流常驻、~15 ms CPU 每 MB 明文（见「八」）。相比 `identity` 方案是用 CPU 换带宽，相比 zstd 解码方案是用 CPU 换内存与依赖。
- **不做的事**：不新增访问日志字段；不改模块注册顺序（`bfe_modules/bfe_modules.go:155` / `:189` 保持 `mod_ai_token_auth` 早于 `mod_body_process`）；不改 `rmb_quota.md` 的计价条款，仅同步 §7.4 的实现描述。

## 六、验收判据对照（对应 issue 的 5 条）

| issue 验收判据 | 落实点 | 验证 |
| --- | --- | --- |
| 1. gzip/zstd 压缩 SSE 的 `ai_cache_read_tokens` 等于上游 `cached_tokens`/`prompt_cache_hit_tokens`，`ai_usage_source=parsed` | 步骤 2（tap → 既有协议适配链）+ 步骤 5（把上游编码收窄到 gzip） | SC03-TC-29/TC-30、SC05 压缩 SSE 用例；单测 `...GzipSSEUsage` / `...DeflateSSEUsage` / `...AnthropicCacheRead` |
| 2. Redis RMB 扣减 == 逐项计价（定点整数，无容差） | 步骤 2 使 `MarkFinalUsageSeen` 生效，`calcChatCost` 原始逻辑即逐项计价 | SC03-TC-29 定点断言（口径同 SC1301-TC022） |
| 3. 客户端可见 SSE 字节流、`Content-Encoding`、时序与修复前一致 | tap 设计第 1 条不变量（`tapEvent.ToBytes()==raw`；EOF 排空解压器后直读源）。**显式偏离项**：步骤 5 会把握手结果收窄为 gzip，客户端可见编码可能由 zstd 变 gzip（仍在其 `Accept-Encoding` 内，体积差 <1%，须签核） | 单测字节守恒断言；SC03-TC-29 `gunzip(body)` 逐字节比对；TC-30 断言回源头被收窄 |
| 4. 未压缩路径（明文 `/responses`、非流式 JSON）记录不得回归 | 仅改 `hasContentEncoding` 分支，明文路径整段未动 | SC29/SC30 全量、SC03 TC-01~TC-28 回归 |
| 5. 上游确无 usage 时估算兜底仍生效且标 `estimated` | 步骤 2 的 raw-only/失败降级 + 步骤 3（估算改用明文长度） | 单测 `...EstimateUsesPlaintext`、`...CorruptFailsOpen`；SC03-TC-31 的未支持编码 fail-open 用例 |

## 七、实施顺序与本期决策

### 决策记录（2026-10-27，含二次评审修订）

**本期范围：步骤 1 + 2 + 3 + 4 + 5（回源协商收窄到 `gzip, deflate`）；步骤 6（`zstd`/`br` 解码）转 backlog。**

一版决策（先做 zstd 解码、协商收窄进 backlog）经实测与评审后**推翻**，修订理由：

1. **zstd 在这类流量上没有带宽收益**：同一 619 KB 的 SSE 流，gzip 145.8 KB vs zstd 147.0 KB（详见「八」）。也就是说「改协商把 zstd 换成 gzip」几乎不损失带宽，却能省掉整条 zstd 解码路径；
2. **zstd 解码的内存代价不可控**：解码器按帧头声明的窗口分配内存，实测 4 MB 窗口 ⇒ 每流约 **5.2 MB** 常驻，而 gzip 只有 40 KB。窗口大小由上游编码器决定，网关无法收窄；`WithDecoderMaxWindow` 设小了会让帧直接失败（fail-open），设大了会撑爆内存；
3. **不再需要新增三方依赖**：省掉供应链/许可审查、+1.0 MB 包体积，以及一次跨版本兼容约束（`v1.18.0` 才能配 `go 1.22`）；
4. **改协商的风险可控**：`auto` 模式与客户端列表求交后只保留 `gzip, deflate`，不回客户端没接受的编码；且「上游不遵守协商」已由 `ResStreamDecodeSkipped` 显式可观测（一版的顾虑是「失败静默」，用告警即可消解）；
5. 代价是把一条**对外可见的行为变更**（客户端可见编码 zstd→gzip）引入本期，需要运维/产品签核（见步骤 5）；`br` 因 ring buffer 无法外部限幅，即使将来做 zstd 也不进 tap。

**改口条件**：

- 若现场出现不遵守 `Accept-Encoding`、强行发 `zstd`/`br` 的上游且占比显著（`ResStreamDecodeSkipped` > 1%，或对账仍有可观比例落 `estimated`），立即启动步骤 6；
- 若某集群不允许改回源头（例如上游是强约定压缩的第三方代理），该集群走步骤 5 的 `identity` 模式或 `off` + 步骤 6。

### 执行顺序

1. **步骤 1 + 步骤 5**（`bfe_util` 的 gzip/deflate 流式解码器 + `doSingleAIForward` 的 `auto` 收窄与 `[Server] AIUpstreamAcceptEncoding` 配置，含 `conf/bfe.conf` 与 `docs/zh_cn/configuration/bfe.conf.md` 同步）；
2. **步骤 2 + 步骤 3**（tap 解码器、接入 `DoResponseProcess`、估算口径随之修正）——与第 1 步同一补丁，含单测；
3. **步骤 4**（可观测性：`ResStream*` 计数 + 注入跳过告警）——`ResStreamDecodeSkipped` 是步骤 6 立项条件的唯一数据来源，必须同期上线；
4. **步骤 6 → backlog**：本期只写触发条件与落地约束，不动代码；
5. 发布验证：`go test`（受影响包）、SC03 TC-01~TC-31、SC05、SC29/SC30、SC35/SC36 集成；canary 观察 `ResStreamDecoded` / `ResStreamDecodeSkipped` / `ResStreamRawOnly` 与进程 RSS、二进制体积（本期不应有体积变化）。

### 对外语义影响

- 步骤 1/2/3/4 **不新增配置项以外的任何对外变化**：转发字节、`Content-Encoding` 头、协商结果都不动（未开配置时），唯一可见变化是「usage 由估算变为上游真实值」——这正是本 issue 要的结果。
- 步骤 5 **新增一个 `[Server]` 配置项**，且默认 `auto` 会改变回源的 `Accept-Encoding`，进而改变客户端可见的响应编码（zstd→gzip）。合入前需：① 运维/产品签核；② 按配置变更流程同步 `conf/bfe.conf` 与 `docs/zh_cn/configuration/bfe.conf.md`；③ 发布说明里明确「AI 流式响应编码由 zstd 变为 gzip，体积差 <1%」。

## 八、副作用与实测数据

> 测量环境：Go 1.25 toolchain 编译，库版本与本方案一致（`klauspost/compress v1.18.0`、`andybalholm/brotli v1.0.2`、`tidwall/gjson v1.18.0`），单流串行、单核。样本：600 个 OpenAI `chat.completion.chunk` 事件 + usage 末帧，619 KB 明文，逐事件 flush 压缩（贴近现场上游行为），压缩比约 4.2x。数字用于量级判断，落地方案应以仓库内 benchmark 复测。

### 编解码开销与内存

| 路径 | 每流 CPU（619 KB 明文） | 单核吞吐 | 每流常驻内存 |
| --- | --- | --- | --- |
| 今日路径（`LineDecoder` + 长度/4） | ~0.2–0.4 ms | — | 0 |
| **gzip 解码（本期）** | 9.1 ms | 65 MB/s | ~40 KB |
| deflate 解码（本期） | 与 gzip 同量级 | — | ~40 KB |
| zstd 解码（backlog） | 3.0 ms | 195 MB/s | **~5.2 MB**（窗口 4 MB 的帧） |
| brotli 解码（不做） | 10.7 ms | 55 MB/s | 0.2 MB 起，上限受流内窗口（标准 16 MB）影响 |
| usage 解析（gjson 扫事件，下界） | 2.8 ms | 196 MB/s | — |

折算：每 MB 明文约 gzip 15 ms / zstd 5 ms / br 18 ms 单核；usage 解析约 5 ms/MB（明文流今天已在付这笔成本）。SSE 明文吞吐 ≈ tokens/s × 4–6 B——50 MB/s 明文时 gzip 约 0.75 核，500 MB/s 时约 7.5 核。

### 带宽对比（同一 619 KB 流）

| 编码 | 压缩后体积 |
| --- | --- |
| gzip | 145.8 KB |
| zstd | 147.0 KB |
| brotli | 138.9 KB |

**结论**：zstd 相对 gzip 无实际收益 ⇒ 收窄协商到 gzip 的带宽代价 <1%，这是本方案成立的关键前提。

### 依赖与包体积

- 本期零新增依赖；`klauspost/compress` 若在步骤 6 引入，`-s -w` 实测二进制约 **+1.0 MB**（空程序 1.03 MB vs 只 import zstd 2.05 MB），License BSD-3-Clause。

### 非性能类副作用（发布前需沟通）

1. **账单口径双向跳变**：命中率高的用户成本降到 1/27 量级（issue 已量化）；反之，此前被「请求体/4 + 压缩后/4」低估的用户会被补扣上调，可能出现配额提前耗尽、429 上升。发布日需在账单/报表打标并备好客服口径。
2. **`ai_usage_source` 分布突变**（estimated → parsed）：依赖该比例的看板/告警需重新基线化。
3. **正确性依赖解码成功率**：`ResStreamDecodeSkipped` / `ResStreamRawOnly` 必须接告警，否则会以「部分流量仍在估算」的方式静默失效。
4. **日志刷屏**：解码失败的 warn 需限流（如每秒 N 条 + 计数），避免异常流打爆日志。
5. **内容审计仍看不到压缩体**：行为不变，但需在发布说明里说清「这次修的是计费，不是内容审查」。

### 不受影响的路径

非 AI 流量、明文 SSE（今天就在付解析成本）、非流式响应、以及无 `QuotaUsageProcessor` 的产品（`ccq == nil` 时不建 tap）——行为与成本都不变。
