# BFE Issue #1406 修复方案：非流式 gzip 响应 usage 采集失效

- Issue: https://github.com/bfenetworks/bfe/issues/1406
- 缺陷：上游返回 `Content-Encoding: gzip` + `Transfer-Encoding: chunked` 的**非流式** AI 响应时，usage 双路采集全部失效——pb3 访问日志 `ai_output_tokens=0`、`ai_total_tokens=0`、`ai_cache_read_tokens=0`（`ai_input_tokens` 仍是鉴权时的估算值），RMB 计费同样按 0 入账。流式（SSE）响应不受影响。
- 代码库：`bfe/`（行号基于当前 HEAD，2026-10-11 逐条核实）
- 关联：同族已闭环 #1364（非流式 chunked，**未压缩** body）、#1398 / #1401（流式 `total=0`）。本缺陷是「**gzip 压缩的**非流式 body」这一新盲区——#1364 新增的 `TestTC16_..._Chunked` 用的是裸 `Content-Type: application/json` + 未压缩 body，恰好绕过了这两个新条件。

## 一、根因（三层缺陷叠加，缺一不可）

### 数据流前提：BFE 对 AI 回源响应不做透明解压

反向代理的 transport 显式关闭了压缩，`bfe_server/reverseproxy.go:284`：

```go
transport := &bfe_http.Transport{
	...
	DisableCompression:    true,
	...
}
```

`bfe_http` 只在「transport 自己添加了 `Accept-Encoding: gzip`」时才透明解压（`bfe_http/transport.go:888-899` 的 `rc.addedGzip`；`addedGzip` 又依赖 `!DisableCompression`，见 `:1043-1055`）。因此当**客户端**自带 `Accept-Encoding: gzip` 时，上游 gzip body 连同 `Content-Encoding: gzip` 原样进入模块层。转发也原样保留：`sendResponse` 用 `bfe_http.CopyHeader(rw.Header(), res.Header)`（`reverseproxy.go:539`）拷头、`copyResponse(res.Body)`（`:544`）拷体。

> 关键约束：**解压只能作用于「仅用于解析的副本」，绝不能改动 `res.Body` / `res.Header`**，否则客户端会收到「声明 gzip、实为明文」的损坏响应。

### 第 1 层（触发条件）：`tokenReadResponseHandler` 的 `ContentLength >= 0` 门槛排除 chunked

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:210-233`：

```go
func (m *ModuleAITokenAuth) tokenReadResponseHandler(req *bfe_basic.Request, res *bfe_http.Response) int {
	ctx := GetTokenAuthContext(req)
	if ctx == nil {
		return bfe_module.BfeHandlerGoOn
	}
	tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
	if res.StatusCode == bfe_http.StatusOK && res.ContentLength >= 0 {
		ctx.aiBasicInfo.MarkResponseCompleted()
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
			body, _ := bodyAccessor.GetBytes()
			UpdateCtxByUsage(ctx, body)
		}
		if tokenUsage.UsedQuota > 0 {
			ctx.aiBasicInfo.MarkFinalUsageSeen()
		}
		if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
			tokenUsage.CompletionTokens = int64(res.ContentLength) / 4
			tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
		}
	}
	return bfe_module.BfeHandlerGoOn
}
```

chunked 响应的 `ContentLength == -1`（`bfe_http/transfer.go` 对 `chunked` 传输编码统一置 -1），第 216 行的合取为假 → 整段跳过：既不 `MarkResponseCompleted()`，也不 `UpdateCtxByUsage()`，`MarkFinalUsageSeen()` 与估算分支全部失效。凡 chunked 非流式响应，可靠的整体解析路径被整体绕过。

### 第 2 层（治本）：body 以压缩字节进入解码器，`mod_body_process` 未处理 `Content-Encoding`

`mod_body_process` 在 `mod_ai_token_auth` 之后运行（`bfe_modules/bfe_modules.go:155` 早于 `:189`；`HandleReadResponse` 过滤链按注册顺序执行）。它用 `res.Header.Get("Content-Type")` 选解码器，并把原始 body 直接喂给解码器，全模块无任何 `gzip` / `Content-Encoding` 判断（`bfe_modules/mod_body_process/*.go` 无匹配）。

对 gzip body，解码出的「事件」是压缩字节，`RawEvent.GetQuotaUsage`（`mod_body_process/body_process.go:429-475`）调用 `utils.ParseUsageFieldsCrossProtocol`（`bfe_model_protocol/utils/usage_parse.go:195-226`）必然全零 → `isguess = true` → `IsFinalUsage = false`、`IsTermination = false`：

- `QuotaUsageProcessor.Process`（`content_quota_usage.go:45-50`）对 `IsFinalUsage` / `IsTermination` 均不成立，`MarkFinalUsageSeen()` 与 `MarkResponseCompleted()` 都不打标；
- `tctx.UsedQuota` 始终保持 0。

### 第 3 层（放大）：`NewContentTypeDecoder` 精确匹配，`; charset=utf-8` 后缀失配

`bfe_modules/mod_body_process/body_process.go:525-537`：

```go
func NewContentTypeDecoder(source io.Reader, contentType string) (EventDecoder, error) {
	var dec EventDecoder
	switch contentType {
	case "application/sse", "text/event-stream", "application/x-sse":
		dec, _ = NewSSEEventDecoder(source)
	case "application/json", "application/ndjson", "application/x-ndjson":
		dec, _ = NewJsonDecoder(source)
	default:
		dec, _ = NewLineDecoder(source)
	}
	return &ContentTypeDecoder{contentType: contentType, dec: dec}, nil
}
```

`switch` 是对整个 header 值的**精确字符串比较**，不解析媒体类型参数。上游常见的 `application/json; charset=utf-8` 落到 `default` → `LineDecoder`（`body_process.go:477-496`）。注意这一层同样误伤流式：`text/event-stream; charset=utf-8` 也会掉进 `LineDecoder` —— 这是本 issue 之外的一个潜伏缺陷。

> 说明：#1364 已让 `RawEvent.GetQuotaUsage` 对「非 SSE 单 JSON」打终态标记，所以**未压缩**的 chunked 非流式响应仍能计费（TC16 覆盖）；本缺陷之所以爆发，是 gzip（第 2 层）与 charset 失配（第 3 层）同时成立，使非流式路径在「第 1 层已放空」的前提下进一步失去唯一兜底。

### 合流：请求收尾守卫清零

`mod_ai_token_auth.go:311-325`：

```go
billingUsage := *tokenUsage
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
	billingUsage.PromptTokens = 0
	billingUsage.CompletionTokens = 0
	billingUsage.CacheReadTokens = 0
	billingUsage.CacheWriteTokens = 0
	billingUsage.CacheWriteTokens1h = 0
	billingUsage.AudioInputTokens = 0
	billingUsage.AudioOutputTokens = 0
	billingUsage.ImageInputTokens = 0
	billingUsage.VideoCount = 0
	billingUsage.ImageCount = 0
	billingUsage.UsedQuota = 0
}
```

`IsFinalUsageSeen()` 与 `IsResponseCompleted()` 均为 false → `estimateBillable` 为 false → 11 个计费字段在计费副本上全部清零：`calcCostUnits`/`calcChatCost`（`:621-846`）按 0 出账；共享的 `TokenUsage.UsedQuota` 本就从未被写入，访问日志读到的也是 0。

**三层关系**：第 1 层让可靠路径失效，第 2 层使唯一兜底无法解析，第 3 层既放大第 2 层、又单独误伤带参数的 SSE。三者对「gzip + chunked + `application/json; charset=utf-8`」这一签名均成立，缺一即不会出现日志/计费全 0。

## 二、修复步骤

设计原则：
1. **不改转发**。解压只作用于解析副本，`res.Body` / `res.Header` 保持上游原样，客户端仍收到 gzip 字节 + `Content-Encoding: gzip`。
2. **非流式整体解析为治本点**。非流式响应本身可整体读入（现有 `tokenReadResponseHandler` 已对定长响应这样做），把 chunked 非流式纳入同一路径，即可绕开 `mod_body_process` 的流式解码链，无须改动其转发语义。
3. **解码器选择修正是独立正确性修复**，同时消除 `; charset=utf-8` 对流式的潜伏误伤。

### 步骤 1（必改，新增共享工具）：`Content-Encoding` 解析副本解压

新增 `bfe_util/content_encoding.go`：

```go
// DecodeContentEncoding returns data decoded from the given HTTP
// Content-Encoding. Unknown or identity encodings, and empty input, are
// returned unchanged; a decode error returns the input untouched so callers
// can fail open (never block billing on a decode failure).
func DecodeContentEncoding(encoding string, data []byte) ([]byte, error)
```

实现要点：
- 按 `,` 拆分多值 `Content-Encoding`，逐个逆序解码（当前只需支持 `gzip`）；
- `gzip` 分支用 `compress/gzip` + `io.ReadAll`，并加解压上限（防止 zip bomb；建议复用 `bfe_http.GetAccessibleBodySize()` 作为上限）；
- `identity`、空串、未知编码 → 原样返回；解压失败 → 原样返回并附 error，由调用方决定是否记录日志；
- `deflate` / `br` 暂不支持（现场只需 gzip），留注释作为扩展点。

放 `bfe_util` 而非模块内，便于单测与后续 `mod_body_process` 复用，避免多处逻辑分叉（与 #1364 把组合链下沉到 `bfe_model_protocol/utils` 同一取舍）。

### 步骤 2（必改，止血 + 治本）：扩宽整体解析门槛并解压解析副本

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`tokenReadResponseHandler` `:210-233`。

将「是否整体解析」判定从 `res.ContentLength >= 0` 改为「非流式的完整 body」：

```go
func isCompleteNonStreamBody(res *bfe_http.Response) bool {
	if res.ContentLength >= 0 {
		return true // 定长：沿用既有路径
	}
	// chunked：仅非 SSE 且内容类型为 JSON 时整体读入，避免在真实流式
	// 响应上阻塞（res.IsSse 由 bfe_http/response.go:214 在读取响应时置位，
	// 其 isSSEResponse 已正确处理媒体类型参数）。
	return !res.IsSse && isJSONMediaType(res.Header.Get("Content-Type"))
}
```

随后改写处理体：

```go
if res.StatusCode == bfe_http.StatusOK && isCompleteNonStreamBody(res) {
	ctx.aiBasicInfo.MarkResponseCompleted()
	var body []byte
	if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
		body, _ = bodyAccessor.GetBytes()
		// 解析副本解压：绝不动 res.Body / res.Header，转发字节不变。
		if decoded, derr := bfe_util.DecodeContentEncoding(res.Header.Get("Content-Encoding"), body); derr == nil {
			body = decoded
		} else {
			log.Logger.Warn("%s: decode content-encoding %q failed: %v",
				m.name, res.Header.Get("Content-Encoding"), derr)
		}
		UpdateCtxByUsage(ctx, body)
	}
	if tokenUsage.UsedQuota > 0 {
		ctx.aiBasicInfo.MarkFinalUsageSeen()
	}
	if tokenUsage.UsedQuota <= 0 && ctx.aiBasicInfo.IsAllowEstimateToken() {
		size := len(body)
		if size == 0 {
			size = int(res.ContentLength)
		}
		if size < 0 {
			size = 0
		}
		tokenUsage.CompletionTokens = int64(size) / 4
		tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
	}
}
```

要点：
- **估算分支修正**：chunked 时 `res.ContentLength == -1`，原式 `int64(-1)/4 == 0`（Go 向零截断），估算退化为仅按 prompt 计；改用解压后 body 长度，定长 gzip 场景也更准确（原来用的是压缩后长度）。
- **读取是有界且可重放的**：`res.GetBodyAccessor()` → `NewBytesBody`（`bfe_http/transfer.go:942-974`）把 body 读入内存（上限 `GetAccessibleBodySize()`，默认 2 MiB、可配至 8 MiB）并替换 `res.Body` 为可重放缓冲，后续 `mod_body_process` 与 `sendResponse` 读到的仍是同样的压缩字节，转发不受影响。
- **不会被下游覆盖**：`QuotaUsageProcessor.Process` 只在 `!rquota.IsGuess` 时写字段（`content_quota_usage.go:56-58`），且估算仅在 `tctx.UsedQuota <= 0` 时叠加；本路径已把 `UsedQuota` 置为正，故 gzip 下 `mod_body_process` 解析失败不会清空或污染已有结果。

### 步骤 3（必改，独立正确性修复）：解码器选择容忍媒体类型参数，且压缩体不做严格解码

文件：`bfe_modules/mod_body_process/body_process.go`，`NewContentTypeDecoder` 与其调用点 `DoRequestProcess` / `DoResponseProcess`。

- 匹配前把 content type 规范化为媒体类型（新增 `mediaTypeOnly`）：取 `;` 之前、`TrimSpace`、`ToLower`，使 `application/json; charset=utf-8` → `JsonDecoder`、`text/event-stream; charset=utf-8` → `SSEEventDecoder`。
- 对 SSE 优先采用已存在的正确信号 `res.IsSse`（`bfe_http/response.go:144-162` 的 `isSSEResponse` 已按 `;` 切分并忽略大小写）。
- **压缩体旁路（集成测试暴露的回归，必改）**：`mod_body_process` 把解码出的事件**重新编码后转发**给客户端（`GeneralEncoder` 逐字节回写 `event.ToBytes()`，`body_process.go:418`），因此严格解码器（`JsonDecoder`/`SSEEventDecoder`）一旦拿到压缩字节就会 `Decode` 报错 → `fillBuffer` 置 `bp.err` → `Read` 返回错误（`body_process.go:186-226`）→ **转发被截断为空**。改动前 `application/json; charset=utf-8` 落到宽容的 `LineDecoder`，恰好掩盖了这一点（这正是线上「只有 usage 坏、响应正常」的原因）；若只做媒体类型规范化，会把该形态导流到 `JsonDecoder` 而**打断转发**（`TestTC22` 首次运行即复现，`forwarded body is not gzip: EOF`）。
  为此新增 `hasContentEncoding`：在响应/请求两条 `default` 分支中，当 `Content-Encoding` 为非 identity 值（如 gzip）时直接选 `LineDecoder` 逐字节透传（`LineDecoder` + `GeneralEncoder` 对任意字节是恒等变换，含无尾换行的末段），usage 解析交由步骤 2 在解码副本上完成。
- `Dec` 显式配置（`sse`/`json`/`line`）优先级不变。

`mediaTypeOnly` 单独即可修复 JSON/SSE 侧失配，但**不能**替代步骤 2：即便选中 `JsonDecoder`，压缩字节仍解析不出 usage，且如上所述会打断转发。

### 步骤 4（加固，可选）：流式链路的 `Content-Encoding` 兜底

本次现场为**非流式**，步骤 2 已完整覆盖；SSE 响应在上游侧通常不压缩（若确被压缩，步骤 3 的旁路会使其逐字节透传、不解析 usage）。若后续需要支持「gzip 压缩的流式响应」并从中解析 usage，方案是给 `BodyProcessor` 增加一条「解码用副本」分支：解码器从 `gzip.Reader` 副本读取以解析 usage，编码器仍回写**原始压缩字节**给客户端，从而保持转发逐字节不变。此为独立改动（需处理增量 gzip 帧边界），不在本 issue 范围内，**本次不实施**，仅记录扩展点。

## 三、回归测试

单元测试（`testing`，放在被测代码旁）：

1. `bfe_util/content_encoding_test.go`（新增）：
   - `TestDecodeContentEncodingGzip`：gzip 往返；
   - `TestDecodeContentEncodingPassThrough`：`identity` / 空串 / 空白 → 原样返回；
   - `TestDecodeContentEncodingEmptyData`：空输入 → 空且无错；
   - `TestDecodeContentEncodingUnknownFailsOpen`：`br` → 原样返回且 error 非空；
   - `TestDecodeContentEncodingInvalidGzipFailsOpen`：非 gzip 字节 → 原样返回且 error 非空。

2. `bfe_modules/mod_ai_token_auth/nonstream_gzip_test.go`（新增）：
   - `TestTokenReadResponseHandler_ChunkedGzipAnthropic`：`ContentLength:-1` + `IsSse:false` + `Content-Type: application/json; charset=utf-8` + `Content-Encoding: gzip`，body 为 gzip(Anthropic usage)；断言 `UsedQuota=36573` / `PromptTokens=36521` / `CompletionTokens=52` / `CacheReadTokens=36096`、`IsFinalUsageSeen() && IsResponseCompleted()` 为真，并**同时**断言 `Content-Encoding` 头与重新读取的 `res.Body` 仍为原始压缩字节（证明解析用的是副本）。
   - `TestTokenReadResponseHandler_ChunkedPlainAnthropic`：同形但不压缩，单独验证步骤 2 的 chunked 门槛扩宽。
   - `TestTokenReadResponseHandler_SSEChunkedSkipped`：`IsSse:true` + `ContentLength:-1` → 断言不整体读取、不解析（流式路径不受影响）。
   - `TestTokenReadResponseHandler_ChunkedEstimateUsesBodyLen`：`ContentLength:-1` + 无 usage 的 JSON + `SetAllowEstimateToken(true)` → 估算按 body 长度而非 -1。

3. `bfe_modules/mod_body_process/content_type_decoder_test.go`（新增）：
   - `TestNewContentTypeDecoderMediaTypeParams`：`application/json` / `application/json; charset=utf-8` / `application/x-ndjson` → `JsonDecoder`；`text/event-stream` / `text/event-stream; charset=utf-8` / `application/x-sse` → `SSEEventDecoder`；`text/plain` → `LineDecoder`。
   - `TestDoResponseProcessSseContentTypeWithCharset`：`res.IsSse=true` + `text/event-stream; charset=utf-8` → 选 `SSEEventDecoder`。
   - `TestDoResponseProcessCompressedPassThrough`：`Content-Encoding: gzip` + JSON content type → 选 `LineDecoder`，且读出的字节与输入逐字节一致（步骤 3 的压缩体旁路）。

集成测试（`bfe/tests/integration`，SC03 计费场景）：

4. `sc03_rmb_quota_test.go` 新增 `TestTC22_RMBQuotaDeduction_Anthropic_NonStream_ChunkedGzip`：
   - fixture：chunked（`NoContentLength = true`）+ `Content-Encoding: gzip` + `Content-Type: application/json; charset=utf-8` + Anthropic body；`ResponseHeaders` 携带压缩头、`Body` 为预压缩字节（`gzipResponseBody`），因此**未改动** `mock_backend.go`；
   - 请求显式带 `Accept-Encoding: gzip`（新助手 `sendRequestWithHeaders`），使 net/http 不透明解压，从而能断言转发原文；
   - 断言：状态 200、命中 1 次、`quota:plan_rmb` 扣减 `900000`（`2000*100 + 8000*50 + 1500*200`），且客户端收到的 `Content-Encoding: gzip`、`gunzip(body) == 上游 body`（转发逐字节不变）。
   - 回归：`TestTC16_..._Chunked`、`TestTC17_..._ContentLength` 保持通过。

> 说明：SC03 的 `bfe.conf` 未加载 `mod_access_pb3`，故 TC-22 在集成层只断言计费与转发；日志字段（`ai_output_tokens` / `ai_total_tokens` / `ai_cache_read_tokens`）由第 2 组单测在共享 `TokenUsage` 上直接断言（访问日志读的正是该对象）。若需集成层断言日志，可另在 SC05（访问日志字段场景）补一条 gzip 用例。

验证命令（本次实跑）：

```bash
cd bfe
go test ./bfe_util/ ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_body_process/
go test ./bfe_server/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_token_auth/... \
        ./bfe_modules/mod_body_process/... ./bfe_util/...
go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ -v   # TC01~TC22
```

## 四、修复后正确路径（目标行为）

非流式 Anthropic（gzip + chunked）请求：

1. transport 原样收到 gzip body（`DisableCompression: true`），`ContentLength == -1`；
2. `tokenReadResponseHandler`：`isCompleteNonStreamBody` 为真（chunked 且非 SSE 且 JSON）→ `MarkResponseCompleted()`；
3. `GetBodyAccessor().GetBytes()` 取压缩字节 → `DecodeContentEncoding("gzip", …)` 得明文 → `UpdateCtxByUsage` 经组合链解析出完整 usage（含 `CacheReadTokens`），写入共享 `TokenUsage` → `UsedQuota > 0` → `MarkFinalUsageSeen()`；
4. `mod_body_process` 对其中的 gzip 字节解析失败（`IsGuess`）→ 不覆盖、不估算；
5. `tokenRequestFinishHandler`：`IsFinalUsageSeen()` 为真 → 守卫不触发 → `calcChatCost`：`normalInput = prompt − cacheRead − cacheWrite` 按输入全价、`cacheRead × 缓存价`、`normalOutput × 输出价`，与上游后台账单一致；
6. 访问日志 `ai_input_tokens` = 响应真实输入、`ai_output_tokens` / `ai_total_tokens` / `ai_cache_read_tokens` 均反映真实 usage（A-01 / A-02 / A-03 全部满足）；
7. 客户端仍收到原始的 gzip 字节与 `Content-Encoding: gzip`，转发逐字节不变。

## 五、边界与残余风险

- **超大非流式 body**：`GetBytes()` 受 `accessibleBodySize`（默认 2 MiB）限制；超过上限时仅前若干字节可解析（`all=false`），usage 可能仍解析失败。此为既有定长路径的同一限制，非本次引入；建议在文档中提示将该参数按最大非流式响应体调整（现场已配 4 MiB）。
- **gzip 流式响应**：步骤 4 未实施，仍不支持；已记录扩展点与前置条件。
- **多值 / 非 gzip 编码**：`DecodeContentEncoding` 对 `deflate` / `br` / 多值链仅做「原样返回 + 告警」，不阻断计费；如现场出现再扩展。
- **不改变对外语义**：无新增配置项、无模块注册顺序调整、无转发字节变化；SSE 与定长路径行为保持不变，仅新增「chunked 非流式」的成功解析与「带参数内容类型」的正确解码器选择。

## 六、实施记录（2026-10-11）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_util/content_encoding.go` | 新增 `DecodeContentEncoding(encoding, data)`（gzip/x-gzip、多值逆序；fail-open；解压上限取 `bfe_http.GetAccessibleBodySize()`） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 新增 `isJSONMediaType` / `isCompleteNonStreamBody`；`tokenReadResponseHandler` 门槛改为「定长或 chunked 非 SSE JSON」，仅解压解析副本，估算改用解码后 body 长度 |
| `bfe_modules/mod_body_process/body_process.go` | 新增 `mediaTypeOnly`（媒体类型规范化）、`hasContentEncoding`（压缩体旁路）；`NewContentTypeDecoder` 按规范化媒体类型分派；`DoResponseProcess` 压缩旁路 → `res.IsSse` → 媒体类型；`DoRequestProcess` 压缩旁路 |
| `bfe_util/content_encoding_test.go` | 新增 5 例 |
| `bfe_modules/mod_ai_token_auth/nonstream_gzip_test.go` | 新增 4 例 |
| `bfe_modules/mod_body_process/content_type_decoder_test.go` | 新增 3 例 |
| `tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | 新增 `TestTC22_..._ChunkedGzip`、`gzipResponseBody`、`sendRequestWithHeaders`（未改 `mock_backend.go`） |
| `docs/zh_cn/sys_design/rmb_quota.md` | §7.4 同步为新的 `tokenReadResponseHandler` 实现与判定 |
| `tests/integration/测试设计文档/`（SC03 场景说明、总体说明、`TC-22-*.md`） | 新增 TC-22，补齐 SC03 TC-15~TC-21 清单 |

实施中发现并一并修复的回归（由 TC-22 首次运行暴露）：媒体类型规范化会把带 `charset` 的 **gzip** 响应导流到 `JsonDecoder`，而 `mod_body_process` 是「解码→重编码转发」，严格解码器遇到压缩字节报错会使**转发截断为空**（改动前该形态落在宽容的 `LineDecoder` 才没暴露）。故在请求/响应两条 `default` 分支增加压缩体旁路（`hasContentEncoding` → `LineDecoder` 逐字节透传）。

验证结果（2026-10-11）：

| 范围 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go vet`（受影响包） | 无告警 |
| `bfe_util` / `mod_ai_token_auth` / `mod_body_process` 单测（含新增 12 例） | 全部通过 |
| `bfe_server`、`mod_ai_route`、`bfe_util/...` 单测 | 全部通过 |
| 集成 SC03 全量（TC01~TC22） | 22/22 通过 |
| 集成 SC02 / SC05 / SC06 / SC07 / SC29 / SC30 | 全部通过 |

未跑：`make test` 全仓（会拉起全部 ~30 个集成场景）。