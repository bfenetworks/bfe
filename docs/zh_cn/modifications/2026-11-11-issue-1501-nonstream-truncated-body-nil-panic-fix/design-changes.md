# BFE Issue #1501 修复方案：非流式截断响应置空 body 引发 JsonDecoder nil panic，RequestLog 与计费信号全失

- Issue: https://github.com/bfenetworks/bfe/issues/1501
- 缺陷：`stream=false` 请求 `/v1/messages`，上游返回 HTTP 200（`Content-Length: 827`）发送部分 JSON 后断开时，BFE 在 `mod_body_process.(*JsonDecoder).Decode` 发生 **nil pointer panic**（`encoding/json.(*Decoder).refill` 对 nil reader 解引用）。panic 被 `conn.serve.func1` recover 后连接中断——**该请求没有 RequestLog 行**，`err_code=BK_READ_RESP_BODY`、usage source、`UpstreamDropBilled`/quota 全部无法归因；客户端只见 `unexpected EOF`。违反 `2026-10-30-issue-1474-upstream-midstream-drop-fix/design-changes.md`「非流式边界」①②③（① `err_code=BK_READ_RESP_BODY`；② 按已转发内容估算计费；③ 不补发终止 chunk，客户端得到截断传输错误）与 `sys_design/ai_error_codes.md` §5。**不需要产品裁定**（panic 与 RequestLog 缺失均有运行日志与源码链实锤）。
- 代码库：`bfe/`（行号基于当前 HEAD `5611f8f1`，逐环核实）

## 一、根因（全环源码实锤）

### 1.1 body 被置空：GetBodyAccessor 的失败副作用

`bfe_http/response.go:303-313`：

```go
r.Body, err = NewBytesBody(r.Body, GetAccessibleBodySize())  // ← 错误时把 nil 赋给 r.Body
if err != nil {
    return nil, errors.New("can't get body")
}
```

`bfe_http/transfer.go:997-1010`：`newBytesBody` 用 `io.ReadAll` 读上游 body，**截断时返回 `(nil, err)`**——已读前缀随 `bb` 一起丢弃。于是 `r.Body` 被赋 nil，而这是**写回响应对象的永久副作用**。

### 1.2 panic：nil reader 进入 JsonDecoder

- 模块顺序（`bfe_modules/bfe_modules.go:155/189`）：`mod_ai_token_auth` 的 `HandleReadResponse`（`tokenReadResponseHandler`）**先于** `mod_body_process` 执行；
- `mod_ai_token_auth.go:395-399`：`isCompleteNonStreamBody` 只看 `ContentLength >= 0`（截断响应也满足）⇒ 进入完整 body 分支，调用 `GetBodyAccessor()`——在这里 eager `io.ReadAll` 阻塞至上游断开 ⇒ 失败 ⇒ **r.Body 已被置空**（调用点以 `err == nil` 静默跳过解析，未感知副作用）；
- `mod_body_process/body_process.go:336`：`DoResponseProcess` 用 `res.Body`（=nil）构造 `NewBodyProcessor`；`:353`/`:644` `NewJsonDecoder(nil)` ⇒ `json.NewDecoder(nil)` 不报错但存储 nil reader ⇒ 首次 `Decode` → `refill` → nil 解引用 **panic**（`body_process.go:597`）；
- panic 经 `BodyProcessor.Read`（`:188`）→ `CopyWithoutBuffer` → `copyResponse`（`reverseproxy.go:1046`）→ `sendResponse` → `ServeHTTPForAI` 上抛，`http_conn.go:283` recover 后**连接在 `mod_access_pb3.requestFinishHandler` 之前终止** ⇒ 无 RequestLog。

### 1.3 语义叠加损伤（ring 1 与放大因素，实锤）

- `MarkResponseCompleted()`（`mod_ai_token_auth.go:396-397`）在 `GetBodyAccessor` **之前**无条件调用——截断响应被提前标记"完成"，#1474 的 upstream-dropped 归因与计费门控被污染；
- 估算播种分支（`:405-409`）：`GetBodyAccessor` 失败时 `bodyLen==0` ⇒ 回退用 `res.ContentLength`（827）估算 completion——**为从未转发的内容播种 token**，与条款②"按已转发内容估算"直接冲突（panic 先于计费发生，故未实际入账，但修复 panic 后若不同步修此点必错）。

## 二、修复设计

原则：**消副作用（不置空）、加防御（nil 不 panic）、正语义（截断不归 completed、不播种）**——三条各自独立成立，缺一不可。截断请求的归因与计费完全交给 #1474 既有路径（`copyResponse` 读错误 ⇒ `BK_READ_RESP_BODY` + `MarkUpstreamDropped()`，`reverseproxy.go` `shouldMarkUpstreamDropped`/`recordCopyError`），不新造机制。

### 步骤 1（核心）：GetBodyAccessor 失败时保留原 body

`bfe_http/response.go`：

```go
	newBody, err := NewBytesBody(r.Body, GetAccessibleBodySize())
	if err != nil {
		// issue #1501: never nil out r.Body here. io.ReadAll may have
		// consumed a prefix before the truncation error; the retained
		// stream only yields that read error, and the forwarding path
		// terminates the response as truncated (issue #1474 yardstick).
		// The wrapped error keeps io.ErrUnexpectedEOF for errors.Is.
		return nil, fmt.Errorf("can't get body: %w", err)
	}
	r.Body = newBody
```

效果：转发路径保留原流（残余前缀已被 `io.ReadAll` 消费，实际拿到的是读错误而非残余字节——评审修订已修正本表述），客户端得到截断传输错误（条款③）；`copyResponse` 读到错误走 #1474 归因（条款①）。实现形态为 `fmt.Errorf("can't get body: %w", err)`，底层 `io.ErrUnexpectedEOF` 不丢弃（评审第 8 点）。

### 步骤 2（防御纵深）：mod_body_process 对 nil body 永不 panic

> **PR #1504 评审修订（2026-11-11，见 `2026-11-11-issue-1501-pr1504-review-revision/design-changes.md`）**：本节初版实现（`DoResponseProcess` nil 早退）被评审判定为"把 panic 平移到下游"——调用方丢弃返回值后 `copyResponse`/`res.Body.Close()` 仍 nil panic，且破坏了 `CreateEventDecoder` 既有的 `bp.err` 正确降级。修订后：

- **删除 nil 早退**；nil body 的降级完全由构造期守卫驱动：`NewJsonDecoder` / `NewLineDecoder` / `NewSSEEventDecoder` / `NewContentTypeDecoder` 对 nil source 构造期返回 error（SSE 与 content-type 分支为评审补全的同族守卫）；
- `CreateEventDecoder` 收到工厂错误时写入 `bp.err` 并返回——`bp` **非 nil**，调用点以 `res.Body = bp` 替换后，转发路径 `Read` 返回 `bp.err`（归因 `BK_READ_RESP_BODY`），`Close` 经新增的 nil-source 守卫安全返回；
- `NewBodyProcessor` 保持 nil source 原样传入（归一化为 `EofReader` 会使工厂守卫失效——工厂收到非 nil reader 后构造成功、错误丢失，此路在评审修订实现时被验证否决），nil-source 安全性由 `BodyProcessor.Close` 的 nil 守卫兜底。

### 步骤 3（语义修复）：截断响应不置 completed、不播种估算

`mod_ai_token_auth.go` `tokenReadResponseHandler` 完整 body 分支重排：

```go
	if res.StatusCode == bfe_http.StatusOK && isCompleteNonStreamBody(res) {
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil {
			// body fully buffered: the response IS complete (issue #1364).
			ctx.aiBasicInfo.MarkResponseCompleted()
			body, _ := bodyAccessor.GetBytes()
			// ... 解压副本、UpdateCtxByUsage、finalUsageSeen（既有）...
			if !usagePresent && ctx.aiBasicInfo.IsAllowEstimateToken() {
				// estimate seeding only when the full body was actually read
				// (issue #1501: never seed from ContentLength on truncation).
				tokenUsage.CompletionTokens = int64(len(body)) / 4
				tokenUsage.UsedQuota = CalcReqUsedQuota(req, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
			}
		}
		// GetBodyAccessor error (truncated body): no completion mark, no
		// seeding — the copy path reports BK_READ_RESP_BODY /
		// upstreamDropped (issue #1474) and billing accumulates only
		// forwarded content via QuotaUsageProcessor.
	}
```

要点：① `MarkResponseCompleted` 移到"body 完整读入"之后——截断时 `responseCompleted=false`；② 估算播种从 `bodyLen==0 ⇒ ContentLength 回退` 改为只在完整读入时执行，基数=真实 body 长度；③ 完整 body 的成功路径（#1364/#1406/#1415）语义逐字节不变。

### 步骤 4（测试）

> **PR #1504 评审修订后**（详见 `2026-11-11-issue-1501-pr1504-review-revision/design-changes.md` §三）：

**单元测试**（三个包）：

| 用例 | 位置 | 断言 |
| --- | --- | --- |
| GetBodyAccessor 截断 | `bfe_http` | 返回 err（`errors.Is(err, io.ErrUnexpectedEOF)`）；**r.Body 非 nil** |
| tokenReadResponseHandler 截断 | `mod_ai_token_auth` | `IsResponseCompleted()==false`；`CompletionTokens` 保持 `COMPLETION_TOKENS_UNKNOWN`（未播种）；`res.Body` 存活 |
| 解码器工厂 nil 守卫 | `mod_body_process` | `NewJsonDecoder`/`NewLineDecoder`/`NewSSEEventDecoder`/`NewContentTypeDecoder`（三种 content-type）nil source 均构造期返回 error，**不 panic** |
| nil body 降级不变量 | `mod_body_process` | 降级后 processor **非 nil**；`res.Body=bp` 后 Read 返回构造错误、Close 安全（评审要求的真实不变量，替代初版"bp == nil"错误契约） |
| 全链定点（评审补充） | `mod_ai_token_auth` → `mod_body_process` | 截断响应依次经过 `tokenReadResponseHandler` → `DoResponseProcess` → Read：不 panic、不置 completed、Read/Close 安全且返回错误 |

**集成测试**（SC29 TC-15，复刻 SC3201-TC035 A-06 形态；评审修订后为 EstimateToken=true/false 双子臂）：
- mock 后端新增响应侧截断能力 `BodyTruncateAt`（非 SSE 模式：声明完整 `Content-Length`、写出 N 字节 flush 后 `panic(http.ErrAbortHandler)`，机制同 `SSEAbortAfter`）；
- 用例：`POST /v1/messages` `stream=false` ⇒ 客户端 200+截断（读错误或短 body）；`PanicClientConnServe` 前后差分 = 0（评审补充的指标断言）；先停 BFE 再读 `exception.log` 断言无 panic（评审修正的空转风险）；RequestLog 存在且 `err_code=BK_READ_RESP_BODY`、`ai_stream=false`；`EstimateToken=true` ⇒ `ai_usage_source=upstream_dropped`、按已转发内容计费（无字节送达 ⇒ 扣 `len(请求体)/4×300`）；`EstimateToken=false` ⇒ `unrecognized` + `ai_zero_cost_reason=upstream_dropped` + 零扣费；无 SSE error 注入事件。

### 步骤 5（集成设计文档）

`测试设计文档/scenario-SC29-.../TC-15-非流式截断响应不panic且按上游断流归因.md` + 场景说明/总体说明更新（沿用 #1474 条款编号引用）。

## 三、影响面与风险

- 改动文件：`bfe_http/response.go`（GetBodyAccessor 赋值顺序）、`bfe_modules/mod_body_process/body_process.go`（nil 防御两处）、`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`（completed/播种精化）、三处包的单测、SC29 集成（mock 能力 + TC-15 + 设计文档）。
- 成功路径零变化：完整非流式 body（#1364）、流式 SSE（#1415/#1474）、压缩流（#1406/#1468）均不经过任何新增分支；SC29 全场景（TC-01~TC-10）+ SC03 回归把关。
- 残余消费语义：`io.ReadAll` 失败时已读前缀丢失（`newBytesBody` 的 `bb` 不返回），保留的原流只剩残余——客户端收到的字节数 < 上游已发前缀。这是 `io.ReadAll` 接口的既有约束（改造它返回部分数据属更大变更，收益有限：截断响应本来就要以错误终止），在防御性单测中固定该语义（残余可读、错误可达）。
- 验证：`go build ./...` + 三包单测 + SC29/SC03/SC20 集成回归；外部 harness SC3201-TC035 A-06 真跑复核。

## 四、验收判据映射（对应 issue）

1. 同形态请求 BFE 日志无 panic、`PanicClientConnServe` 不增——步骤 1+2（panic 源与兜底双保险）。
2. RequestLog 存在：field 3=`BK_READ_RESP_BODY`、field 4 有错误信息、`ai_stream=false`——步骤 1 后连接正常走完 finish handler，既有 `recordCopyError` 归因路径生效。
3. `EstimateToken=true` ⇒ field 764=`upstream_dropped` + `UpstreamDropBilled`+1 + cost/quota 按已转发内容（708=706+707）；`false` ⇒ 764=`unrecognized` + 765=`upstream_dropped` + 零扣费——步骤 3（`responseCompleted=false` 使请求落入 #1474 upstreamDropped 门控；播种精化保证估算基数=已转发内容）。
4. 客户端 200+截断传输错误、不注入 SSE error 事件——步骤 1（残余流转发 + 读错误自然终止）。
5. 流式 partial-drop 与 terminal-marker 不回归——改动均不触碰流式路径，SC29 全场景回归。

## 五、评审修订

PR #1504 评审（xuleiming，CHANGES_REQUESTED）指出初版 `DoResponseProcess` nil 早退"把 panic 平移到下游"、单测固化错误契约等 8 项问题，全部属实；修订方案与落地记录见 `2026-11-11-issue-1501-pr1504-review-revision/design-changes.md`，本文档 §一.1.2、§二.1、§二.2、§四 已同步修订。修订实现中一处与评审建议的字面偏差：`NewBodyProcessor` 采用 **nil source 保留 + `Close` 守卫**而非归一化 `bfe_http.EofReader`——后者会使解码器工厂收到非 nil reader、构造成功、`bp.err` 丢失（错误路径被验证失效），已在修订文档 §一.4 附记。
