# BFE Issue #1501 后续修复方案：非流式截断响应的客户端异常终止信号（#1504 部署后复验）

- 来源：[issue #1501](https://github.com/bfenetworks/bfe/issues/1501) 评论「#1504 部署后复验结果」（MercuryLi，2026-11-12，经 GitHub API 核实原文）。
- 复验结论：#1504 已修复原缺陷——非流式 200 body 中途截断不再 panic，RequestLog 与计费归因恢复（`BK_READ_RESP_BODY` / `upstream_dropped` / cost 为正）。
- **剩余缺陷**：同一形态下客户端收到 **HTTP 200 + 空 body + 干净 EOF**，得不到截断传输错误。修复前客户端能看到 `unexpected EOF`，那是 panic 时代异常连接中断附带提供的；panic 修掉后连接收尾变为优雅 FIN，截断信号随之消失。**违反 `2026-10-30-issue-1474-upstream-midstream-drop-fix/design-changes.md`「非流式边界」③**（客户端必须得到截断传输错误）。
- 代码库：`bfe/`（行号基于 HEAD `e2e0ef45` 核实；行为链各 hop 见 §一）。
- 关联：#1474（上游中途断流归因）、#1478（客户端中断计费）、#1501（非流式截断 nil panic，PR #1504 已修）、`2026-11-11-issue-1501-nonstream-truncated-body-nil-panic-fix`、`2026-11-11-issue-1501-pr1504-review-revision`。

## 一、根因（复验评论根因定位的逐 hop 源码核实）

客户端信号丢失链，四跳全部实锤：

1. **截断时已读前缀被丢弃**（`bfe_http/transfer.go:1005-1009` `newBytesBody`）：`io.ReadAll` 失败返回 `(nil, err)`，已读前缀随错误丢弃——#1504 修复方案固定的既有语义（残余流 0 字节是 QA 环境客户端 `bytes=0` 的直接原因）。
2. **拷贝错误只"结束处理"**（`bfe_server/reverseproxy.go:1692-1699`）：`sendResponse` 返回 `responseCopyError`（readSide、clientClosed=false，经 `isUpstreamDropErr` 判定为上游断流）后，非 multiplex 协议仅置 `action = closeAfterReply` 并 `recordCopyError`（`ErrCode=BK_READ_RESP_BODY`、`MarkUpstreamDropped()`，:1201-1237）——**没有任何使客户端连接异常终止的动作**。
3. **连接收尾是优雅 FIN**（`bfe_server/http_conn.go:516-521` → `defer c.close()` → :221-224 `close()`：`finalFlush()` + `rwc.Close()`）：`closeAfterReply` 走 `finishRequest()`（`bfe_server/response.go:353-372`，flush + `cw.close()` 补终止 chunk）后裸 `Close()`，发送干净 FIN。`closeWriteAndWait()`（:241-247）只在 `requestBodyLimitHit` 时调用，本路径不触发。
4. **FIN 无法表达截断**：BFE 转发给客户端的响应分帧形态下（`chunk_writer` 已按上游 `Content-Length` 或 chunked 写出头），干净 EOF 与完整响应不可区分——QA 实测客户端 HTTP 栈报"干净 EOF"而非读错误。

一句话：**#1504 消掉了 panic，也消掉了 panic 时代异常断开附带的截断信号；拷贝错误路径缺少"响应未完成时显式异常终止连接"这一步。**

## 二、修复设计（采纳 QA 建议方向并落地细化）

### 步骤 1（核心，必做）：非流式上游断流的客户端连接异常终止

文件：`bfe_server/reverseproxy.go`，`ServeHTTPForAI` 的 `send_response` 错误路径（:1692-1699）。

判定谓词（新增小函数，便于单测）：

```go
// shouldAbortTruncatedNonStream reports whether a failed response copy must
// abort the client connection (issue #1501 follow-up): the upstream broke a
// non-streaming response mid-transfer while the client is still attached.
// A clean FIN cannot signal truncation; the client must see a transport error.
func shouldAbortTruncatedNonStream(res *bfe_http.Response, err error, proto string) bool {
	return res != nil && !res.IsSse && isUpstreamDropErr(err) &&
		!CheckSupportMultiplex(proto)
}
```

错误路径改造：

```go
	if err != nil {
		if !CheckSupportMultiplex(basicReq.Session.Proto) {
			action = closeAfterReply
			if shouldAbortTruncatedNonStream(res, err, basicReq.Session.Proto) {
				// issue #1501 follow-up: the non-streaming response is
				// incomplete; close the connection abnormally so the client
				// gets a transport error, not a clean FIN.
				action = closeDirectly
				setRSTOnClose(basicReq.Connection) // 见步骤 2
			}
		}
		p.recordCopyError(basicReq, res, aiMeta, err)
	}
```

两层机制，覆盖全部响应分帧形态：

1. **`action = closeDirectly`**（既有机制，`http_conn.go:609-613` → `prepareForCloseConn`）：跳过 `finishRequest()` 的 flush、**终止 chunk 与 trailer 写出**——chunked 响应缺 0-chunk、定长响应缺承诺字节，客户端 HTTP 栈按帧校验稳定得到 `unexpected EOF`。
2. **RST 兜底（步骤 2）**：对 close-delimited 形态（既无 `Content-Length` 也无 chunked——此时 FIN 本身就是边界，closeDirectly 与优雅收尾无法区分），在连接关闭前对底层 TCP 连接 `SetLinger(0)`，使 `Close()` 发送 RST，客户端 recv 返回 `ECONNRESET`。TLS 场景同路径生效（无 close_notify + RST，客户端报 `tls: unexpected EOF`/`connection reset by peer`）。

**守卫（明确不做）**：
- 仅 `isUpstreamDropErr`（readSide && !clientClosed）：客户端写失败/客户端中断后果保持既有 `closeAfterReply` 语义（issue #1352/#1473 不动）。
- 仅非流式（`!res.IsSse`）：SSE 流式维持 #1474 的注入协议错误事件路径（QA 意见：流式行为保持现状）。
- 仅 HTTP/1.x（`CheckSupportMultiplex` 排除 h2/spdy）：multiplex 下连接级 RST 会误伤同连接其他流；h2 的截断信号需 stream error（RST_STREAM），另行立项。
- 非 200 状态码不特殊化：任何状态的非流式上游截断直通形态同样适用（错误体被截断同样应报传输错误）。

### 步骤 2（必做）：RST 兜底实现

文件：`bfe_server/reverseproxy.go`（或 `bfe_server/http_conn.go` 就近放 `conn` 辅助方法）。

```go
// setRSTOnClose arms an abortive TCP close (SO_LINGER 0) so the final Close
// sends RST instead of FIN. Only *net.TCPConn is supported; other transports
// (h2/spdy, unix socket, TLS-wrapped handled at TCP layer by caller) keep
// the graceful close. Best-effort: linger failure degrades to closeDirectly
// alone, which still covers Content-Length/chunked framings.
func setRSTOnClose(conn net.Conn) {
	type lingerSetter interface{ SetLinger(sec int) error }
	if ls, ok := conn.(lingerSetter); ok {
		if err := ls.SetLinger(0); err != nil {
			log.Logger.Warn("set SO_LINGER(0) for truncated non-stream response failed: %v", err)
		}
	}
}
```

要点：
- 类型断言而非具体 `*net.TCPConn`，TLS 场景在调用点 unwrap 后同样适用（QA 环境为明文 HTTP，`http://172.19.1.222:8080`，`basicReq.Connection` 即 `*net.TCPConn`）；不支持的传输静默降级为仅 `closeDirectly`（chunked/定长形态已覆盖，close-delimited 形态记 Warn 可观测）。
- linger 0 丢弃的是**内核中尚未发送**的数据；已 flush 到客户端的头与已转发前缀不受影响。为降低 RST 与在途数据竞争，保留 `close()` 的 `finalFlush()` 现有顺序（先 flush 后 Close）。
- 触发面极小：仅"非流式 + 上游断流 + 客户端仍在"的错误路径，正常响应、keep-alive 连接均不经过。

### 步骤 3（必做）：单元测试

`bfe_server/reverseproxy_ai_test.go`（或就近 `_test.go`）：

| 用例 | fixture | 期望 |
| --- | --- | --- |
| 谓词表驱动 | `shouldAbortTruncatedNonStream`：非流式+readSide+http/1.1 / SSE+readSide / clientClosed / writeSide / h2 proto / res=nil / 非 2xx 状态 | 仅"非流式+上游断流+非 multiplex"为 true |
| 错误路径动作 | `ServeHTTPForAI` 拷贝错误注入（mock transport/body），非流式 200 截断 | 返回 `closeDirectly`；`recordCopyError` 照旧（`BK_READ_RESP_BODY`）；`setRSTOnClose` 对 `*net.TCPConn` 幂等可重复调用 |
| 客户端中断不误伤 | clientClosed 形态拷贝错误 | 仍为 `closeAfterReply`，不碰 linger |

### 步骤 4（必做）：集成测试收紧 + 新增臂

SC29 `TestTC15_NonStreamTruncatedBodyNoPanic`（`sc29_usage_snapshot_billing_test.go:826`）现有客户端断言偏弱：`readErr == nil && len(respBody) >= len(body)` 即通过——Go 客户端按 `Content-Length` 校验时报错，但 close-delimited 形态下干净 EOF 可通过（QA 实测形态）。**收紧为**：`readErr != nil`（必须为传输读错误，不得是干净 EOF），两条臂（EstimateToken=true/false）同标准；RequestLog / 计费 / panic 断言不变。

新增臂（同 TC 或 TC-16）：
- **close-delimited 形态**（HTTP/1.0 请求或 `Connection: close` 协商，无 `Content-Length`、无 chunked）：断言客户端 recv 返回错误（RST）而非干净 EOF——钉住步骤 2 的 RST 兜底。
- **回归**：SSE 流式 partial-drop 臂（SC29 TC11 等）不回归——SSE 错误路径仍走 `closeAfterReply`/注入事件，不触发 abort 谓词。

### 步骤 5（部署复验，QA）

修复构建部署后重跑 SC3201-TC035 最小复现，直读：① 客户端 `read_error` 非 nil（截断传输错误）；② RequestLog `BK_READ_RESP_BODY` 行保持、`ai_stream=false`；③ EstimateToken=true 时 field 764=`upstream_dropped`、cost 为正，false 时 764=`unrecognized` + 765=`upstream_dropped`、cost=0；④ A-01～A-05（流式 partial-drop / terminal-marker）全通过；⑤ `PanicClientConnServe` delta=0。

## 三、影响面与风险

- 改动文件：`bfe_server/reverseproxy.go`（错误路径一处动作选择 + 两个纯函数）、`bfe_server/reverseproxy_client_close_test.go`（单测）、`sc29_usage_snapshot_billing_test.go`（TC15 收紧 + 新增 TC16 裸 TCP RST 臂）、本设计文档。
- 行为变化：仅"非流式 + 上游断流 + 客户端仍在 + HTTP/1.x"的错误收尾——从"优雅 FIN"变为"异常终止（缺终止帧，close-delimited 时 RST）"。这正是「非流式边界」③要求的语义；其余拷贝错误形态（客户端中断、写失败）动作不变。
- 计费与日志零影响：`recordCopyError` / `FinishReq` / `mod_access_pb3` 路径完全在动作选择之前/之外，RequestLog 照常写出（#1504 已恢复的归因不回归）。
- 与 #1474 的关系：不改动注入协议错误事件的 SSE 路径；不改动 `upstreamDropped` 计费门控；只是把 #1504 修复时丢失的客户端截断信号补回来。
- 残余风险：close-delimited + 非 TCP 传输（unix socket）形态无 RST 兜底，仅 closeDirectly，客户端可能仍见干净 EOF——记 Warn 可观测，生产形态（TCP/TLS）不受影响；h2 非流式截断的 stream error 信号另行立项。

## 四、替代方案与否决记录

| 方案 | 结论 | 理由 |
| --- | --- | --- |
| 只发 RST（SO_LINGER 0），不动 closeDirectly | 否决 | chunked/定长形态下多此一举，且 linger 0 有丢弃内核在途数据、RST 与在途字节竞争的风险；closeDirectly 按 HTTP 帧校验表达截断更标准 |
| 只 closeDirectly，不设 linger | 部分采纳 | 覆盖 chunked/定长形态；close-delimited 形态 FIN 即边界、无法表达截断（QA 实测形态），必须 RST 兜底 |
| 复用 `panic(ErrClientReset)` 路径触发异常断开 | 否决 | panic 走 `conn.serve` recover（`http_conn.go:277-288`）并计 `PanicClientConnServe`——拿监控告警当控制流，污染 panic 指标语义 |
| 依赖上游 `Content-Length` 让客户端自行校验 | 否决 | QA 复验实测该形态下客户端得到干净 EOF；转发分帧形态不受 BFE 控制，不能作为契约 |
| h2/spdy 一并做 RST | 否决（本次范围外） | 连接级 RST 误伤同连接其他流；h2 截断信号应走 RST_STREAM，单独立项 |

## 五、实施记录（2026-11-12）

### 5.1 落地代码（HEAD `e2e0ef45` + 本修复）

- `bfe_server/reverseproxy.go:1696-1708`：`ServeHTTPForAI` `send_response` 错误路径——非 multiplex 时先置 `closeAfterReply`，`shouldAbortTruncatedNonStream` 成立则改 `closeDirectly` 并 `setRSTOnClose(basicReq.Connection)`；`recordCopyError` 调用位置不变（日志/计费归因与本机制解耦）。
- `bfe_server/reverseproxy.go:1750-1764`：`shouldAbortTruncatedNonStream`（非流式 + `isUpstreamDropErr` + 非 multiplex，含 `res` 空值防御）。
- `bfe_server/reverseproxy.go:1766-1776`：`setRSTOnClose`——接口断言 `SetLinger(0)`，仅 `*net.TCPConn` 生效；不支持的传输静默降级（仅 `closeDirectly`， chunked/定长形态已覆盖），失败记 Warn。

### 5.2 落地测试

- 单测（`bfe_server/reverseproxy_client_close_test.go`）：`TestShouldAbortTruncatedNonStream` 表驱动 10 例（http/1.1、http/1.0、非 200 直通、SSE 排除、clientClosed 排除、writeSide 排除、h2/spdy 排除、nil res/err）；`TestSetRSTOnClose`（`*net.TCPConn` 生效 + 幂等 + `net.Pipe` 静默忽略）。
- 集成（SC29）：
  - TC15 两条臂客户端断言收紧为 `readErr != nil`（干净 EOF 即失败），RequestLog / 计费 / panic 断言不变；
  - 新增 `TestTC16_NonStreamTruncatedClientReset`：裸 TCP 请求 + `io.ReadAll`——RST 时返回连接错误、干净 FIN 时返回 nil，直接钉住异常终止；同时断言 200 响应头、`PanicClientConnServe` 不增、RequestLog `BK_READ_RESP_BODY`。
- **与步骤 4 初稿的偏差**：close-delimited 专项臂未单独实现——`chunk_writer` 对 HTTP/1.1 恒分帧（定长或 chunked），BFE→客户端的 close-delimited 形态在测试环境不可构造；且实现中 RST 对该路径无条件兜底，close-delimited 形态由构造保证被覆盖，TC16 的裸 TCP 臂即钉此语义。

### 5.3 验证结果

- `go test ./bfe_server/` 全量通过；`go vet` 通过。
- SC29 全场景通过（25.5s，含 TC07/TC08/TC11 等全部 SSE 上游断流臂——SSE 路径不受影响，注入协议错误事件行为不变）。
- SC29 TC15（两臂，收紧后）+ TC16 通过：客户端收到截断传输错误（RST），RequestLog/计费归因保持。
- SC03（RMB 配额）/ SC05（访问日志字段）/ SC12（客户端中断计费）回归通过（见 git 提交记录）。
- 未改动：非 AI 代理路径（`reverseproxy.go:987` 通用 `send_response`）的错误收尾保持 `closeAfterReply`——普通静态响应截断不在本卡范围；h2/spdy 的 stream error 截断信号另行立项。
