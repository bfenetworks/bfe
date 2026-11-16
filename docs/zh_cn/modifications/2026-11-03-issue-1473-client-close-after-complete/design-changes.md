# BFE Issue #1473 修复方案：完整投递后的客户端挂断不应记为 CLIENT_WRITE

- Issue: https://github.com/bfenetworks/bfe/issues/1473
- 缺陷一句话：SSE 流式响应**已经完整投递**（协议终止事件已解析、final usage 已入账）后客户端才挂断连接，BFE 仍按"客户端写失败"记 `err_code=CLIENT_WRITE`、`err_msg` 被写成看似上游读错误的 `BodyProcessError: read tcp ... use of closed network connection`，并计入 `ErrClientWrite`；`ai_stream_truncated`（pb 字段 815）在默认配置下仍不会有置位点。
- 代码库：`bfe/`。行号以当前 HEAD `85f96495`（`v1.9.0-dev`）为准，并对照线上 tag `v1.8.8.7`（issue 观测版本）标出**已被后续修复消除的部分**。
- 设计依据：`docs/zh_cn/sys_design/ai_error_codes.md` §2.4（`CLIENT_WRITE` = 客户端中断）、`docs/zh_cn/sys_design/ai_access_log_fields.md` §1.2-1（全生命周期可观测）、`bfe-access-pb v0.3.15` 字段 815（`ai_stream_truncated`：EOF 时缺失协议终止事件）、先例 #1415/#1474。
- **需要产品裁定**（issue 明示）。核实 pb 与下游现状后的推荐已定为 **A + C**：A 修语义（不置错误码、不计 `ErrClientWrite`、`err_msg` 准确），C 加 per-request 正向标记字段 `ai_client_closed_after_complete`（pb 字段 816）。成本见 §3.1；执行顺序为 **pb 先行（§四 步骤 1）→ bfe（步骤 2-6）→ log-reader 与文档（步骤 7）**。

## 一、当前状态盘点（HEAD 相对 v1.8.8.7 已有变化）

issue 的证据基于 `v1.8.8.7`。先厘清 HEAD 已经变了的与仍然存在的：

| 要素 | v1.8.8.7 | HEAD `85f96495` | 本 issue 是否仍需处理 |
| --- | --- | --- | --- |
| 拷贝错误归因 | 任意拷贝错误 → `ErrClientWrite`（`reverseproxy.go:984-992`） | 已区分上下游：`responseCopyError{readSide, clientClosed}`（`:1094-1118`），上游断流记 `ErrBkReadRespBody`（`:1585-1596`，#1474） | **部分**。客户端断连后果仍落入 `ErrClientWrite`（`:1597-1601`），未消费"响应已完成" |
| 响应完成标记 | 事件路径置位，错误分支不消费 | 同左（`aiMeta.IsResponseCompleted()` 在 `:1602-1608` 只用于 clean-EOF 补标记） | **是**（本 issue 核心） |
| `ai_stream_truncated`（815） | 全源码无写入点（死字段） | 已接线：`streamTruncationInjector` 在**非 EOF 读错误且未见终止标记**时置位（`:112-120`），`aiErrorStreamFilter.finalize()` 在**流归一开启**时补 clean-EOF 置位（`reverseproxy_ai_error.go:466-484`）；写日志见 `mod_access_pb3/request_log.go:745-746` | **部分**。默认配置（`NormalizeUpstreamError` 未配）只装 injector，**clean-EOF 路径仍无置位点**——见 §2.6 |
| `err_msg` 指向性 | `BodyProcessError: read tcp ...` | 同左（`basicReq.ErrMsg = err.Error()`，`:1601`） | **是** |

结论：issue 的"误标成 CLIENT_WRITE"与"err_msg 误导"两环在 HEAD 依然成立；815 只剩 clean-EOF 一个缺口。

## 二、根因（HEAD 逐环核实）

### 2.1 SSE 场景客户端断连是唯一收尾触发源，且没有写超时兜底

`reverseproxy.go:922-929`（`ServeHTTPForAI`，`res.IsSse` 时）：

```go
timeoutReadClient = -1
timeoutWriteClient = -1
cancelOnClientClose = true
basicReq.IsSse = true
```

SSE 响应既没有读客户端超时，也没有写客户端超时，唯一能结束"阻塞在上游读"的事件就是客户端连接关闭。

### 2.2 CloseWatcher 主动关上游：读错误是客户端断连的**果**

`copyResponse`（`:1016-1029`）：

```go
cw := bfe_http.NewCloseWatcher(cn, func() {
	atomic.StoreInt32(&clientClosed, 1)
	src.Close()          // 客户端走了 → 关上游 body
})
```

上游 body 被本地关闭后，下一次 `src.Read` 必然报错；`newResponseCopyError(err, writeErr == nil, &clientClosed)`（`:1109-1118`）此时得到 `readSide=true, clientClosed=true`。

### 2.3 body_process 把该错误包装成"上游读错误"文案

`bfe_modules/mod_body_process/body_process.go:84-94`：`BPError.Error() = "BodyProcessError: " + inner`。内层是 `net.ErrClosed`（`use of closed network connection`），因此**日志文本指向错误方向**。

### 2.4 AI 出口二分法缺第三态：已完成 + 客户端断连

`reverseproxy.go:1579-1601`：

```go
if err != nil {
	if isUpstreamDropErr(err) {                 // readSide && !clientClosed
		basicReq.ErrCode = bfe_basic.ErrBkReadRespBody
		p.proxyState.ErrBkReadRespBody.Inc(1)
		...
	} else {
		basicReq.ErrCode = bfe_basic.ErrClientWrite   // 客户端断连后果落在这里
		p.proxyState.ErrClientWrite.Inc(1)
	}
	basicReq.ErrMsg = err.Error()                     // 误导文案
}
```

`isUpstreamDropErr` 只排除了"客户端已关"的情况，于是 `clientClosed=true` 时**一律**落到 `CLIENT_WRITE`；分支里没有任何地方看 `aiMeta.IsResponseCompleted()`。

### 2.5 完成状态在错误发生前就已经存在

- 事件路径：`mod_body_process/content_quota_usage.go:44-50`，`rquota.IsTermination` → `MarkResponseCompleted()`（`bfe_basic/request_ai_basic.go:276-281`）；
- clean-EOF 路径：`shouldMarkAIStreamCompleted`（`:1625-1629`，#1415）。

即 LATE 样本（收到 `message_stop`/`[DONE]`/`response.completed` 后才挂断）在进入错误分支时 `IsResponseCompleted()==true`，只是没人消费。

**依赖提示**：事件路径的置位来自 `mod_body_process`（`QuotaUsageProcessor`）。未加载该模块的部署里 `IsResponseCompleted()` 恒 false，本修复对该类部署不生效（见 §八 的待评估项）。

### 2.6 `ai_stream_truncated` 仅剩 clean-EOF 缺口

`normalizeUpstreamError`（`reverseproxy_ai_error.go:51-95`）的安装条件：

```go
eff := cfg.Effective()                                     // 未配置 ⇒ Enabled/StreamEnabled 均 false
if !eff.Enabled && !eff.StreamEnabled && !cfg.StreamTruncatedErrorEnabled() { return }
...
if isStream {
	if !eff.StreamEnabled {
		if cfg.StreamTruncatedErrorEnabled() {             // 默认 true
			res.Body = newStreamTruncationInjector(...)     // 只装 injector
		}
		return
	}
	inner := &aiErrorStreamFilter{...}                      // finalize() 才能覆盖 clean-EOF
	...
}
```

`StreamTruncatedErrorEnabled()` 默认 true（`cluster_conf_load.go:355-360`），所以**默认部署只装 injector**；而 injector 的 `Read`（`:96-100`）在 `err == io.EOF` 时直接返回，**不做任何标记**：

```go
if err == nil || err == io.EOF {
	return n, err        // clean EOF：sawTerminal 为假也不标记
}
```

于是"clean EOF + 缺终止事件"（issue 判据 3 的场景）在默认配置下 `ai_stream_truncated` 仍然为假——与 issue 描述的现象在 HEAD 依旧一致。

### 2.7 与 #1415 的对偶关系（设计原则一致性）

#1415 修的是"上游 clean-EOF、无终止事件 → 不能算中断"（`shouldMarkAIStreamCompleted`）；本 issue 是同一原则的镜像："**完成在前、客户端挂断在后**，不能算写失败"。两者的判定依据都是"终止事件是否已到"，只是一个走 clean-EOF、一个走事件标记。

## 三、需要产品裁定的点

issue 明示需裁定。核实 pb 与下游现状后（见 §3.1），推荐 **A + C 一起做**：A 修语义、C 给 per-request 正向标记。

| 选项 | 做法 | 优点 | 代价 |
| --- | --- | --- | --- |
| **A（必做）** | 已完成 + 客户端断连：**不置 `ErrCode`**、不增 `ErrClientWrite`、`err_msg` 写准确文案；另加 `proxy_state` 计数 `ReqAiClientClosedAfterComplete` | 判据 1 的"记录不含 `err_code:"CLIENT_WRITE"`"与"`ERR_CLIENT_WRITE` 不增"同时成立；请求从 `req_err_*` 日志流回到 `req_*`（`request_log.go:167-180` 按 `ErrCode` 分流），正是"不是错误"该有的位置 | 单靠 A 只能靠"无 err_code + 聚合计数"判读，**缺少 per-request 正向证据**（对账时无法直接筛出这一类行） |
| **C（推荐，与 A 同卡）** | 新增 pb 字段 `ai_client_closed_after_complete`（字段 **816**，810-816 块内 815 之后的第一个空位） | 语义最干净、per-request 可机检；`err_code` 为空 + 该 bool 为真，两个正向/负向证据互为校验；与 `ai_stream_truncated`(815) 同一 block、同一命名风格，读日志的人一眼能懂 | 需 `bfe-access-pb` 发一次版（见 §3.1，成本低于预期） |
| B | 复用 `ai_usage_source` 新增取值（如 `complete_client_close`） | 无需 pb 变更 | 该字段语义是"usage 来源"（`rmb_quota.md` §7.5 的六态），塞入"关闭时机"造成双语义；且它同时驱动计费分支判定，风险高于收益。不推荐 |
| D | 只降级指标（不改正错误码） | 改动最小 | 违反 `ai_error_codes.md` §2.4，判据 1 只满足一半。不推荐 |

**A 不能省**：C 若单独做（保留 `err_code=CLIENT_WRITE` + 新字段），判据 1 后半句"`ERR_CLIENT_WRITE` 计数不增加"仍不成立，§2.4 的语义违约也仍在。所以 C 是 A 的补充，不是替代。

### 3.1 C 的实际成本（核实后比原判断低）

| 环节 | 事项 | 成本 |
| --- | --- | --- |
| `bfe-access-pb` | proto 加 `optional bool ai_client_closed_after_complete = 816;` + 重新生成 `bfe_access.pb.go` + `docs/protobuf.md` 登记 + 打 tag | 该仓按"每个字段一次发版"运作：v0.3.16 加 `ai_zero_cost_reason`(765)、v0.3.17 加 `ai_price_warn`(766)，均为同批小改动。字段块 810-816 已在 proto 里显式预留，816 未被占用 |
| `bfe` | `go.mod` bump + `request_log.go` 写一行（`if aiInfo.ClientClosedAfterComplete { reqLog.AiClientClosedAfterComplete = proto.Bool(true) }`），与既有 810-815 的写法完全一致 | 1 行 + 一次依赖升级 |
| `log-reader` | `go.mod` bump 到同一版本（该仓已用 `bfe-access-pb v0.3.17`，且留了本地 `replace` 注释），转发链路不改逻辑 | 一次依赖升级 |
| `ai-gateway-observability` | **不阻塞**：810-815 这一整块（`ai_upstream_err_code`/`ai_err_normalized`/`ai_err_normalize_miss`/`ai_stream_error_rewritten`/`ai_stream_truncated`）目前**都不在** ClickHouse/Doris/StarRocks 的 SQL 与视图里，投影是选择性、按需求增量的。要进报表时再补列 | 0（本卡范围内） |

唯一的顺序约束：**pb 先发版 → bfe/log-reader bump**（即 §四 的步骤 1 → 步骤 2）。发版若不可行，退回 A-only（见步骤 1 的说明），语义与计数仍成立。

裁定不影响的部分：判据 2（中断语义不回归）、判据 3（815 接线）、判据 4（`err_msg` 准确化）在任一选项下都应做。

## 四、修复方案

执行顺序：**先发 pb（步骤 1）→ 再动 bfe（步骤 2-6）→ 最后同步 log-reader 与文档（步骤 7）**。这样 bfe 侧任何一步合入时字段都已可用，不会出现"代码引用了不存在的 pb 字段"的中间态。

| 步骤 | 仓 | 内容 | 依赖 |
| --- | --- | --- | --- |
| 1 | `bfe-access-pb` | 字段 816 + 发版 v0.3.18 | 无（纯新增，向后兼容） |
| 2 | `bfe` | `go.mod` 升级到 v0.3.18 | 步骤 1 |
| 3 | `bfe` | AI 出口三态（语义 + 计数 + 置标记） | 步骤 2 |
| 4 | `bfe` | 访问日志写出字段 816 | 步骤 3 |
| 5 | `bfe` | `err_msg` 准确化 | 步骤 3（可与 4 同 PR） |
| 6 | `bfe` | 815 的 clean-EOF 接线 | 无（可与 3/4/5 并行） |
| 7 | `log-reader` + 文档 | 依赖同步 + 三份设计文档 | 步骤 1、步骤 4 |

### 步骤 1（必改，先行）：`bfe-access-pb` 新增字段 `ai_client_closed_after_complete`（816）并发布

`bfe_access_pb/bfe_access.proto` 的 810-816 块内、815 之后追加：

```proto
    // Response fully delivered (protocol terminal event parsed) and the client
    // then closed the connection. Not a client abort and not a write failure:
    // err_code stays empty for these requests (issue #1473).
    optional bool ai_client_closed_after_complete = 816;
```

同步：重新生成 `bfe_access.pb.go`、`docs/protobuf.md` 登记 816、`CHANGELOG.md` 记一条（照 v0.3.16/v0.3.17 的写法），打 tag **v0.3.18**。

> 若该发版不可行：退回 §三 的 A-only 形态（语义与计数仍成立，仅缺 per-request 正向标记），步骤 2/4 与 log-reader 的 bump 一并取消，其余步骤不受影响。

### 步骤 2（必改）：`bfe` 升级 `bfe-access-pb` 依赖

`go.mod` bump 到 v0.3.18，`go build ./...` 验证通过。本步无行为变化：字段在步骤 3/4 接线前恒不写出。

### 步骤 3（必改）：给 AI 出口补第三态"已完成 + 客户端断连"

`bfe_server/reverseproxy.go`，新增判定 helper（与 `isUpstreamDropErr` 并列）：

```go
// isClientAbortConsequence reports whether a sendResponse error is the
// consequence of the client going away rather than a fault of its own: the
// cancelOnClientClose watcher fired, or the write to the client failed
// (issue #1473). In both cases a read-side error on src is collateral.
func isClientAbortConsequence(err error) bool {
	var ce *responseCopyError
	if !errors.As(err, &ce) {
		return false
	}
	return ce.clientClosed || !ce.readSide
}
```

AI `send_response` 错误分支（`:1579-1601`）改为三态，判定收敛到一个 helper：

```go
switch {
case isUpstreamDropErr(err):
	// 既有 #1474 分支，不变
	basicReq.ErrCode = bfe_basic.ErrBkReadRespBody
	p.proxyState.ErrBkReadRespBody.Inc(1)
	if shouldMarkUpstreamDropped(res, aiMeta) {
		aiMeta.MarkUpstreamDropped()
	}
case isClientCloseAfterComplete(err, aiMeta):
	// issue #1473：响应已完整投递（终止事件已解析），客户端在之后挂断。
	// 不是中断，不是写失败，不记错误码、不计写错误。
	aiMeta.ClientClosedAfterComplete = true
	p.proxyState.ReqAiClientClosedAfterComplete.Inc(1)
default:
	// 终止事件前断开 / 写失败 / 其它拷贝错误：沿用 CLIENT_WRITE
	basicReq.ErrCode = bfe_basic.ErrClientWrite
	p.proxyState.ErrClientWrite.Inc(1)
}
if basicReq.ErrCode != nil {
	basicReq.ErrMsg = clientAbortErrMsg(err)     // 步骤 5
}
```

判定 helper（与 `isUpstreamDropErr` 并列）：

```go
// watcher 已触发且响应已完成：唯一够格走"完整投递后挂断"的组合
func isClientCloseAfterComplete(err error, aiMeta *bfe_basic.AiBasicInfo) bool {
	var ce *responseCopyError
	if !errors.As(err, &ce) {
		return false
	}
	return ce.clientClosed && aiMeta != nil && aiMeta.IsResponseCompleted()
}
```

> 实施时的收紧（相对初版本文案）：初稿把"写失败"（`!ce.readSide`）也算作本态，但那意味着最后一批字节**可能没到客户端**，按写错误记账更诚实。最终只有"CloseWatcher 触发 + 响应已完成"才降级，写失败仍记 `CLIENT_WRITE`。

配套：

- `bfe_basic/request_ai_basic.go`：新增公开布尔字段 `ClientClosedAfterComplete`（与 `StreamTruncated` 同形，直接赋值/直读，步骤 4 无需访问器）；
- `bfe_server/proxy_state.go`：新增 `ReqAiClientClosedAfterComplete *metrics.Counter`（`metrics.Init` 反射建计数器，无需其它注册）；
- 归因逻辑抽成 `(*ReverseProxy).recordCopyError(basicReq, res, aiMeta, err)`，`ServeHTTPForAI` 只调一行——这样归因矩阵可直接单测；
- 不改 `action`/连接关闭逻辑：已断连的连接本就无需 `closeAfterReply`，沿用现状即可。

### 步骤 4（必改）：`mod_access_pb3` 写出字段 816

`bfe_modules/mod_access_pb3/request_log.go`，紧跟 810-815 那段（`if aiInfo.StreamTruncated { reqLog.AiStreamTruncated = proto.Bool(true) }`）之后：

```go
if aiInfo.ClientClosedAfterComplete {
	reqLog.AiClientClosedAfterComplete = proto.Bool(true)
}
```

与既有 812-815 一致：只在为真时写出，`optional bool` 缺省即"未发生"，正常请求的行里不会出现该字段。

### 步骤 5（必改）：`err_msg` 不再把客户端断连写成上游读错误

```go
// clientAbortErrMsg returns a stable, direction-correct message for a copy
// error caused by the client going away; the raw error stays in the log
// (logCopyError) so the transport detail is not lost (issue #1473).
func clientAbortErrMsg(err error) string {
	var ce *responseCopyError
	if errors.As(err, &ce) {
		switch {
		case ce.clientClosed:
			return errMsgClientClosed      // "client closed connection"
		case !ce.readSide:
			return errMsgClientWriteFailed // "client write failed"
		}
	}
	return err.Error()
}

// 原始错误（含 readSide / clientClosed 归因）只进 bfe.log
func logCopyError(req *bfe_basic.Request, err error) {
	var ce *responseCopyError
	if !errors.As(err, &ce) {
		log.Logger.Info("sendResponse copy error, logid[%s], err[%v]", req.LogId, err)
		return
	}
	log.Logger.Info("sendResponse copy error, logid[%s], readSide[%v], clientClosed[%v], err[%v]",
		req.LogId, ce.readSide, ce.clientClosed, ce.err)
}
```

两种文案都是"客户端侧"的结论，不再指向上游；上游读错误仍保留原文（走 `ErrBkReadRespBody`）。原始 `err` 用 `log.Logger.Info` 记一行（含 `readSide`/`clientClosed`），供排查。

判据 4 要求"不得只靠 `err_msg` 文本区分"——区分靠 `err_code`（空 vs `CLIENT_WRITE`）与计数，文案只负责不再误导。

### 步骤 6（必改，判据 3）：injector 在 clean EOF 且无终止标记时置 `ai_stream_truncated`

`bfe_server/reverseproxy_stream_inject.go` 的 `Read`：

```go
n, err := f.src.Read(p)
if n > 0 {
	f.scanTerminal(p[:n])
}
if err == io.EOF {
	// issue #1473：clean EOF 且未见协议终止事件 ⇒ 流被截断（字段 815 的定义）。
	// 只在终止标记已知（markers 非空）时判定：未知适配器无从判断，宁可不错标。
	if len(f.markers) > 0 && !f.sawTerminal() && f.aiMeta != nil && !f.aiMeta.StreamTruncated {
		f.aiMeta.StreamTruncated = true
		incStateCounter(f.state.ErrStreamTruncated)
	}
	return n, err
}
```

要点：

- **Gemini 豁免已内建**：`newStreamTruncationInjector` 对 `StreamEndsAtEOF()` 适配器直接返回（`markers` 为 nil），上面的 `len(f.markers) > 0` 守卫让它不出错；
- **不在 clean EOF 注入错误事件**：客户端已拿到全部字节，#1474 的注入只针对读错误；此处只标记；
- 与 `aiErrorStreamFilter.finalize()` 的置位**去重**：两者都可能命中，用 `!f.aiMeta.StreamTruncated` 幂等保护（`finalize` 在 StreamEnabled 时也可能先置位，顺序不影响结果）；
- **残余风险**：`scanTerminal` 是逐次读块的 `strings.Contains`，终止标记跨块拆分时会漏判 → clean EOF 时误标截断。实施时把标记扫描改为"保留上一块尾部 `maxMarkerLen-1` 字节再拼接"，或在 EOF 时用拼接过的最小窗口判定。

### 步骤 7（必改）：`log-reader` 依赖同步 + 设计文档

**7.1 `log-reader`**

`go.mod` bump `github.com/bfenetworks/bfe-access-pb` 到 v0.3.18（转发链路无需改逻辑）。若该仓对 `BfeLog` 做显式字段映射，需同步映射新字段——实施时确认。

**7.2 bfe 设计文档**

- `docs/zh_cn/sys_design/ai_error_codes.md` §2.4：补一行"响应已完整投递后的客户端挂断"——不产生错误码，`err_code` 为空，携带 `ai_client_closed_after_complete=true`，计入 `ReqAiClientClosedAfterComplete`；
- `docs/zh_cn/monitor/proxy_state.md`：新增 `REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE` 一行；
- `docs/zh_cn/sys_design/ai_access_log_fields.md`：`err_code` 语义补"空值 = 无错误，含完整投递后的客户端挂断"；登记新字段 `ai_client_closed_after_complete`（816）；`ai_stream_truncated` 补"默认配置下 clean-EOF 缺终止事件同样置位"。

**7.3 pb 仓文档**

`bfe-access-pb` 的 `docs/protobuf.md`、`CHANGELOG.md` 已在步骤 1 随字段一起更新，此处只做交叉引用确认。

## 五、行为变化清单

| 项 | 修复前 | 修复后 |
| --- | --- | --- |
| 完整投递后客户端挂断（LATE） | `err_code=CLIENT_WRITE`、`ErrClientWrite+1`、`err_msg=BodyProcessError: ...`、写入 `req_err_*` 日志流、无正向标记 | `err_code` 为空、`ai_client_closed_after_complete=true`（字段 816）、`ReqAiClientClosedAfterComplete+1`、`ErrClientWrite` 不增、`err_msg="client closed connection"`、回到 `req_*` 日志流 |
| 终止事件前断连（EARLY，真实中断） | `CLIENT_WRITE` + `ErrClientWrite+1` + 误导文案 | 不变（仅 `err_msg` 变 `"client closed connection"`） |
| 上游中途断流（#1474） | `ErrBkReadRespBody`、`ai_usage_source=upstream_dropped` | 不变 |
| clean EOF 缺终止事件（默认配置） | `ai_stream_truncated` 不置位 | 置位 + `ErrStreamTruncated+1`（Gemini 豁免；#1415 的 `estimated` 计费不变） |
| 计费 | LATE: `ai_usage_source=parsed`，正常入账 | 不变（`IsResponseCompleted` 与 `IsFinalUsageSeen` 均不变；`aborted` 分支仅在 `!IsFinalUsageSeen` 时生效，`mod_ai_token_auth.go:447-469`） |
| 非 AI 路径（`reverseproxy.go:984-1000`） | 任何拷贝错误 → `CLIENT_WRITE` | **不动**（无"完成"概念，属另一议题） |

## 六、回归与验证

### 单元测试（`bfe_server`）

1. `TestIsClientAbortConsequence`：`{readSide:true, clientClosed:true}` → true；`{readSide:false}` → true；`{readSide:true, clientClosed:false}` → false；非 `responseCopyError` → false。
2. `TestSendResponseClientCloseAfterComplete`：构造 `IsSse` 响应 + `aiMeta` 已 `MarkResponseCompleted()`，注入 watcher 触发后的读错误 → 断言 `ErrCode == nil`、`ErrClientWrite` 计数不增、`ReqAiClientClosedAfterComplete==1`、`ErrMsg=="client closed connection"`。
3. 对偶：同一构造但 `IsResponseCompleted()==false` → `ErrCode==ErrClientWrite`、计数 +1（判据 2 不回归）。
4. `TestStreamTruncationInjectorCleanEOF`：markers 已知 + 无终止标记 + EOF → `StreamTruncated` 置位；有终止标记 + EOF → 不置位；Gemini 适配器（`StreamEndsAtEOF`）→ 不置位。
5. 既有 `reverseproxy_stream_completion_test.go`（client abort 不置完成）与 `reverseproxy_ai_error_test.go` 保持绿。

### 集成测试（`bfe/tests/integration`）

参考现成的 SSE 强相关场景：`scenario-SC29-usage-snapshot-billing/sc29_upstream_drop_test.go`（#1474 上游断流的同一套机制：SSE mock 后端 + `common.ParseAccessLogAfterStop` + `GetErrCode()`/`GetAiUsageSource()` 断言）。

**已落地**（放在现成场景里，复用其裸 TCP 客户端与访问日志解析能力，未新开场景）：

- **SC12 TC-05（判据 1/4）** `TestTC05_ClientCloseAfterCompleteIsNotAClientWrite`：mock 后端按 `message_start → content_block_delta → message_delta(usage) → message_stop` 发送并 `SSEHold` 保持连接（生产 keep-alive 形态，确保客户端 FIN 是结束拷贝的唯一事件）；客户端读到 `message_stop` 后直接关闭连接。断言：`REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE=1`、`ERR_CLIENT_WRITE=0`、配额扣减等于实际 usage（与 SC12 TC-03 同值）、访问日志 `err_code`/`err_msg` 为空、`ai_client_closed_after_complete=true`、`ai_stream_truncated` 未置位、`ai_usage_source=parsed`。
- **SC12 TC-06（判据 2）** `TestTC06_ClientCloseBeforeTerminalEventStaysAClientWrite`：同样 hold 住流，客户端在 `message_delta` 后（`message_stop` 之前）关闭。断言：`ERR_CLIENT_WRITE>=1`、`REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE=0`、`err_code=CLIENT_WRITE`、字段 816 未置位、`err_msg` ∈ {`client closed connection`,`client write failed`}（明确不是上游读错误文案）。
- **SC29 TC-04/TC-05（判据 3）**：既有的 clean-EOF 估算计费用例补断言 `ai_stream_truncated=1`（openai 无 `[DONE]` / anthropic 无 `message_stop`），并确认 `ai_client_closed_after_complete` 未置位、`ai_usage_source=estimated` 与估算计费不变。该场景的 cluster **未配 `NormalizeUpstreamError`**，正好覆盖"默认配置下 injector 是唯一在看的组件"这条路径。
- **正常完成臂**（判据 5）：SC12 TC-03（读到 EOF 才结束）保持原有断言，新字段不出现，即"不污染正常行"由既有用例兜住。

为支持访问日志断言，SC12 场景在本次一并加载了 `mod_access_pb3`（`testdata/bfe.conf` + `testdata/mod_access_pb3/mod_access_pb3.conf`），并新增 `proxyStateMetric` / `waitProxyStateMetric` / `lastRequestLog` 三个 helper。

### 手工/生产验收（判据 5 与上线观察）

- 非流式与正常流式请求的访问日志字段全集不变（对比上线前后抽样）；
- 上线后 `ERR_CLIENT_WRITE` 应回落到"真实中断"量级（生产基线 877/153,357 ≈ 0.57%，其中约 52% 应迁出）；`REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE` 应≈迁移量；
- `ai_stream_truncated` 从 0 变为非 0（clean-EOF 截断的真实量级），且与 `ai_usage_source=estimated` 的分布自洽。

## 七、验收标准（对应 issue 判据）

1. 完整 SSE + 客户端收到终止事件即关：pb3 不含 `err_code:"CLIENT_WRITE"`、携带 `ai_client_closed_after_complete=true`，`ERR_CLIENT_WRITE` 不增，`ai_usage_source`/`ai_total_tokens`/`ai_cost_value` 与正常完成一致（条款 §2.4；TC-A）；
2. 终止事件前关闭：仍记 `CLIENT_WRITE`，且与判据 1 可机检区分（TC-B）；
3. clean-EOF 无终止事件：`ai_stream_truncated=1`（Gemini 豁免），且 `ai_usage_source=estimated`、`ai_cost_value>0` 不回归（步骤 6 + TC-C）；
4. `err_msg` 不再把客户端断连写成 `BodyProcessError: read tcp ...`（步骤 5）；
5. 相邻不回归：非流式/正常流式字段不变；`unrecognized` 口径与 `UsageUnrecognized` 计数不变；`ai_stream_error_rewritten`(814) 逻辑不变；`go build/vet ./...`、`go test ./bfe_server/`、SC29 全绿。

## 八、边界与非目标

- **只改 AI 出口**：非 AI 的 `send_response`（`:984-1000`）无"响应完成"语义，维持 `CLIENT_WRITE`；`mod_access` 文本日志随 `ErrCode` 自然变化，无需改代码。
- **字段只在唯一场景置位**：`ai_client_closed_after_complete` 只在"AI SSE + `IsResponseCompleted()` 为真 + 客户端侧断连"时写出（`optional bool`，按 812-815 的既有风格仅在为真时写），非 AI、非流式、正常完成（读到 EOF）三类请求一律不出现该字段，避免污染现有行。
- **不自造"完成"**：判定只用既有 `IsResponseCompleted()`（事件路径 / clean-EOF 路径）。未加载 `mod_body_process` 的部署拿不到事件完成标记 → 该部署下 LATE 仍记 `CLIENT_WRITE`。步骤 6 的 injector 改动可以让"终止标记已见"独立于 body_process，但**不改变** `IsResponseCompleted` 的语义，所以若要让无 body_process 部署也生效，需另加 `MarkResponseCompleted()` 到 `scanTerminal`——本方案把它列为**待评估项**（会影响估算计费门控，需与 #1415/#1478 一起评审），不在默认改动内。
- **不改 close 时序**：issue 候选④"转发完终止事件后主动关上游"能消除竞态，但会改变上游连接复用与 `res.Body.Close()` 语义，收益不抵风险，本卡不做。
- **Gemini 恒不标记截断**：`StreamEndsAtEOF` 适配器按协议定义豁免，保持现状。
- **计费口径零变化**：本方案只动 `ErrCode`/指标/日志字段，不改 `IsFinalUsageSeen`/`IsResponseCompleted`/估算门控。

## 附：条款对照

| 断言 | 条款 | 落点 |
| --- | --- | --- |
| A-01 | `ai_error_codes.md` §2.4：`CLIENT_WRITE/CLOSE/RESET` = 客户端中断 | 步骤 3 的第三态 |
| A-02 | `ai_access_log_fields.md` §1.2-1：全生命周期可观测、可区分 | 步骤 3（`err_code` 空 + 新计数）+ 步骤 4（字段 816 的 per-request 标记） |
| A-03 | `bfe-access-pb` 字段 815：EOF 时缺失协议终止事件 | 步骤 6 |
| A-04 | `rmb_quota.md` §7.5 / #1410：未识别与中止口径不回归 | §八 计费零变化 + 判据 5 |

## 九、实施记录（2026-11-03）

### 9.1 已落地改动

| 步骤 | 文件 | 改动 |
| --- | --- | --- |
| 1 | `bfe-access-pb`（另一仓） | 新增 `ai_client_closed_after_complete = 816`，`build.sh` 重新生成；`docs/protobuf.md`、`CHANGELOG.md` 同步；**v0.3.18 已发布**（tag + GitHub Release，fork 与 upstream 同指向 `5293a85`） |
| 2 | `go.mod` | `github.com/bfenetworks/bfe-access-pb` v0.3.17 → **v0.3.18** |
| 3 | `bfe_server/reverseproxy.go` | 新增 `isClientAbortConsequence` / `isClientCloseAfterComplete` / `clientAbortErrMsg` / `logCopyError` / `(*ReverseProxy).recordCopyError`；`ServeHTTPForAI` 的 `send_response` 错误分支改为三态，归因逻辑下沉到 `recordCopyError` 便于单测 |
| 3 | `bfe_basic/request_ai_basic.go` | 新增公开字段 `ClientClosedAfterComplete bool`（与 `StreamTruncated` 同形，直读直写） |
| 3 | `bfe_server/proxy_state.go` | 新增计数 `ReqAiClientClosedAfterComplete`（导出名 `REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE`，`metrics.Init` 反射建计数器） |
| 4 | `bfe_modules/mod_access_pb3/request_log.go` | 紧跟 `ai_stream_truncated` 之后写出字段 816（仅 true 时写） |
| 5 | `bfe_server/reverseproxy.go` | `err_msg` 客户端侧统一为 `client closed connection` / `client write failed`；原始传输错误（含 `readSide`/`clientClosed`）只进 `bfe.log` |
| 6 | `bfe_server/reverseproxy_stream_inject.go` | injector `Read` 在 `io.EOF` 且未见终止标记时置 `ai_stream_truncated`（`len(markers)>0` 守卫、Gemini 自动豁免、`!StreamTruncated` 幂等）；`scanTerminal` 增加上一块尾部重叠窗口，修掉跨块漏判 |
| 7 | `docs/zh_cn/sys_design/ai_error_codes.md` §2.4 | 新增"完整投递后的客户端挂断"行 + `CLIENT_WRITE` 语义说明 |
| 7 | `docs/zh_cn/monitor/proxy_state.md` | 新增 `REQ_AI_CLIENT_CLOSED_AFTER_COMPLETE` 段 |
| 7 | `docs/zh_cn/sys_design/ai_access_log_fields.md` | 登记字段 816、更新 815 的生效条件（默认生效、clean-EOF 也置位）、字段计数 47→48 |

### 9.2 实施中相对方案的收紧

1. **写失败不再降级为"完整投递后挂断"**。初稿用 `isClientAbortConsequence`（`clientClosed || !readSide`）判定，但写失败意味着最后一批字节可能没到客户端，按写错误记账更诚实。最终只有 `ce.clientClosed && IsResponseCompleted()` 走新态；写失败仍记 `CLIENT_WRITE`，`err_msg` 为 `client write failed`。
2. **`err_msg` 分两种文案**（`client closed connection` / `client write failed`），而不是统一一句，便于不带日志也能区分断连与写失败。
3. **跨块终止标记**：原实现的 `scanTerminal` 逐块 `strings.Contains`，标记跨读块会漏判。因本卡让"未见标记 + clean EOF"变成截断判定，漏判会直接把正常流标成截断，故一并加了重叠窗口（保留上一块尾部 `maxMarkerLen-1` 字节，且拷贝避免引用调用方缓冲区）。
4. **线级标记 vs 事件解析（写测试时发现的真实缺陷）**：anthropic 适配器的终止标记是 `event: message_stop`，而只发 `data:` 行的兼容上游永远匹配不到。第一版改动因此把这类**已完成**的流也标成截断，并在客户端断连时向它补发一个错误事件。现引入 `streamTruncationInjector.terminated()`：线级标记命中 `OR` 事件解析器已判完成（`aiMeta.IsResponseCompleted()`）即为"已终止"，clean EOF 的截断判定与读错误的注入判定都用它。这样既不误标，也不再向已完成的流注入噪声事件。

### 9.3 测试

单元测试（`bfe_server/reverseproxy_client_close_test.go`，全部 PASS）：

- `TestIsClientAbortConsequence`：watcher / 写失败 / 上游读失败 / 普通错误 / nil 五个分支；
- `TestIsClientCloseAfterComplete`：完成+watcher=true；完成但写失败、未完成、上游读错误、nil meta 均为 false；
- `TestClientAbortErrMsg`：两种客户端文案 + 上游错误保留原文；
- `TestRecordCopyError`：五态归因矩阵（完成+挂断 / 中断 / 写失败 / 上游断流 / 非 AI），逐项断言 `err_code`、`err_msg`、三个计数器与两个 AI 标记；
- `TestStreamTruncationInjectorCleanEOF`：clean-EOF 置位、终止事件抑制、Gemini 豁免、一字节一读的跨块标记不漏判。

全仓测试（`go test ./...`）：

- `go build ./...`、`go vet ./...` 通过；
- 101 个包 + 全部集成场景跑完，失败仅 3 处：`scenario-SC13-tls-conf-reload-path` 2 条（**与本次无关**，Windows 路径/大小写的既有问题，改前即存在）、`scenario-SC20-ai-cache-exact-match` 的 `TestTC13_CacheHitSkipsRateLimit` 1 条（`Post ...: EOF`，负载下的瞬时连接问题，单独重跑与整包重跑均 PASS）；
- 直接相邻的场景单独重跑全绿：SC12（客户端中断计费）、SC27（上游错误归一，含 `ai_stream_truncated` 断言）、SC29（上游断流计费）、SC20（AI 缓存）。

集成测试（`bfe/tests/integration`，与实现同批落地）：

- **SC12**（`scenario-SC12-client-abort-billing`）新增 `TestTC05_ClientCloseAfterCompleteIsNotAClientWrite`（判据 1/4）与 `TestTC06_ClientCloseBeforeTerminalEventStaysAClientWrite`（判据 2），两者都用 `SSEHold` 让上游保持连接、客户端读到指定标记后关闭 TCP，使 CloseWatcher 路径确定复现；场景同时加载 `mod_access_pb3` 以便断言访问日志。实跑 PASS。
- **SC29**（`scenario-SC29-usage-snapshot-billing`）在既有 clean-EOF 用例 TC-04（openai 无 `[DONE]`）/ TC-05（anthropic 无 `message_stop`）上补 `ai_stream_truncated=1` 与 `ai_client_closed_after_complete` 缺席的断言（判据 3）。实跑 PASS。
- 相邻场景整包回归：SC12 / SC29 / SC27（上游错误归一）/ SC20（AI 缓存）全绿；`go test ./bfe_server/` 全绿。
- 设计文档同步：`测试设计文档/scenario-SC12-客户端中断计费/` 新增 TC-05、TC-06 两份用例文档并更新场景说明（配置文件、用例列表、TC-04 过时说明）；`scenario-SC29-.../场景说明.md` 的 TC-04/TC-05 行补 issue #1473 断言。

### 9.4 log-reader（确认无需改动）

`log-reader` 走 `reader_modules/mod_fields.Extract` + `OutputFields` 白名单投影到 Kafka JSON；810-815 这一整块（`ai_upstream_err_code` / `ai_err_normalized` / `ai_err_normalize_miss` / `ai_stream_error_rewritten` / `ai_stream_truncated`）在该白名单里**本来就没有映射**，因此本卡也与兄弟字段保持一致——不新增映射、不 bump `go.mod`。若后续要把这组字段接入报表/数仓，`mod_fields.Extract`、`OutputFields`、observability 三处需一起补，属独立议题。
