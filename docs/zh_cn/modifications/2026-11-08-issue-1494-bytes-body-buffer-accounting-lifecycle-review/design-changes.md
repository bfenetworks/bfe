# BFE Issue #1494 修复评审修订：保留期记账的释放归属、并发安全与口径对齐

- Issue: https://github.com/bfenetworks/bfe/issues/1494（系统测试 SC3101-TC006）
- 代码基线：`bfe/`，`v1.9.0-dev`；前序提交 `5603558b`（核心方案已落地）。本文档记录对该提交的**评审修订**，不是重新设计。
- 前序设计：`docs/zh_cn/modifications/2026-11-08-issue-1494-bytes-body-buffer-accounting-lifecycle/design-changes.md`（下称"初版设计稿"）。
- 评审结论：**方案 A（拆开"关闭源"与"释放记账"、请求终态结算）成立，不在本卡推翻。** 根因链四环已逐环复核属实（见 §一）；需修订的是实现层的释放归属与并发安全（§二）、以及两条与实现不一致的口径（§三）。
- 关联：#1493（前一提交）、#1426（body 改写走 `SetBytes`）、#1468（body 读写链）、#1420 系列（AI key 重试）。

## 一、根因链复核（本轮重新逐环核对，结论：属实）

| 环节 | 位置 | 复核结论 |
| --- | --- | --- |
| 加账点在读满 body 之后 | `bfe_http/transfer.go:1011-1018` | 属实，限速上传下加账发生在上传结束 |
| 进入 attempts 循环前包装 | `bfe_server/reverseproxy.go:1497`（cluster fallback）、`:2222`（key-level retry） | 属实；且更早——`http_conn.go:554` `extractClientModel` 对**所有** AI 请求无条件包装 |
| 写完主 Target 即减账 | `bfe_http/transfer.go:205-229` → 初版 `Close()` | 属实，减账与关源耦合在同一处 |
| 重放不加账 | 初版 `SetBytes()` 的 `if !b.released` 分支 | 属实，`released=true` 后完全不触碰 gauge |

**少报窗口的精确界定**：`Close()` 不减账之外，`b.buf` 并未释放，仍被 `req.Body`（`bytes_body`）引用到请求结束。因此真实常驻但未计账的区间是

```
[body 写完上游（Close）  →  请求结束（响应写完 / SSE 流结束 / 最后一次 attempt 收尾）]
```

即该指标实际度量的是"此刻正在上传/写上游的 body 总量"，而非"常驻 buffer 内存"。上游响应慢、fallback 链长、SSE 长流的场景，绝大部分常驻时间落在不计账区间内——这也是现场"24 秒全程为零、只有 34.9ms spike"的完整解释。

## 二、必须修订的三项（正确性 / 健壮性）

### 2.1 释放归属：`defer` 捕获的是注册瞬间的 body 指针

`reverseproxy.go:1508` 的 `defer bfe_http.ReleaseBodyBuffer(basicReq.HttpRequest.Body)` —— Go 的 defer 形参在**语句执行时**求值。此后只要 `req.Body` 被替换，被 retain 的那个 buffer 就永远不会 `Release()`；而 `retained=true` 使它的 `Close()` 也永不减账，于是 gauge 永久虚高且不可 drain。

今天不触发，是因为两条既有事实恰好成立：

1. `extractClientModel`（`http_conn.go:554` → `bfe_basic/condition/primitive.go:1180` → `request.go:981-1007`）在进 reverseproxy 前已把 body 包成 `bytes_body`；`bytes_body` 自身实现 `BodyAccessor`，`GetBodyAccessor` 不会二次包装；
2. 唯一会替换 body 的模块 `mod_body_process`（`body_process.go:320`）注册在 `HandleAfterLocation`（`mod_body_process.go:257`），在 `reverseproxy.go:1441` 执行，早于 defer 注册点。

但 `prepareRequestBodyForRetry` 内部的 `req.GetBodyAccessor()`（`reverseproxy.go:2626`）本身**具备替换 body 的能力**，而 `aiClusterInvoke:2222` 的第二次 prepare 就在 defer 之后。记账正确性因此依赖一条没有任何代码强制的不变量。

**修订**：由 `prepareRequestBodyForRetry` 把 retain 的对象记录到请求上（`bfe_basic.Request` 的 context），`ServeHTTPForAI` 的 defer 释放"记录的对象"；保留按指针读的路径作为兜底。同时 `Release()` 复位 `retained`，让 `Close()` 成为第二道兜底（见 §四.2）。

### 2.2 并发安全：`Release()` 与 `Close()` 分属不同 goroutine

- `bytes_body.Close()` 跑在 **persistConn.writeLoop goroutine**（`bfe_http/transport.go:691-692` 起 goroutine，`:969` writeLoop → `Request.write` → `transferWriter.WriteBody` → `:205-229` `BodyCloser.Close()`）；
- `Release()` 跑在连接请求 goroutine（`ServeHTTPForAI` 的 defer）。

初版设计稿"原则 4：恰好一次（`released` 标志保证）"只在单 goroutine 下成立。`retained`/`released`/`srcClosed` 均是无同步的裸 bool，且 `Release()` 还会 `src.Close()`。上游早回响应（body 尚未写完）时存在并发窗口：重复减账（gauge 推负）、并发关闭客户端 body。

既存的 `released` 本来就有跨 goroutine 读写（`SetBytes` vs `Close`），属于老隐患；本卡把记账正确性押在它上面，因此必须补齐。

**修订**：`bytes_body` 的状态位与 `buf` 长度读取收进一把 mutex（对象是 per-request 的，无竞争热点）；集成用例补跑一次 `-race`。

### 2.3 释放点：函数返回 ≠ 文档定义的"最后一次 attempt 结束"

初版实现把释放挂在 `ServeHTTPForAI` 返回处，而写进 `bfe.conf.md` 的定义是"最后一次 attempt 结束"。attempts 循环在 `reverseproxy.go:1536` 就 break，但 SSE 响应要流式写到 `:1667` 才结束——这期间 buffer 仍在 gauge 里、仍在限额判断里，却**不可能再 Rewind**。

gauge 作为"常驻内存"是准的（buffer 确实还被 `req.Body` 引用），但作为 fallback 配额闸门是误伤：SSE 长流会把别人的重试额度长期压掉。

**修订**：在 attempts 循环收敛处（`reverseproxy.go:1568` 附近）显式 `Release`，`defer` 保留作兜底（两者幂等）。该位置覆盖所有已 retain 的出口：更早的 `goto response_got`（`:1371/:1416/:1434/:1461`）都在 prepare 之前，不可能 retain；循环之后的 `goto response_got`（`:1598/:1606`）在源码顺序上都晚于该点。

## 三、口径修订（文档 ↔ 实现对齐）

### 3.1 `TotalBodyBufferSize` 的真实语义不是"内存上限"

`bfe.conf.md:37` 与 `mod_ai_route_bfe_changes.md:530/:738` 都写"达到上限后不再对请求体做缓存封装"。但在 AI 路径上，body 早在 `extractClientModel` 就被 `io.ReadAll` 成 `bytes_body`，那时**没有任何限额检查**；`prepareRequestBodyForRetry` 的 `TotalBytesBodyBuffer() >= limit` 只决定"还给不给 fallback/key 重试配额"。

因此限额能约束的是**重试配额**，不是缓冲本身；并发大 body 请求仍会先进入内存。初版设计稿 §五"上限语义不变：'达到上限后不再包装'仍由 prepare 闸门执行"与它自己的"实施期修订 2"互相矛盾。

**修订**：三处文档统一改为"达到上限后不再授予 fallback/key 重试配额（不再包装为可重放 body）"，并明确"该上限不阻止 AI 请求体的首次缓冲"。

### 3.2 单请求 body ≥ limit 会禁用自己的 fallback

由于 early wrap 计入当前请求自身字节，`TotalBytesBodyBuffer() >= limit` 在空载进程里也会对"body ≥ limit"的单个请求成立。这是 limit>0 部署下的既成语义，需在 `bfe.conf.md` 明写，避免被当作缺陷上报。（初版设计稿仅作为"臂 B 参数约束"记录，未进入配置文档。）

### 3.3 其他文字与引用修正

- `bfe.conf.md` 新增文案"不再可能被 fallback/key 重放重放"重复字；
- 初版设计稿 §二 的行号在实施后漂移（`Rewind` 现为 `:992-1001`、`SetBytes` `:965-986`），需改为"以当前工作树为准"的实际位置；
- 初版设计稿 §三.3 的"随 `aiClusterInvoke` 返回到外层后不再发生 Rewind"表述有误（外层 fallback 循环正会 rewind）。结论（不在 `aiClusterInvoke` 内加 defer）不变，理由应改为"外层仍需保留到 fallback 决策"。

## 四、实现细节修订

### 4.1 删除 `SetBytes` 的重激活分支

初版为"防异常时序"新增了 `released == true` 时 `add(new)` 并复位 `released` 的分支。两条理由使其不应保留：

1. **非保留路径会造成永久虚高**：body 被 `Close` 减账后再 `Rewind`（`all=true` 时允许），旧行为是静默不计数，新行为加回去但没有任何配对释放（`mod_ai_batch` 的 `defer rb.Rewind()` 即此类）；
2. **保留路径下不可达**：§4.2 让 `Release()` 在清 `released` 的同时清 `retained`，两者又在同一把锁内写，`retained && released` 不可能同时成立。

**修订**：直接删除该分支（而非收紧为 `else if b.retained` 留一段死代码），`SetBytes` 回到"仅在未减账时做净额调整"的语义。

### 4.2 `Release()` 复位 `retained`

`Release` 后 `retained` 保持 true，使后续 `Close()` 永不减账，等于放弃了兜底。**修订**：`Release()` 内 `retained = false`，使 `Close()` 成为合法的第二道结算点（幂等）。

### 4.3 状态位加锁与读侧原子化

`bytes_body` 增加 `sync.Mutex`，`Read`/`Close`/`RetainForRetry`/`Release`/`SetBytes`/`Rewind`/`Peek`/`GetBytes`/`SrcStarted` 全部在锁内读写状态；**锁不覆盖阻塞的源读取**（`Read` 只锁住 `r`/`err` 的快照，真正的 `Read` 在锁外执行），否则 `Release` 关闭源以解除阻塞的语义会被锁住。

`bodyReader.srcStarted` 改为 `atomic.Bool`：它由写入 goroutine 在 `Read` 中置位、由请求 goroutine 在 `Rewind` 中读取，无法用 `bytes_body.mu` 覆盖。

## 五、测试补充

单测（`bfe_http`、`bfe_server`，沿用既有 `resetTotalBytesBodyBuffer` 与 fallback 测试基建）：

| 用例 | 断言 |
| --- | --- |
| `TestRetainedBufferReleasedWhenBodyReplaced`（bfe_server） | prepare 成功 retain 后把 `req.Body` 换成别的包装：终态释放仍结算被 retain 的对象，gauge 归零；重复释放为 no-op（钉住 §2.1） |
| `TestBytesBodyConcurrentCloseRelease`（bfe_http） | `Close()` 与 `Release()` 并发调用（16×2 goroutine）下 gauge 只减一次、不为负（钉住 §2.2） |
| `TestBytesBodyRewindAfterReleaseUnretainedNoop`（bfe_http，替换初版的 `...ReactivatesAfterRelease`） | 非保留路径 `Close()` 后 `SetBytes`：gauge 保持不变（钉住 §4.1） |

释放点（§2.3）没有单独单测：它是 `ServeHTTPForAI` 的语句位置，单测只能复刻调用序列而测不到位置本身。改由两处覆盖——`releaseRetainedBodyBuffers` 的幂等断言（重复调用不二次减账），以及集成 TC13 臂 A 的 drain 断言（请求结束后回落）。

回归：`go test ./bfe_http ./bfe_server`；`-race` 需在 Linux/CI 上跑（本机 `CGO_ENABLED=0` 且无 gcc，`go test -race` 直接报错，未能本地执行）；集成 SC01 全量（TC13 两臂 + TC08/09/10 对照）、SC05/SC08/SC28/SC12/SC03/SC29/SC27。

## 六、影响面与兼容性

会变的行为：

- `limit = 0`（默认，含现场）：仅监控口径变化——`total_bytes_body_buffer` 从"正在上传/写上游的字节"变为"常驻的可重放 buffer"，稳态读数显著抬高，属修正；
- `limit > 0`：闸门重新生效，`fallback 被禁用` 的日志会增多（保留中的 buffer 真正占用额度，且请求自身 body 也计入）；发布说明须提示运维同时观察该指标与禁用日志；
- 释放点前移到循环收敛：SSE 长流不再占用重试额度；`total_bytes_body_buffer` 的回落时间点早于请求结束。

不改的行为：fallback 触发条件与状态码白名单、batch 直通禁用重试、`AccessibleBodySize` 截断、响应侧 `bytes_body` 记账（`response.go:310`）、`TokenUsage`/计费/日志口径。

## 七、实施记录

| # | 项 | 状态 |
| --- | --- | --- |
| 1 | `bfe_http/transfer.go`：`bytes_body` 状态位加 `sync.Mutex`；`bodyReader.srcStarted` 原子化；`Release` 复位 `retained`；删除 `SetBytes` 重激活分支（§四） | 已实施 |
| 2 | 释放归属：`prepareRequestBodyForRetry` 改为返回被保留对象；`bfe_server` 侧新增 `retainedBodyBuffers` + `retainBodyBufferForReplay` / `releaseRetainedBodyBuffers`（挂在 `bfe_basic.Request.Context`），无需 bfe_http 新增导出 | 已实施 |
| 3 | `bfe_server/reverseproxy.go`：defer 提前到包装之前；两处 prepare 调用点记录被保留对象；attempts 循环收敛处显式释放，defer 作兜底 | 已实施 |
| 4 | 单测：`bfe_http` 3 例（含并发）、`bfe_server` 3 例（含 body 被替换 / 重复释放） | 已实施（`-race` 待 CI） |
| 5 | 文档口径：`bfe.conf.md` `Server.TotalBodyBufferSize`、`mod_ai_route_bfe_changes.md` §4.3.5/§8.3（§三） | 已实施 |
| 6 | 初版设计稿：补勘误段（指向本文档），修正"重放重放"重复字 | 已实施 |
| 7 | 集成回归（SC01 全量 + 6 个对照场景） | 见 §八 |

**实施结果**

| 范围 | 结果 |
| --- | --- |
| `go build ./...`、`go vet ./bfe_http/ ./bfe_server/` | 通过 |
| `go test ./bfe_http/`（9 例 bytes_body/记账相关，含并发用例） | 通过 |
| `go test ./bfe_server/`（3 例 body buffer 相关） | 通过 |
| 集成 SC01 全量 + SC02/03/05/08/12/27/28/29 回归 | 全部通过（详见 §八） |
| `go test -race` | 未执行：本机 `CGO_ENABLED=0` 且无 gcc，`-race` 要求 cgo；须在 Linux/CI 补跑 |

## 八、集成回归

2026-11-08 本机实跑（真实 `bfe` 进程 + mock 后端）：

| 场景 | 结果 |
| --- | --- |
| SC01 全量（含 TC13 两臂、TC08/09/10 对照） | 通过（14.6s） |
| SC13 TC13 单例（`TestTC13_RetainedBodyBufferGaugeLifecycle`） | 通过（8.3s，臂 A 6.85s / 臂 B 1.47s） |
| SC02 多 API Key（key 级重试直接相关） | 通过（54.0s） |
| SC03 RMB 计费 | 通过（74.0s） |
| SC05 访问日志 AI 字段 | 通过（59.7s） |
| SC08 fallback 重算 | 通过（6.2s） |
| SC12 客户端中断计费 | 通过（24.6s） |
| SC27 上游错误归一 | 通过（40.5s） |
| SC28 batch 直通 | 通过（57.7s） |
| SC29 usage 快照计费 | 通过（32.8s） |

TC13 臂 A 的在途 gauge 可见性与 drain 断言在释放点前移后仍通过，说明"循环收敛即释放"不影响判据 1/4。
