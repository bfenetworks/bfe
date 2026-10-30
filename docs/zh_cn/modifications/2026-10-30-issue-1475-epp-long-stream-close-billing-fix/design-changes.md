# BFE Issue #1475 修复方案：经 EPP 集群的长流式响应连接不关闭且不计费

- Issue: https://github.com/bfenetworks/bfe/issues/1475
- 缺陷：经 EPP 集群的流式请求，输出较长时（实测 ≥1500 token），客户端已收到 `[DONE]`，网关却不关闭连接（客户端等到 90 秒超时），且这次请求**不计费**。输出较短（≤1000 token）正常，同样的请求经 WRR 集群正常，EPP 非流式也正常。测试期间网关日志新增 8 条 "EppClient response body buffer overflow" 告警。
- 代码库：`bfe/`（行号基于 HEAD `f72d0ee5`，2026-10-30，逐条核实）；EPP 消费端行为核实于 `ai-gateway-epp`（HEAD `8417262`，经 demux 交给 llm-d-router v0.11.0 的 `StreamingServer.Process`）
- 关联：`bfe/docs/zh_cn/sys_design/epp_integration.md`（§"EndOfStream 无条件送达"的表述与本缺陷直接矛盾，需同步修正）、issue #1474（上游断流计费，已修复，与本卡互补：#1474 解决"上游死了"，本卡解决"EPP 链路活着但收尾卡死"）。**本卡只改 bfe，ai-gateway-epp 不需要修改。**

## 一、根因

### 1.1 表象层（issue 分析已确认，两处均与现象吻合）

- **假溢出**：`ProcRespBody`（`bfe_util/epp/epp_client.go:277-301`）有两条溢出路径——字节预算（默认 1MB，`:285`）和 **channel 写满**（`select case c.datach <- d: default:`，`:294-300`）。`datach` 容量只有 10（`:210`），而 `copyResponse` 以 32KB 缓冲读 body——转发 goroutine 消费变慢时 **10 个待发送分片（理论上至 320KB，分片更小时阈值更低）就触发"溢出"，1MB 预算形同虚设**。
- **收尾阻塞**：`EppResponseBodyFilter.Close()`（`:365-369`）→ `EppClient.Close()`（`:160-167`）**无超时等待 `<-c.donech`**。转发 goroutine（`:213-270`）在 body 结束后要发 EndOfStream 并做最终 `Recv()`（`:264`），全程无超时；`donech` 永不关闭则 `Close()` 永不返回，`ServeHTTPForAI` 不返回 → `HandleRequestFinish`（计费点）不执行、客户端连接不关闭。
- 溢出后客户端仍收到完整 body（`Read` 只拷贝送 EPP，`:354-363`），与"客户端已收到 `[DONE]`"一致。

### 1.2 根因层：bfe 不按 ext-proc 协议排水，与 EPP 形成流控死锁

表象之上的真正根因是**协议消费方式不匹配**，这是本方案区别于 issue "后续建议"的关键增量：

**EPP 侧（符合协议）**：llm-d 的 `StreamingServer.Process`（llm-d-router v0.11 `pkg/epp/handlers/server.go`）对流式响应**每一个 response-body chunk 都生成并发送一个 `ProcessingResponse`**（`server.go:798-804` 的 per-chunk 发送循环）——这是 envoy ext-proc FULL_DUPLEX_STREAMING 的标准约定（响应顺序 Header→Body→Trailer，`updateStateAndSendIfNeeded`），真实 envoy 逐条 Recv，故 EPP 在 envoy 生态工作正常。EPP 另有专用 reader goroutine 持续 `Recv()`（`server.go:372-384`），但 recvCh 缓冲只有 1——主循环卡住时 reader 最多领先一条就停止消费。

**bfe 侧（未按协议消费）**：响应路径只在两个点 `Recv()`——ResponseHeaders 之后一次、EndOfStream 之后一次（`epp_client.go:223,264`），**中间的 per-chunk 响应从不读取、直接丢弃**。作为对照，请求路径（调度，`bfe_balance/bal_gslb/bal_gslb.go:421-459`）是逐消息 `Send → RecvTimeout` 配对的——只有响应路径破了协议。

**死锁链条**（响应体超过 gRPC HTTP/2 流控窗口后逐级触发）：

1. EPP 的 per-chunk `Send` 把发送窗口填满 → 主循环阻塞在 `srv.Send`（`server.go:801`）；
2. EPP reader goroutine 因 recvCh（cap 1）满而停滞 → 不再 Recv → EPP 接收窗口耗尽；
3. bfe 转发 goroutine 的 `Send(chunk)` 阻塞在 gRPC 流控上 → `datach` 堆积触发展望中的 "buffer overflow" → `aborted=1`，停止向 EPP 喂数据（客户端不受影响）；
4. body 完整送达客户端、`copyResponse` 成功返回、`res.Body.Close()` 关闭 `datach` → goroutine 继续执行 `Send(EndOfStream)`——**仍阻塞在同一条流控死锁上** → `donech` 永不关闭；
5. `EppClient.Close()` 永远等待 → `ServeHTTPForAI` 不返回 → 连接不关闭、`HandleRequestFinish` 不执行 → **不计费**。

**为什么短流正常**：500/1000 token 的整个交换能塞进 gRPC 窗口内，EPP 的全部 per-chunk 响应进入发送队列、被 bfe 的两次 `Recv` 之外的窗口缓冲吸收，EndOfStream 后的最终 `Recv` 能拿到响应、`donech` 正常关闭。窗口大小（默认初始窗口 64KB，BDP 探测上限 4MB，与对端实现/配置相关）决定症状阈值，与"仅 ≥1500 token 触发"的实测一致。

**与 issue "后续建议"的对应关系**：建议 (b)（datach 容量/预算对齐）与 (c)（溢出补救）针对的是症状——溢出是背压死锁的**结果**而非独立缺陷；建议 (a)（超时）是兜底。**根本修复是 (a) 之上的第零层：响应排水**。只做 (b)(c) 不做排水，死锁仍在（只是溢出点推迟）；只做排水不做超时，EPP 真死（非慢）时收尾仍可能挂——所以方案按"排水（根本）+ 超时（兜底）+ 对齐（清理）"三层给出。

## 二、修复方案（仅 bfe）

### 步骤 1（必改，根本修复）：响应路径排水

文件：`bfe/bfe_util/epp/epp_client.go`，`ProcRespHeader`（`:204-271`）。

1. 保持首条消息的 `Send(ResponseHeaders) → Recv()` 配对不变（调用方等待头响应，现有语义：bfe 不应用响应改写，头响应目前也被丢弃，见 §三边界）。**从第二条响应消息起，全部由一个 drainer goroutine 独占消费**：
   ```go
   // 头响应 Recv 成功后启动：
   go func() {
       defer close(c.respDone)
       for {
           _, err := c.stream.Recv() // drainer 独占 stream.Recv
           if err != nil {
               return // io.EOF / 流错误 / ctx cancel 均退出
           }
       }
   }()
   ```
   drainer 丢弃响应与现状语义一致（现状本就丢弃所有响应内容），但保证了 EPP 的 per-chunk `Send` 永不被 bfe 的接收端卡死——EPP 主循环持续运转、reader 持续 Recv，bfe 的 `Send(chunk)` 不再遭遇流控背压。
2. 转发 goroutine 在 `Send(EndOfStream)` 成功后**不再自行 `Recv()`**（`:264` 移除，否则与 drainer 双读竞争），直接返回、`close(donech)`。流对端的完整性由 drainer 保证（EPP 的最后一条响应被 drainer 消费后 Recv 返回 EOF，`respDone` 关闭）。
3. `Close()`（`:160-167`）改为同时等待 `donech` 与 `respDone`：**两个 channel 都关闭**（或任一因错误关闭，drainer 退出即关 `respDone`；转发 goroutine 退出即关 `donech`）才完成收尾，随后 `cancel()`。
4. 排水落地后，`datach` 由 goroutine 即时消费，"channel 写满"路径事实上不再触发（除非 EPP 整体卡死，此时由步骤 2 的兜底接管）。

### 步骤 2（必改，兜底）：收尾超时，保证计费必达

文件：`bfe/bfe_util/epp/epp_client.go`；计数在 `EppState`（`:45-49`）。

1. `Close()` 等待 `donech`/`respDone` 增加 deadline（复用 `callTimeout` 量级，或独立常量如 30s；**不新增配置面**，EPP 收尾正常是毫秒级，30s 仅覆盖极端情况）：超时后 `cancel()` 强制 teardown 并**照常返回**，新增计数 `EppCloseTimeout` + 一条 Warn。关键性质：**`EppResponseBodyFilter.Close()` 一定返回 → `ServeHTTPForAI` 一定走完 → `HandleRequestFinish` 一定执行 → 这次请求一定计费**（这正是本 issue 的资金诉求）。
2. 转发 goroutine 的 `Send(EndOfStream)` 不加独立定时器：gRPC `SendMsg` 的阻塞无法被 timer 打断，能打断它的只有 `c.cancel()`（ctx 取消使阻塞 Send 返回错误），而 cancel 由 `Close()` 的超时分支统一触发——单点兜底、语义等价、避免每分片起 goroutine 的成本。
3. gRPC 拨号可选调大初始流控窗口（`grpc.WithInitialWindowSize`/`WithInitialConnWindowSize`，如 4MB）：进一步降低大流上的 BDP 交互成本。非必须，与排水正交，实施时视改动面决定。

### 步骤 3（建议，清理）：缓冲区对齐 + 文档修正

1. **字节预算成为唯一溢出判据**：`datach` 从定容 channel 改为 `byteQueue`（互斥锁 + 切片 + 单槽唤醒 channel，push 永不阻塞、pop 阻塞等待）。定容 channel 的"写满即溢出"对小于 32KB 的分片是假溢出（10 条 247B 的 SSE 帧 = 2.4KB 即触发，生产 1500 token 的 8 条 overflow 告警正是此形态），任何按容量对齐的方案都依赖分片大小的先验，本质不可靠——只有字节预算能作为判据。`bufPending` 记账逻辑不变。
2. 溢出语义保留不变：真溢出（EPP 卡死且兜底超时前的极端窗口）时仍整流跳过、客户端 body 不受影响、`RespBodyOverflow` 计数保留——此时它重新成为"EPP 消费异常"的真实信号而非噪声。
3. 修正 `bfe/docs/zh_cn/sys_design/epp_integration.md:141` 附近"EndOfStream 无条件送达保证流终结、计量不丢"的表述：EndOfStream 的送达与应答依赖对端按协议消费与响应，bfe 侧已排水 + 超时兜底后该保证成立，并按本方案的实际机制重写该段。

### 不需要修改 ai-gateway-epp

EPP 的 per-chunk 响应是 ext-proc FULL_DUPLEX_STREAMING 协议约定行为，bfe 补齐排水后双端即匹配。唯一相关的已知偏差（**不在本卡范围**）：llm-d 在响应 chunk 里做模型名回写（`rewriteModelName`，`server.go:601`），bfe 丢弃全部响应内容、改写不生效——现状语义保持，如需 bfe 应用 EPP 响应改写属协议完全对齐的独立增强。

## 三、与既有语义的兼容性（安全边界）

- **计费**：本修复只影响"收尾能否到达 `HandleRequestFinish`"。到达后计费逻辑（usage 解析、估算、RMB 扣减）完全复用既有链路；与 #1474 正交（#1474 处理上游断流，本卡处理 EPP 链路收尾卡死）。
- **SC14（EPP 调度对接）**：请求路径（调度）不动；响应路径新增 drainer 后，mock EPP 若不回 per-chunk 响应（现状 stub 行为），drainer 只是多读不到消息、阻塞到流结束——注意 drainer 必须在**流真正关闭**时退出（Recv 返回 EOF/错误），Close 的 deadline 保证其不泄漏。
- **溢出计数基线**：排水落地后 `RespBodyOverflow` 只反映真实背压异常，数值会大幅下降（修复前长流必现 8 条/轮测试），属修正而非丢失。
- **drainer 泄漏防护**：drainer 仅依赖 `stream.Recv` 返回错误退出 + `Close()` 的 deadline 兜底 `cancel()`；`cancel()` 使 Recv 立即返回，drainer 退出。无新增常驻 goroutine（每请求一个，随流生命周期结束）。
- **响应改写保持丢弃**：drainer 延续现状（响应内容不应用），不改变任何对客户端可见的字节。

## 四、回归测试

### 单元测试（`bfe/bfe_util/epp/epp_client_test.go` 扩展）

1. **排水语义**：构造 fake stream server，对每个 response-body chunk 回一个 `ProcessingResponse`（复刻 llm-d 行为）；写入 >窗口量的 body（如 5MB），断言：不触发 `RespBodyOverflow`、body 全部送达、EndOfStream 后 `donech`/`respDone` 均关闭、`Close()` 快速返回。
2. **死锁复现（修复前红、修复后绿）**：fake server 回 per-chunk 响应且不回 EndOfStream 的应答（或延迟应答），断言 `Close()` 在 deadline 内返回并计 `EppCloseTimeout`（修复前该用例超时失败——作为回归锚）。
3. **EPP 不读流的场景**：fake server 完全不 Recv，断言 body 送达、`datach` 在预算保护下溢出（`RespBodyOverflow` 计 1）、`Close()` 仍在 deadline 内返回。
4. 既有用例（`ProcRespBody` 预算、溢出、`RecvTimeout`）全绿。

### 集成测试（SC14 扩展，issue 要求补 1500/3000 token 回归）

SC14 的进程内 mock EPP 是 stub，**不足以触发本 bug**（需要 per-chunk 应答的 llm-d 行为）。补一个 `perChunkEcho` mock EPP 模式：对 ResponseHeaders 回一条响应、对每个 ResponseBody chunk 回一条响应、EndOfStream 后再回一条（复刻 `server.go` 的响应序列）。

5. **TC 长流式回归**：llm-d-inference-sim 后端输出 1500/3000 token 流式，经 EPP 集群 ⇒ 断言 ① 收到 `[DONE]` 后连接在秒级关闭（修复前挂到超时）；② 扣费非 0 且与 usage 一致；③ 日志无 "buffer overflow"；④ 收尾计数 `EppCloseTimeout=0`。
6. **对照臂**：500/1000 token 流式、非流式 1500/3000、WRR 集群流式——行为与修复前一致。
7. 全量回归：SC14（EPP 调度）、SC29（计费快照，含 #1474 新 TC-07~09）、SC03（RMB 扣减）。

验证命令：`cd bfe && go test ./bfe_util/epp/ -v` 与 `go test ./tests/integration/implementation/scenario-SC14-epp-scheduling/ -v`；提交前 `make test`。

## 五、修复后正确行为（目标链路）

EPP 流式 3000 token（修复前挂死不计费的形态）：

1. ResponseHeaders `Send → Recv`（头响应，不变）→ 启动 drainer 独占消费后续响应；
2. body 逐 chunk：`copyResponse` 读 32KB → `ProcRespBody` 进 `datach` → 转发 goroutine `Send(chunk)`——EPP reader 持续 Recv、主循环持续回 per-chunk 响应、drainer 持续丢弃，**全链路无阻塞点**；
3. body 结束：goroutine 发 `EndOfStream` 后返回、`close(donech)`；EPP 回最后一条响应，drainer Recv 到 EOF、`close(respDone)`；
4. `res.Body.Close()` → `Close()` 双 channel 秒级完成 → `cancel()`；
5. `ServeHTTPForAI` 返回 → `HandleRequestFinish` 按 usage 扣费（实测 0.0060042 元）→ 连接正常关闭。日志零 "buffer overflow"。

EPP 进程假死（极端兜底）：`Close()` deadline（30s）到 → `cancel()` → goroutine/drainer 的 Send/Recv 返回错误退出、`Close()` 返回 → 收尾计费照常执行，仅多一条 `EppCloseTimeout` Warn。

## 六、边界与残余风险

- **Close deadline 取值**：正常收尾毫秒级，30s 是纯兜底；若 EPP 侧确实有慢收尾（如流控驱逐的大队列请求），可能在 deadline 后强制打断 EPP 的应答——但应答内容本就丢弃，对客户端与计费均无影响。
- **EPP 完全不读流的集群**：排水修复不了"EPP 不 Recv"（对端进程 hang），此时回到字节预算溢出 + Close deadline 兜底——连接会关闭、会计费，只是 EPP 侧该请求无计量，告警 `RespBodyOverflow` 保留为真实信号。
- **窗口阈值仍是经验值**：症状阈值取决于 gRPC 窗口与分块大小，不同部署可能右移/左移；排水修复与阈值无关，任何长度流都不再触发。
- **无新增配置面、无模块顺序调整、无转发字节变化**；回滚 = revert `epp_client.go` 单文件（+ 文档段）。
- 部署后验收：按 issue 实测矩阵重跑（EPP 流式 500/1000/1500/3000 token），断言 ≥1500 token 的连接在 `[DONE]` 后秒级关闭且扣费与 usage 一致。


## 七、实施记录（2026-10-30）

已按本方案实施，两处偏差均在下方标注。

| 文件 | 改动 |
| --- | --- |
| `bfe_util/epp/epp_client.go` | ① 排水：`ProcRespHeader` 头响应 `Recv` 成功后启动 `drainResponses` goroutine 独占 `stream.Recv` 并丢弃响应；转发 goroutine 发完 EndOfStream 直接返回（移除原最终 `Recv`，消除双读竞争）；头阶段失败的早退路径 `markRespDone`。② 兜底：`Close()` 等 `donech`+`respDone` 双通道，超时（默认 `eppCloseTimeout=30s`，新增 `WithCloseTimeout` option 可调，测试用）后 `cancel()` 照常返回并计 `EppCloseTimeout`。③ `datach` 由定容 channel 改为 `byteQueue`（锁+切片+单槽唤醒 channel），字节预算成为唯一溢出判据。**偏差：原方案"容量 10→32"在 SC14 实测中被证伪——247B 小帧 × 32 条目 = 7.9KB 即假溢出（生产 8 条 overflow 告警的正是此形态），任何容量对齐都依赖分片大小先验，故改为队列化** |
| `bfe_util/epp/epp_client_test.go` | `fakeProcessClient.Recv` 首调返回响应、随后 `io.EOF`（模拟头响应后流定居）；`newTestEppClient`/`TestProcRespBodyBudgetFreedOnConsume` 适配 `byteQueue` |
| `bfe_util/epp/epp_client_drain_test.go`（新增） | 真实 gRPC + fake ext-proc server（复刻 llm-d per-chunk 应答）：`TestProcRespHeaderDrainsPerChunkResponses`（5MB body 全量送达、零溢出、秒级 Close）、`TestCloseTimeoutWhenEppNeverResponds`（EPP 完全不消费 → deadline 返回 + 计数）、`TestCloseTimeoutWhenEppHangsAfterEndOfStream`（EndOfStream 必达 + deadline 兜底）、`TestCloseSchedulingOnlyClient`（无响应路径 client 的 Close 空安全） |
| `tests/integration/common/mock_epp.go` | 新增 `WithPerChunkResponse()`：对每个 response-body chunk（含 EndOfStream）回一个 `ProcessingResponse`，复刻 llm-d `StreamingServer` 的 FULL_DUPLEX_STREAMING 行为——现有 stub 只应答 EndOfStream，触发不了本 bug |
| `tests/integration/implementation/scenario-SC14-epp-scheduling/sc14_long_stream_test.go`（新增） | TC08：mock 后端发 5000 帧 ≈1.1MB SSE（>gRPC 窗口），per-chunk mock EPP ⇒ 断言客户端 25s 内完整收流（修复前挂死超时即失败）、EPP 收到完整 body（交换真正排水）、含生产阈值形态 |
| `docs/zh_cn/sys_design/epp_integration.md` | §4.5 重写：新增"响应排水"与"收尾兜底"两节，修正"EndOfStream 无条件送达保证计量不丢"的错误表述 |

验证结果（2026-10-30）：

| 范围 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 全量单元测试（`go list ./... \| grep -v tests/integration`） | 全部通过 |
| `bfe_util/epp`（含新增 4 项排水/超时用例） | 13/13 通过 |
| `bfe_balance/...`（EPP 调度请求路径） | 全部通过 |
| 集成 SC14 | 8/8 通过：新增 TC08 长流排水回归 + TC01~07 原样通过 |
| SC29（#1474 计费，相邻改动面） | 9/9 通过 |

实施中发现并修正：`byteQueue` 落地前，TC08 以 7.9KB 假溢出失败（32 条 247B 帧填满定容 channel），印证了根因分析中"溢出是背压/启动时序症状"的判断，也把修复从"容量对齐"升级为"字节预算唯一判据"。

部署后验收：按 issue 实测矩阵重跑（EPP 流式 500/1000/1500/3000 token），断言 ≥1500 token 的连接在 `[DONE]` 后秒级关闭且扣费与 usage 一致。
