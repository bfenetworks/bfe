# BFE Issue #1416 修复方案：AI 路由未命中 404 毒化 keep-alive 连接（复用即 EOF）

- Issue: https://github.com/bfenetworks/bfe/issues/1416
- 缺陷：AI 网关模式下，未命中 AI 路由的请求**第一笔**按设计返回可读的 HTTP `404`（正文 `AI route not found`，`Content-Length: 18`），但**同一 client 的第二笔**请求在复用的池化连接上以 `EOF`/`RemoteDisconnected` 结束——服务端在响应后主动关闭连接却未在响应中宣告（无 `Connection: close` 头），客户端认为连接可复用并将其投入空闲池，后续请求被投放到已死连接。连接死亡为确定性行为（issue 对照实验 13/13：miss key 8/8 + curl 手工 5/5；对照臂 200 连接存活 3/3）；无效 key 同样触发 ⇒ 缺陷落在**通用 no-route 404 响应路径**，与 entity model-prefix 规则无关；BFE 本机回环可复现 ⇒ 排除中间网络设备。
- 代码库：`bfe/`（行号基于当前 HEAD `dc86cce1`，2026-10-13 #1415 提交，逐条核实）
- 关联：#1359（前驱卡片，`da737d1e` 修复 mod_header 对 nil `HttpResponse` 的 panic，2026-09-08 PASSED）。本卡**不是** #1359 的复发：#1359 是首笔请求即 `empty reply`（panic 拆连接），本卡首笔 404 完整可读、失败落在**复用连接的第二笔**——是同产品条款（mod_ai_route §5 + `2026-09-07-internal-resp-nil-httpresponse-fix/design-changes.md` §2 的 A-02「连接与响应回调保持安全可复用」）下的**另一条缺陷**。

## 一、根因

### 1.1 机制链（逐环核实）

1. `ServeHTTPForAI` 的 no-route 分支（`bfe_server/reverseproxy.go:1220-1231`）：

   ```go
   aiResult = basicReq.GetAiRouteResult()
   if aiResult == nil {
       // AI gateway mode: no route hit, return 404
       ...
       res = bfe_basic.CreateSpecifiedContentResp(basicReq, bfe_http.StatusNotFound,
           "text/plain", "AI route not found")
       action = closeAfterReply   // ← 缺陷本体（:1229）
       goto response_got
   }
   ```

2. 响应本身完全满足 keep-alive 复用条件：`CreateSpecifiedContentResp`（`bfe_basic/common.go:87-102`）显式设置 `Content-Length: 18`（`:97`）、不设置 `Connection` 头；响应字节被完整写出。

3. 服务端级 `action = closeAfterReply` 经 `serveRequest` 使 `isKeepAlive = false`：`bfe_server/http_conn.go:630` —— `isKeepAlive = (ret1 == keepAlive) && (ret2 == keepAlive)`，`ret1` 即上述 `action`。

4. `conn.serve` 主循环关闭连接：`bfe_server/http_conn.go:509-514` —— `if !isKeepAlive || w.closeAfterReply { ... break }`，跳出循环即关闭 TCP（FIN）。

5. **毒化的关键在"不宣告"**：决定是否写 `Connection: close` 响应头的是 **HTTP 层** `w.closeAfterReply`（`bfe_server/chunk_writer.go:429-434`），而 no-route 请求是标准的 HTTP/1.1 keep-alive 请求，HTTP 层标志全程为 false（见 1.3 的置位条件逐一不成立）⇒ 响应中**没有** `Connection: close` ⇒ 客户端（Go `http.Transport`、curl、python http.client）按响应自界定（Content-Length）判定连接健康并投入复用池 ⇒ 下一笔请求写到死连接上，读到 FIN → `EOF`。

该形态与 issue 的判别观测精确吻合：主形态是 FIN/EOF 而非 RST ⇒「响应正常结束但连接被显式关闭」，不是「带未读数据的 close-with-unread-data」。

### 1.2 写法来源

`action = closeAfterReply` 从引入 commit `0b4a0536`（"Implement ServeHTTPForAI for AI gateway routing (WIP)"）起就存在，系照搬非 AI 路径 `findCluster` 错误分支的 `// close connection` 惯例（`bfe_server/reverseproxy.go:771-774`，上游 BFE 遗留写法）。**同函数内即有反例/先例**：转发尝试全部失败后构造 500 的分支（`reverseproxy.go:1367-1378`）**没有**设置 `action`，默认 `keepAlive`——即"内部构造响应 + 连接复用"在该函数中早已并存运行，无隐含依赖连接关闭的逻辑。

### 1.3 keep-alive 安全机制已存在（修复可行性依据）

BFE 的响应写出层（fork 自 Go net/http）在**写响应头之前**会按需排空请求体（`bfe_server/chunk_writer.go:351-367`）：

```go
// Per RFC 2616, we should consume the request body before
// replying, if the handler hasn't already done so. ...
if w.req.ContentLength != 0 && !w.closeAfterReply {
    n, _ := io.CopyN(ioutil.Discard, w.req.Body, maxPostHandlerReadBytes+1)
    if n >= maxPostHandlerReadBytes {
        w.requestTooLarge()          // body 过大：放弃复用
        setHeader.connection = "close"
    } else {
        w.req.Body.Close()           // 排空成功：连接可安全复用
    }
}
```

且所有"不能安全复用"的场景都由 **HTTP 层**独立置位 `w.closeAfterReply = true`，从而经 `:429-434` **带宣告地**关闭（客户端不会复用，无毒化）：

| 场景 | 置位点 | 行为 |
| --- | --- | --- |
| HTTP/1.0 且非 keep-alive 请求 | `chunk_writer.go:335-337` | 宣告式关闭 |
| 请求带 `Connection: close` | `chunk_writer.go:339-341` | 宣告式关闭 |
| 全局关闭 keep-alive（`DoKeepAlives()` false） | `chunk_writer.go:343-345` | 宣告式关闭 |
| `Expect: 100-continue` 且 body 未读 | `chunk_writer.go:347-349` | 宣告式关闭 |
| body 超过 256KB（`maxPostHandlerReadBytes`，`:50`）排不空 | `chunk_writer.go:355-363` | 宣告式关闭 |

因此 no-route 404 分支上的**服务端级** `closeAfterReply` 对正确性既无必要（安全性已由 HTTP 层覆盖），又有害（关闭而不宣告 ⇒ 毒化）。标准 AI 网关请求（HTTP/1.1、body ≤256KB）下修复后连接必然被正确归还。

### 1.4 与 #1359 修复的关系

`da737d1e` 后 `CreateSpecifiedContentResp` 已挂接 `request.HttpResponse`（`bfe_basic/common.go:100`），`mod_header` 的 `RSP_HEADER_SET` 响应回调不再 panic——首笔 404 可读即其证明。但该修复只解决了"响应回调安全"（条款 A-01 方向），no-route 分支的服务端级关连接动作从未被审视——本次按条款 A-02「未命中之后的连接……保持安全可复用」补齐。

## 二、修复步骤

设计原则：**连接复用与否是传输层事实，由 HTTP 层按 RFC 2616 语义统一裁决**（排空 body / 按协议版本与请求头处置）；反向代理层只在"响应已无法完整交付"（写失败、模块契约要求关闭）时才用服务端级 `action` 干预。

### 步骤 1（必改，P0 条款）：no-route 404 分支不再关闭连接

文件：`bfe_server/reverseproxy.go:1227-1230`，删除 `action = closeAfterReply`（`action` 为命名返回值，零值即 `keepAlive`，`bfe_server/http_conn.go:49-57`），保留注释说明：

```go
res = bfe_basic.CreateSpecifiedContentResp(basicReq, bfe_http.StatusNotFound,
    "text/plain", "AI route not found")
// issue #1416: keep the connection reusable. The response is self-delimited
// and chunk_writer drains the request body before writing headers, closing
// without advertising "Connection: close" poisoned keep-alive reuse (the
// second request on the pooled connection failed with EOF).
goto response_got
```

首笔 404 的 status / headers / body 字节**完全不变**；仅连接处置从"静默关闭"变为"安全复用"。

### 步骤 2（建议同卡捎带）：AI 路径同形内部响应分支对齐

同形 = 「自界定内部响应 + 服务端级 `closeAfterReply` + 无 `Connection: close` 宣告」，毒化机制与步骤 1 完全相同，且同属 AI 网关产品条款管辖：

| 分支 | 位置 | 响应 |
| --- | --- | --- |
| `ServeHTTPForAI` findProduct 失败 | `reverseproxy.go:1188-1191` | 500 空 body（`CreateInternalSrvErrResp`） |
| `doSingleAIForward` 协议不匹配拒绝 | `reverseproxy.go:1622` | `AiError.CreateErrorResponse`（自界定 JSON） |
| `doSingleAIForward` `HandleAfterAITargetModel` 回调拒绝 | `reverseproxy.go:1669-1670` | 同上 |

处理同步骤 1（删 `closeAfterReply`，下同注）。

### 明确不改（语义契约或非缺陷，改动属越权）

- 各 `BfeHandlerFinish` / `BfeHandlerClose` 分支（如 `reverseproxy.go:1166, 1206, 1247, 1447`）：模块回调显式要求"响应后关闭"，是模块契约；
- `sendResponse` 写失败置 `closeAfterReply`（`reverseproxy.go:1469`）：写路径已损坏，关闭正确；
- `ServeHTTP`（非 AI 路径）的 `findCluster` / `findProduct` / preprocess 400 三处（`reverseproxy.go:771-774, 818-822, 871-876`）：上游 BFE 遗留惯例，不受本 issue 产品条款管辖，若对齐单独立卡。

### 步骤 3（观测配套）

无新增配置项、无开关、无模块顺序调整。no-route 请求在 access log 中本就以 `ErrCode=ErrBkFindLocation` 可圈定，修复后该行记录的请求连接将被复用，无日志字段变化。

## 三、回归测试

### 集成测试（SC01 扩展，`tests/integration/implementation/scenario-SC01-route-table-lookup/`）

该场景已有 #1359 回归设施（mod_header `RSP_HEADER_SET` fixture + no-route 请求），在其上新增**连接复用**断言（SC1203-TC024 对应 api 仓 e2e 已有人工证据，此处固化为自动化门禁）：

1. **复用主用例**：同一 `http.Client`（**禁用自动重连**兜底掩盖——`Transport` 复用单连接，如 `MaxConnsPerHost=1` 或裸 `net.Conn`+`http.ReadResponse`）对 no-route 端点（无效 key 或路由未覆盖前缀）连发两笔 `POST /v1/chat/completions` ⇒ 断言两笔均完整读到 404 + 正文 `AI route not found`，且**第二笔未发生连接重建**（`httptrace` 的 `GotConn` 复用同一 `Conn`）；
2. **对照臂**：命中路由的请求在同一连接上 3 连发全 200 且连接存活（固化 issue 的 3/3 对照）；
3. **形态臂**（证明"该关的关、该留的留"，由 HTTP 层独立裁决）：HTTP/1.0 请求、`Connection: close` 请求、>256KB body 的 no-route 请求 ⇒ 连接被关闭**且响应携带 `Connection: close`**；
4. **回归**：#1359 用例（mod_header 头注入 + no-route 404 + 无 panic）原样通过。

### 部署门禁复跑

api 仓 `SC1203-TC024` 全量复跑：`control-readiness`、`entity-no-route-first`、`entity-no-route-connection-reuse` 三臂全部 PASSED（此前 reuse 臂的间歇 PASS 系 Go Transport 写前探测死连接时自动换连的掩盖，修复后应**稳定** PASS，不再依赖竞态）。

### 验证命令

`cd bfe && go test ./bfe_server/...` 与 `go test ./tests/integration/implementation/scenario-SC01-route-table-lookup/ -v`；提交前 `make test`。

## 四、修复后正确行为（目标链路）

1. 第一笔 no-route 请求：`ServeHTTPForAI` no-route 分支构造 404（自界定）→ `sendResponse` 写出过程中 `chunk_writer` 排空请求 body（≤256KB）→ 404 完整写出，响应**不含** `Connection: close`；
2. `serveRequest`：`ret1 == keepAlive` 且 `FinishReq` 返回 `keepAlive` ⇒ `isKeepAlive = true`（`http_conn.go:630`）；`finishRequest` 关闭 body 并 flush（`bfe_server/response.go:353-369`）；
3. `conn.serve` 循环不退出，连接等待下一笔请求（读超时由 `TimeoutReadClientAgain` 既有时钟接管）；
4. 同一 client 的第二笔（乃至第 N 笔）no-route 请求在**原连接**上复现可读 404——与 200 命中路径的连接处置完全一致，满足条款 A-02「未命中之后的连接与响应回调保持安全可复用」。

## 五、边界与残余风险

- **排空阻塞**：client 宣称 `Content-Length` 但不发全 body 时，`chunk_writer` 排空会阻塞到读超时——与 Go net/http 同语义，且非 AI 路径所有正常 keep-alive 响应都运行同一逻辑（`:355-367`），非本修复新增风险；
- **大 body**（>256KB，如 file/batch 形态上传）：排不空 ⇒ `requestTooLarge()` ⇒ 带 `Connection: close` 宣告式关闭，连接不复用——行为与修复前等价（修复前也是关），但变为有宣告的关闭，客户端不再误复用；
- **`Expect: 100-continue` + body 未读**（`chunk_writer.go:347-349`）：HTTP 层自动宣告式关闭，安全；
- **HTTP/2 前端**：经 `bfe_http2` 模块多路复用，无单连接请求序问题，且 `send_response` 写失败本就对多路复用协议跳过关连接（`reverseproxy.go:1466-1469` 注释），不受影响；
- **改法为纯删除 + 注释**：回滚 = revert 单点；首笔 404 响应字节不变，客户端解析无感知；
- **残余（遗留事项，不在本卡）**：各 `BfeHandlerFinish` 分支与 `sendResponse` 写失败的服务端级 `closeAfterReply` 同样是"关而不宣告"形态（模块契约使然）。若未来要统一，正确方向是让 `action = closeAfterReply` 在框架层显式落 `Connection: close` 响应头（复用 `chunk_writer.go:429-434` 的 HTTP 层机制），而非继续依赖逐分支约定；建议另立卡评估。

## 六、实施记录（2026-10-13）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | ① `ServeHTTPForAI` no-route 404 分支删除 `action = closeAfterReply`（issue #1416 核心修复）；② `ServeHTTPForAI` findProduct 失败 500 分支删除 `action = closeAfterReply`；③ `doSingleAIForward` 协议不匹配拒绝（`BfeHandlerResponse`）`closeAfterReply → keepAlive`；④ `aiClusterInvoke` cluster 查表失败（`ErrBkNoCluster`，caller 转为内部 500）`closeAfterReply → keepAlive`——实施中发现的第 4 处同形分支，符合设计文档步骤 2 判据（AI-only 函数）；⑤ `doSingleAIForward` `HandleAfterAITargetModel` 回调拒绝按返回码区分：`BfeHandlerResponse → keepAlive`（与框架其它 `BfeHandlerResponse` 语义一致），`BfeHandlerFinish` **保持 `closeAfterReply` 不变**（模块契约，对应设计文档"明确不改"） |
| `docs/zh_cn/sys_design/mod_ai_route.md` | §8.1 错误码表后新增「连接语义」段：内部构造响应保持 keep-alive 可复用及 issue #1416 背景 |
| `tests/integration/implementation/scenario-SC01-route-table-lookup/`、`tests/integration/测试设计文档/scenario-SC01-路由表查找与绑定/` | 新增 TC-12 回归测试与设计文档，更新场景说明 §5 与测试场景总体说明 |

实施中对设计文档的修正（均已回写文档）：

- **TC-12 断言口径**：Go `http.Client` 将响应 `Connection` hop-by-hop 头从 `resp.Header` 剥离、语义反映为 `resp.Close`（httptest 实证）；测试断言 `resp.Close` 作为线上宣告的客户端可见等价信号。
- **TC-12 步骤 5（HTTP/1.0）**：Go `http.Transport` 线上恒发 HTTP/1.1（设置 `req.Proto` 无效，裸连实证），改用裸 TCP 直连构造 HTTP/1.0 请求，断言 server 响应后关闭连接（下一读 EOF）。
- **形态臂 256KB 边界**：AI 网关路径下 `extractClientModel` 会经 `GetBodyAccessor` 急切缓冲请求体至 `accessibleBodySize`（SC01 为 4MB，`bfe_http/transfer.go:950-974`），故「排不空」形态臂需 body > 4MB + 256KB（用 5MB），此时 `chunk_writer` 排空间耗尽 → `requestTooLarge()` → 带宣告关闭（且 `http_conn.go:510-512` `closeWriteAndWait` 避免 RST）。

回归测试新增（SC01 扩展，`tests/integration/implementation/scenario-SC01-route-table-lookup/sc01_route_table_lookup_test.go`）：

- `tracedClient`（`httptrace` 记录每次 `GotConn` 的连接身份与 `Reused`）：连接身份相等断言可击穿 Go Transport 对死连接的自动换连掩盖（issue 中间歇 PASS 的来源）。
- `TestTC12_NoRoute404ConnectionReuse` 六步：①② 同一 client 连发两笔 no-route 404，断言第二笔**连接身份相同**且可读（核心回归）；③ 对照臂 3 连发 200 同连接存活；④ 5MB body no-route → 404 + 带宣告关闭（`resp.Close=true`）；⑤ 裸连 HTTP/1.0 → 404 + server 关闭连接；⑥ `Connection: close` 请求头 → 404 + `resp.Close=true`。全部步骤在 mod_header `RSP_HEADER_SET` 启用下执行（#1359×#1416 组合形态）。
- 设计文档：`TC-12-未命中404后连接复用.md` 新增；`场景说明.md` §5 与 `测试场景总体说明.md`（同步补登遗漏的 TC-11）更新。

验证结果（2026-10-13）：

| 范围 | 结果 |
| --- | --- |
| `go build ./bfe_server/`、`go vet ./bfe_server/`、SC01 包 vet | 通过 |
| 全量单元测试（`go list ./... | grep -v tests/integration`，94 包） | 全部通过（exit 0，无 FAIL） |
| SC01 全场景（TC-01~TC-12） | 12/12 通过（含新增 TC-12 与 #1359 回归 TC-11） |
| **变异测试**：`git stash` 仅回退 `reverseproxy.go`（修复前代码）跑 TC-12 | **确定性变红**：`step 2: second no-route request must reuse the first connection (reused=false, sameConn=false)`——即 issue 的换连掩盖形状被连接身份断言精确捕获；`git stash pop` 恢复后转绿 |
| SC02（多 API-Key 8 例）、SC07（限流 Redis）、SC12（客户端中断计费 4 例）、SC25（缓存前置查询） | 全部通过（注：首轮四场景并行跑时 SC02/SC12 各出现一次超时性抖动，串行复跑均稳定通过，与本次改动无关——两场景断言均不涉连接 action） |
| gofmt | 新增/改动文件全部干净；`reverseproxy.go` 仅 HEAD 既有的 doc 注释格式差异（与本次改动无关），未顺带重排 |

未跑：`make test` 全仓（拉起全部 ~30 个集成场景）；已覆盖本次改动影响面（bfe_server 全包单测 + 直接关联的 5 个集成场景）。

遗留事项（代码外）：

- api 仓 `SC1203-TC024` 部署门禁复跑（`entity-no-route-connection-reuse` 臂应**稳定** PASSED，不再依赖 Go Transport 竞态）。
- `BfeHandlerFinish` 拒绝路径（`mod_ai_token_auth/model_check.go:114` allow/block 拒绝、`mod_ai_rate_limit` 429/batch 拒绝）仍按模块契约"关而不宣告"，其根治（框架层统一落 `Connection: close` 或模块改语义）另立卡评估。
- 非 AI 路径 `ServeHTTP` 三处同形分支（`reverseproxy.go` 的 findCluster/findProduct 500、preprocess 400）为上游遗留惯例，不受本 issue 条款管辖，未改动。
