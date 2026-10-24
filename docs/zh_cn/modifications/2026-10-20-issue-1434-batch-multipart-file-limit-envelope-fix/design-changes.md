# 批量单文件限额判定对象修复（multipart 信封被计入 batch 文件 bytes/lines 上限）

日期：2026-10-20

状态：**代码与测试已实现并验证（2026-10-20，见 §九实施记录）**。§二 的行号与结论基于实施前工作区 HEAD `05f05d68` 逐条核实，§三 的运行时数据来自实施前的临时单测探针（已删除）。

- Issue：[bfenetworks/bfe#1434](https://github.com/bfenetworks/bfe/issues/1434)（由 [rainway-ai-gateway/ai-gateway-api#256](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/256) clone）
- 代码库：`bfe/`（数据面，`mod_ai_batch` + `mod_ai_rate_limit`）
- 关联：`2026-10-07-ai-batch-support`（批量一期，§9 限流语义）、`2026-10-19-issue-254-ai-batch-task-state-and-settle-fix`（同链路：文件行/字节统计口径第一次修正）、`docs/zh_cn/sys_design/ai_batch.md` §6/§9
- 关键前提：issue 提交于 2026-10-19，**其根因定位针对的是当时 HEAD**；`32aded27`（2026-10-20，#254 修复）已在 `upstream/v1.8.9-dev` 上引入 `multipartFilePartStats`，把"整包计数"从**绑定统计**与**前置 413** 上部分摘除。本文档描述的是**修完那一步之后仍然残留**的判定面，以及 issue 复现路径在新代码上的当前实际表现。

---

## 一、缺陷摘要

`batch_limits.max_file_bytes` / `max_file_lines` 的产品语义是**单文件**上限（`2026-10-07-ai-batch-support/design-changes.md` §9.1）。但 `/v1/files` 走的是 `multipart/form-data`，数据面在三个判定点上仍把**整个 multipart 请求信封**（boundary 行、part 头、`purpose` 字段、CRLF）当成文件内容来度量，导致远小于上限的合法文件被拒：

- 已知 `Content-Length` 时，逐策略预检与 `mod_ai_batch` 预检都用**信封总长**比较 `max_file_bytes` → `413 BATCH_FILE_TOO_LARGE`；
- 出站流式闸门（`countingBody`）按整条信封累加 bytes/lines → 文件内容未超限也会在转发中途 abort，经 `bfe_server/reverseproxy.go:1376-1381` 变成 `500`（`UPLOAD_ABORT_STREAM`）。

第二个判定点最容易被漏掉：**即使 413 那条路径被修好（现状已经如此），issue 的原始场景仍然失败**，只是错误从 413 变成 500/连接中断。这是本文档要收口的部分。

## 二、判定点全景（当前 HEAD，逐条核实）

| # | 判定点 | 当前行为 | 现状 |
|---|--------|----------|------|
| A | `bfe_modules/mod_ai_rate_limit/batch_check.go:56-62` | `req.HttpRequest.ContentLength > b.MaxFileBytes` → 逐策略拒 `batch_file` | **未修**（multipart 上必然偏大；真实流水线里它先于 B 命中） |
| B | `bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go:96-102` | `outreq.ContentLength > BatchEffMaxFileBytes` → 413 | **未修**（与 A 同型，属冗余复核） |
| C | `bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go:113-136` | 全缓冲时用 `multipartFilePartStats` 取 `file` part 的 bytes/lines 判定 413 | **已修**（#254）；但门槛是 `accessor.GetBytes()` 的 `all=true`，且解析失败即退回整包 |
| D | `bfe_modules/mod_ai_batch/file_counter.go:110-148` | `countingBody` 按**整包信封**累加 `uploadBytes`/`uploadLines`，超限 `errUploadLimitExceeded` | **未修**（`:137` 无条件安装在出站 body 上，缓冲路径也在内） |

信封与文件内容的偏差来源：每个 part 的 `\r\n--<boundary>\r\n` 分隔行 + part 头 `\r\n`、末尾 CRLF、`purpose` 字段各占行；boundary 与文件名越长偏差越大。实测（`filename=batch.jsonl` + `purpose=batch`，与 SC28 `e.upload` 同形）：

| 场景 | 文件内容 | multipart 信封 | 偏差 |
|------|----------|----------------|------|
| issue 手工探针 | 1 行 / 203 B | 11 行 / 509 B | +10 行 / +306 B |
| 本次核实 | 1 行 / 138 B | 11 行 / 502 B | +10 行 / +364 B |
| 本次核实（SC28 `inputJSONL`） | 2 行 / 274 B | 12 行 / 638 B | +10 行 / +364 B |

行数偏差恒定为 part 结构行数（本形态 = +10），因此 `max_file_lines` 只要小于"文件行数 + 10"就会命中 D —— 这正是 issue 里 `max_file_lines=10` 拒掉 1 行文件、而 `=12` 放行（信封 11 行 ≤ 12）的原因，也解释了 issue 对照实验里 `request_count=1 bytes=203` 的"能接受"其实是被信封边界放过的巧合。

### 2.1 运行时证据（本次临时单测探针复现，已删除）

同一 1 行 / 138 B 的 multipart 上传（信封 502 B / 11 行），直接调用 `preCheckUpload`：

```
max_file_bytes=1048576 -> ret=GoOn(1)  status=0    partBytes=138   # C 生效，前置放过
max_file_bytes=400     -> ret=Finish(0) status=413 partBytes=0     # B 命中：信封 502 > 400
后续读取 outreq.Body   -> 502/502 字节 + err=upload exceeds effective file limits,
                          aborted=true, countedLines=11, countedBytes=502   # D 命中
```

即：**C 修好之后，`max_file_lines=10` 的原始场景仍会被 D 打断**；`max_file_bytes` 较小（例如按小额度配额）时 B 仍会误判 413。

## 三、根因

1. **判定的度量对象错位**：A/B/D 三处都把"请求信封"当作"单文件"。契约来源是 §9.1 的 `max_file_*` 定义 + §9.2"前置 413 与 chunked 中断读同一组有效上限"，即**同一份计数**必须是文件内容。
2. **C 的修复只覆盖了一半路径**：`mod_ai_batch_handlers.go:113-136` 只在 `all=true`（整包已在 `GetAccessibleBodySize()` 内缓冲，默认 2 MB）时给权威计数，且**放行之后仍无条件安装 D**（`:137`），所以"前置放行 + 流式中断"这对组合必然自相矛盾。
3. **流式路径没有 multipart 感知**：`file_counter.go` 的注释已经承认"this counter sees the WHOLE multipart envelope"，把它当成"粗粒度护栏"；但护栏用的上限是**文件上限**，两者口径不一致，合法上传必被误伤。`sys_design/ai_batch.md` §9 也把 chunked 近似计数记为已知限制——那条限制只该影响**记账精度**（`lines_src=approx`），不该影响**限额判定**。
4. **逐策略预检先行**：A 在 `mod_ai_rate_limit`（先注册）里用 `req.HttpRequest.ContentLength` 判字节，服务端视角下 `ContentLength` 永远是信封长度，因此 B 的修复单独存在也无法让线上行为正确。

## 四、修复方案

### 4.1 设计原则

1. **限额的度量对象只有一个**：multipart 请求取 `name="file"` part 的 payload；非 multipart（raw JSONL / octet-stream）上传，信封即文件内容，口径不变。前置 413、流式中断、`BATCH_FILE` 绑定三处**共用同一份计数结果**。
2. **能精确定位就不装闸门**：一旦 `file` part 的 bytes/lines 已知（无论整包是否缓冲完），判定即为权威；不再安装整包计数闸门，避免"前置放行、后面又被拦"。
3. **无法定位才退化**：只有 `file` part 未完整出现在缓冲窗口内（> `GetAccessibleBodySize()`）或 multipart 解析失败时，才走流式增量计数；解析失败时保留**严格口径**（宁可误拒、不可绕过），并以 `UPLOAD_COUNT_APPROX` + `lines_src=approx` 暴露。
4. **不改透传语义**：请求体字节、`Content-Type`、`Content-Length` 一律原样转发（与 `2026-10-20-issue-1426` 的透传约束一致），闸门只旁路计数、不改写。

### 4.2 步骤 1（必改）：共享的 multipart 判定 helper

`mod_ai_rate_limit` 与 `mod_ai_batch` 都要用"这是不是 multipart 上传"，且两个模块都已 import `bfe_basic`（`batch_op.go` 已承载 `ClassifyBatchOp` 这类请求分类原语）。在 `bfe_basic` 新增：

```go
// bfe_basic/batch_op.go（或同包新文件 batch_file.go）

// IsMultipartUpload reports whether the request body is a multipart/form-data
// envelope and returns its boundary. Batch file limits are defined on the
// `file` part content; the envelope (boundaries, part headers, other fields)
// must never be measured against max_file_bytes/max_file_lines.
func IsMultipartUpload(header bfe_http.Header) (boundary string, ok bool) {
    ct := header.Get("Content-Type")
    if ct == "" {
        return "", false
    }
    mediatype, params, err := mime.ParseMediaType(ct)
    if err != nil {
        return "", false
    }
    if mediatype != "multipart/form-data" && mediatype != "multipart/mixed" {
        return "", false
    }
    boundary = params["boundary"]
    return boundary, boundary != ""
}
```

`mod_ai_batch/multipartFilePartStats`（`file_counter.go:47`）改为接收 `boundary string` 而不是自己再解析一次 Content-Type，保持单一解析点。

### 4.3 步骤 2（必改）：逐策略预检跳过 multipart 信封

文件：`bfe_modules/mod_ai_rate_limit/batch_check.go`（`checkBatchLimits`，`:56-62`）。

```go
case bfe_basic.BatchOpUpload:
    // multipart: Content-Length is the whole envelope (boundaries + part
    // headers + form fields), never the file size — the check moves to
    // mod_ai_batch, which measures the `file` part (issue #1434).
    if _, multipart := bfe_basic.IsMultipartUpload(req.HttpRequest.Header); multipart {
        break
    }
    if b.MaxFileBytes > 0 && req.HttpRequest.ContentLength > b.MaxFileBytes {
        return "batch_file"
    }
```

非 multipart（raw JSONL）保持原样：那里 `ContentLength` 就是文件字节数，早期拒绝仍然有效（SC28 TC-04a 的语义不能变）。

### 4.4 步骤 3（必改）：`preCheckUpload` 重排——先定位 part，再判定，能定则不装闸门

文件：`bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go:86-140`。目标结构：

```go
func (m *ModuleAiBatch) preCheckUpload(req *bfe_basic.Request, ctx *batchContext,
    aiMeta *bfe_basic.AiBasicInfo) (int, *bfe_http.Response) {

    outreq := req.OutRequest
    if outreq == nil {
        return bfe_module.BfeHandlerGoOn, nil
    }
    ctx.contentLength = outreq.ContentLength
    ctx.knownContentLen = outreq.ContentLength >= 0

    boundary, multipart := bfe_basic.IsMultipartUpload(outreq.Header)

    // 1) raw body: envelope == file content, the early byte check stays
    if !multipart && ctx.knownContentLen && ctx.contentLength > 0 {
        if lim := aiMeta.BatchEffMaxFileBytes; lim > 0 && ctx.contentLength > lim {
            ... // 现有 413（UPLOAD_REJECT_SIZE）
        }
    }

    // 2) authoritative count when the `file` part can be located; the buffer
    //    may be a prefix of the body (all=false): a fully read part is still
    //    exact (issue #1434)
    if outreq.Body != nil {
        if accessor, err := outreq.GetBodyAccessor(); err == nil && accessor != nil {
            if buf, _ := accessor.GetBytes(); len(buf) > 0 {
                fileBytes, fileLines := int64(len(buf)), int64(bytes.Count(buf, []byte{10}))
                counted := !multipart
                if multipart {
                    if stats, ok := multipartFilePartStats(boundary, buf); ok {
                        ctx.uploadPartOK = true
                        ctx.uploadFilename = stats.Filename
                        ctx.uploadPartBytes, ctx.uploadPartLines = stats.Bytes, stats.Lines
                        fileBytes, fileLines = stats.Bytes, stats.Lines
                        counted = true
                    }
                }
                if counted {
                    if overBatchFileLimit(fileBytes, fileLines, aiMeta) {
                        ... // 现有 413，消息里的数字现在是文件内容口径
                    }
                    // 判定已是权威：不再安装流式闸门
                    return bfe_module.BfeHandlerGoOn, nil
                }
            }
        }
        // 3) degraded: file part 未完整出现在缓冲窗口内 → 按 part 增量计数
        outreq.Body = newUploadLimitBody(outreq.Body, boundary, ctx, aiMeta)
    }
    return bfe_module.BfeHandlerGoOn, nil
}
```

要点：

- 删除原 `:96-102` 的 multipart 字节预检（raw 分支保留）。
- 去掉 `all=true` 的硬门槛：`multipartFilePartStats` 在读满 `file` part 内容后即返回 `ok=true`，后续 part 的截断不影响已经完整的 part；`file` part 自身被截断时 `countPartContent` 会返回 `io.ErrUnexpectedEOF` → `ok=false`，安全。
- 判定通过后**不再安装闸门**，直接 GoOn（第 3 条原则），彻底消除"前置放行 + 流式中断"的自相矛盾。

### 4.5 步骤 4（必改）：新增流式 multipart 增量闸门，替换 `countingBody`

文件：`bfe_modules/mod_ai_batch/file_counter.go`（新增实现），`mod_ai_batch_handlers.go:137` 的安装点改到 4.4 的降级分支。

- 保留现有 `countingBody` 作为**退化实现**，改名 `envelopeCountingBody`，注释明确"仅用于无法定位 `file` part 的场景（解析失败/非 multipart）"。
- 新增 `multipartLimitBody`：**原样转发字节**（透传约束），旁路维护 RFC 2046 的分隔符状态机，只对 `name="file"` 的 part payload 累加 bytes/lines 并检查上限；`newUploadLimitBody` 按 boundary 是否可用选择它或 `envelopeCountingBody`。

状态机（O(1) 内存，纯旁路，不改一字节输出）：

```
状态：preamble → 等待首个 "--<boundary>"；headers → 累积到 CRLFCRLF（上限 8 KiB）；
      payload → 当前 part 的 body；epilogue → 末分隔符 `--<boundary>--` 之后

分割符探测：只在 payload 阶段，匹配 `\r\n--<boundary>` 且其后紧随 `\r\n`（下一 part）
            或 `--`（结束）才算真分隔符；否则窗口内字节属于 payload，必须回补计数

跨块：保留尾部 len("\r\n--"+boundary)+2 字节的窗口，下次 Read 拼接后再判定
     （文件内容可能包含形似 boundary 的字节，不能按前缀误切）

计上限：仅当当前 part 的 Content-Disposition name == "file" 时累加；
        bytes > BatchEffMaxFileBytes 或 lines > BatchEffMaxFileLines
        → 返回 (n, errUploadLimitExceeded)，置 ctx.uploadAborted（与今天一致）

收尾：EOF 时若 file part 已完整读完 → 回填 ctx.uploadPartOK/uploadPartBytes/
      uploadPartLines/uploadFilename，让 buildUploadFileFields 拿到 exact 值

退化：boundary 缺失、part 头超限、分隔符非法 → 切换到 envelopeCountingBody
      语义（整包计数 + UPLOAD_COUNT_APPROX + lines_src=approx）
```

行为边界：

- 客户端在 `file` part 之前中止 / part 头不完整 → 退化计数，判定按整包（严格口径）。
- 多个 `name="file"` part（不规范客户端）→ 只计第一个，与 `multipartFilePartStats` 一致。
- `filename*`（RFC 2231）、引号包裹的 boundary → 交给 `mime.ParseMediaType` / `mime.ParseMediaType(Content-Disposition)` 处理，不自写解析。
- 无 `Content-Length`（chunked）与有 `Content-Length` 都适用：闸门只看流内容，不依赖 CL。

### 4.6 步骤 5（同步）：计数器与绑定来源语义

- `UPLOAD_COUNT_APPROX` 只统计**真正退化**的上传（今天所有 chunked 上传都算 approx，修复后只有解析失败才算）。
- `UPLOAD_ABORT_STREAM` 的含义收窄为"文件内容确实超限、且到流式阶段才发现"，与 `UPLOAD_REJECT_SIZE`（前置 413）互斥且口径一致。
- `buildUploadFileFields`（`mod_ai_batch_handlers.go:416-448`）的优先级不变（File 对象 `bytes` > file part > 整包兜底），但第三级来源的频率会显著下降；`sys_design/ai_batch.md` §9 中"chunked 上传只能近似"的限制相应改写为"仅在无法定位 `file` part 时近似"。

### 4.7 步骤 6（同步）：文档

- `docs/zh_cn/sys_design/ai_batch.md` §6 增补一条：限额判定对象 = `file` part；§7 计数器表更新 `UPLOAD_COUNT_APPROX`/`UPLOAD_ABORT_STREAM` 含义；§9 修订 chunked 近似限制；§10 变更记录追加本文档。
- `docs/zh_cn/modifications/2026-10-07-ai-batch-support/design-changes.md` §9.2 第 5 条补一句：`max_file_bytes` 的"已知 CL"只适用于非 multipart 上传，multipart 由 `mod_ai_batch` 按 `file` part 判定。
- `CHANGELOG.md`：按现有格式追加一条 `fix(mod_ai_batch,mod_ai_rate_limit): measure batch file limits on the multipart file part (bfe#1434)`。

## 五、回归测试

### 5.1 单测（`testing`，落在 `bfe_modules/mod_ai_batch/`、`bfe_modules/mod_ai_rate_limit/`）

| # | 用例 | 断言 |
|---|------|------|
| 1 | multipart 1 行 / 138 B，`max_file_lines=10`、`max_file_bytes=1048576` | GoOn；**完整读出 `outreq.Body` 无 error**、字节数 = 信封长度（回归 D） |
| 2 | 同一文件，`max_file_bytes=400`（信封 502） | GoOn（回归 B） |
| 3 | 文件 10 行 / `max_file_lines=10`；11 行 / `=10` | GoOn；413 `BATCH_FILE_TOO_LARGE`，`limit_type=batch_file`（issue 建议的 ±1 边界） |
| 4 | 文件 275 B / `max_file_bytes=275`；274 B / `=275` | GoOn；413 |
| 5 | raw `application/octet-stream` 200 B / `max_file_bytes=100`；100 B / `=100` | 413；GoOn（TC-04a 语义保持） |
| 6 | 缓冲窗口小于 file part（`bfe_http.SetAccessibleBodySize` 临时调小） | 走闸门；未超限全量读出无 error；`ctx.uploadPartBytes/Lines` = 文件内容，`uploadPartOK=true`（不再 approx） |
| 7 | 文件内容含 `--<boundary>`、`\r\n--<boundary>x`、跨块的半截分隔符 | 不误判分隔符，bytes/lines 精确 |
| 8 | `purpose` 在 `file` 之前 / 之后；多字段；多 part | 只计 `file` part |
| 9 | 1 字节粒度读取驱动同一 multipart（头/分隔符跨块） | 状态机与整块读取结果一致 |
| 10 | 无 boundary / 畸形 multipart | 退化为整包计数，`UPLOAD_COUNT_APPROX` 语义不变 |
| 11 | `checkBatchLimits`：multipart（CL=502、`MaxFileBytes=400`）/ raw（CL=200、`=100`） | `""`（交给 mod_ai_batch）/ `"batch_file"` |

命令：

```bash
cd bfe
go test ./bfe_modules/mod_ai_batch/ ./bfe_modules/mod_ai_rate_limit/ ./bfe_basic/ -v
make test
```

### 5.2 集成用例（SC28 新增，现有一律保留）

- **TC-17 multipart 上传按文件行数判定**：`batch_limits.max_file_lines=2`、`max_file_bytes=1048576`，用 `e.upload(t, inputJSONL)`（2 行 / 274 B，信封 12 行 / 638 B）→ `200`，provider 收到 1 次上传，`BATCH_FILE.lines=2 / bytes=274` 且 `lines_src=exact`；同一夹具改 `max_file_lines=1` → `413 BATCH_FILE_TOO_LARGE`，provider 收到 0 次上传。
- **TC-18 multipart 上传按文件字节判定**：`max_file_bytes=274`（信封 638）→ `200`；`max_file_bytes=273` → `413`。
- **TC-17/18 附断言（chunked + multipart）**：无 `Content-Length` 的 multipart 上传，文件超限（文件 10 行 / `max_file_lines=3`）→ 非 2xx 且 provider 未拿到完整请求（`UPLOAD_ABORT_STREAM`）；文件未超限（文件 2 行 / `max_file_lines=3`）→ `200`（回归 D 的流式分支）。
- 保留 `TestTC04a_BatchLimitFileBytes`（raw 字节上限）与 `TestTC10_ChunkedUploadLineLimitAbort`（raw chunked）作为非 multipart 回归。

命令：

```bash
cd bfe
go test ./tests/integration/implementation/scenario-SC28-ai-batch-support/ -run 'TestTC0|TestTC1' -v
```

### 5.3 反向验证（必须做）

修复前先落 TC-17/18 与单测 1/2/6，确认**红**（本文档 §2.1 的探针数据已证明单测 1/2/6 在现状下必然失败）；修复后同批用例转绿，且 TC-04a/TC-10 以外的既有 SC28 用例零回归。

## 六、修复后目标行为

- `POST /v1/files`（multipart）在 `max_file_lines` / `max_file_bytes` 下的判定只取决于 `file` part 内容：文件行数/字节数 ≤ 上限必放行（前置或流式皆然），> 上限才 `413 BATCH_FILE_TOO_LARGE`（能前置判定时）。
- 信封开销（本形态 +10 行 / +364 B，随 boundary、文件名、字段数浮动）不再影响任何租户的可用额度；`purpose` 字段、part 头、boundary 一律不计。
- `max_file_bytes` 按小额度配置（例如 1 KB 级）时不再误拒合法文件——这是 A/B 两处修好后最直接的收益。
- 请求体字节仍原样透传；`Content-Length` 不变；provider 侧观察到的 multipart 与客户端发送的一致。
- 绑定与审计：`BATCH_FILE.lines/bytes` 的来源在能定位 part 时全为 exact；`lines_src=approx` 仅出现在解析失败的异常上传上。
- 非 multipart（raw `application/jsonl` / `octet-stream`）行为、错误码、计数器完全不变。

## 七、边界与残余风险

- **超限的流式上传仍不是"干净 413"**：请求头已发往 provider，限流在 body 中途命中时只能中断，客户端看到 5xx/连接被截断（与今天一致）。本修复只保证"该中断时才中断"，不改变错误形态；若产品要求流式超限也返回 413，需另行设计（例如先把 `file` part 落临时文件再判定，与"大文件直转"约束冲突，不建议）。
- **误拒 vs 绕过**：multipart 解析失败时保留整包严格口径（可能误拒带非常规编码的合法请求），优先保证限额不可绕过；该类请求同时由 `UPLOAD_COUNT_APPROX` + `lines_src=approx` 可观测。
- **`Content-Length` 预检彻底移除后**，明显超限的 multipart 请求不再有"零读拒绝"的快路径，最多会读/转发到"上限 + 信封开销"字节后才中断（默认 `max_file_bytes` 100 MB 时仍受全局硬顶约束）。若要保留快速拒绝，可加一条**只可能误拒为 0**的粗判：`ContentLength > limit + 保守开销上界` 才拒绝——但开销上界随客户端 part 数无界，本文档不纳入必改项。
- **`GetAccessibleBodySize()` 默认 2 MB**：≤2 MB 的上传（含 issue 全部样本）走权威前置判定；>2 MB 的 `file` part 才进流式闸门。调小该值的部署不影响正确性，只影响"前置 vs 流式"的比例。
- **不改配置面/模块顺序**：无新增配置项、无 `AiBasicInfo` 字段变更、`mod_ai_rate_limit` → `mod_ai_batch` 顺序不变。
- **不影响其他限额维度**：`max_create_rpm`、`max_active_batches`、tpm/rpm/concurrency 的判定路径不在本文档范围内。

## 八、实施记录（2026-10-20）

按 `bfe-rd-workflow` Phase 1–7 落地；Phase 2（configuration 文档）不适用：本次无新增/变更配置项。

### 8.1 改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/batch_op.go` | 新增 `IsMultipartUpload(header) (boundary string, ok bool)`：`multipart/form-data`/`multipart/mixed` 且 boundary 非空才算信封；两个模块共用同一判定 |
| `bfe_modules/mod_ai_rate_limit/batch_check.go` | 逐策略字节预检对 multipart 直接让位（`Content-Length` 只在 raw 上传时等于文件大小）；`resolveBatchFileLimits` 注释改为"streaming limit gate" |
| `bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go` | `preCheckUpload` 重排：删除 multipart 的 `Content-Length` 413；`file` part 完整落在缓冲前缀内即权威（去掉 `all=true` 门槛）；判定通过后**不再安装闸门**；只有无法定位 `file` part 时才装 `newUploadLimitBody` |
| `bfe_modules/mod_ai_batch/file_counter.go` | `multipartFilePartStats` 改接收 boundary；`countingBody` 更名 `envelopeCountingBody`（退化路径）；新增 `multipartLimitBody`（原样转发 + RFC 2046 分隔符状态机，只计 `name="file"` 的 part）与 `newUploadLimitBody` 工厂；`file` part 读完即回填 `uploadPartBytes/Lines/Filename`（绑定 exact） |
| `bfe_modules/mod_ai_batch/batch_context.go` | 字段注释改口径（`uploadBytes/Lines` = 整包信封，仅作近似兜底） |
| `bfe_modules/mod_ai_batch/upload_limit_test.go`（新建） | `preCheckUpload` 真值表 5 个用例 + 流式路径 1 个 + 闸门状态机 2 个（含 1 字节粒度、boundary 相似串、退化） |
| `bfe_modules/mod_ai_batch/mod_ai_batch_test.go` | `multipartFilePartStats` 用例改用 boundary；新增"前缀含完整 part 仍 exact / 截断进 part 不 exact"；`countingBody` 用例改名 |
| `bfe_modules/mod_ai_rate_limit/batch_check_test.go` | 新增 `TestCheckBatchLimitsMultipartBytePrecheck`：multipart 信封 502/limit 400 不拒绝；raw 401/limit 400 仍拒绝 |
| `bfe_basic/batch_op_test.go` | 新增 `TestIsMultipartUpload` 真值表 10 例 |
| `docs/zh_cn/sys_design/ai_batch.md` | §6 改写为"统计口径 = 限额判定口径"（含前置/流式分工）、§7 计数器语义拆分、§9 修订 chunked 近似限制、§10 变更记录 |
| `tests/integration/implementation/scenario-SC28-ai-batch-support/sc28_ai_batch_test.go` | 新增 `TestTC17_BatchLimitFileLinesMultipart`（5 子用例）、`TestTC18_BatchLimitFileBytesMultipart`（5 子用例）与 `uploadMultipart`/`uploadMultipartChunked` helper |
| `tests/integration/测试设计文档/scenario-SC28-批量任务支持/TC-17-*.md`、`TC-18-*.md`（新建） | 测试例设计文档；`场景说明.md` 与 `测试场景总体说明.md` 登记（SC28 计划用例 16 → 18） |
| `CHANGELOG.md` | `### Fixed` 追加一条（bfe#1434） |

### 8.2 实施中确认的偏差（与 §四/§五 方案的差异）

1. **流式闸门的触发条件比方案更窄**：方案里"chunked / 超缓冲上限即走闸门"，实测（以及 SC28 的 `accessibleBodySize=4 MB`）比方案更宽——只要 `file` part 完整落在缓冲前缀内就是权威前置判定。因此 chunked 超限在缓冲范围内是**干净 413**（不是中断），闸门只对"`file` part 超过 `visibleBodySize`"生效。TC-17 据此拆成 `chunked over limit rejected on file content`（断言 413 + 报错里的行数是文件的 10 行而非信封的 20 行）与 `oversized stream aborts on file content`（5.4 MB 文件，断言中断）。
2. **闸门状态机实现细节修正**（均由单测先红后绿暴露，方案未覆盖）：
   - 首个 `--boundary` 恰好在本轮缓冲末尾时，必须保留 boundary 本身（原实现只保留其后字节会丢掉分隔符）；
   - 查找分隔符必须搜索"整个剩余缓冲"再判断起点是否落在可判定区间内，否则分隔符起始字节会被计入文件内容（每文件多计 1–2 字节/行，1 字节读取时确定性触发误中断）；
   - `overLimit()` 不能因 `finished`（`file` part 已读完）短路：`file` part 与超限可能发生在同一个 Read 里，短路会放过超限上传。
3. **`multipartFilePartStats` 接收 boundary 而非 Content-Type**：解析只做一次（`IsMultipartUpload`），避免两处重复 `mime.ParseMediaType`。
4. **multipart 解析失败保留整包严格口径**（与方案一致）：退化后把后续字节都计入文件内容，可能误拒非常规编码的合法请求，但限额不可绕过。

### 8.3 验证结果

| 范围 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| `go test ./bfe_basic/... ./bfe_http/... ./bfe_modules/... ./bfe_server/...` | 全部通过（含新增 9 个单测函数） |
| SC28 全量（19 个测试函数，其中 TC-17/TC-18 含 10 个子用例） | 通过（14.0s） |
| 回归 SC02 多 API-Key（20.6s）、SC07 Redis 限流（2.2s） | 通过 |
| **反向验证**：临时回退 `preCheckUpload`（恢复 multipart `Content-Length` 413 + 无条件安装信封闸门）后重跑 TC-17/TC-18 | 确定性 FAIL：① `file lines == limit` → `500`（信封闸门中途中断，复现 issue 的流式症状）；② `file bytes == limit`、`per-policy precheck` → `413`（复现 issue 的 413 症状）。恢复代码后转绿 |

### 8.4 未做与待办

- **Git 提交：未执行**（`bfe-rd-workflow` 要求人工确认；当前改动全部留在工作区）。
- `2026-10-07-ai-batch-support/design-changes.md` §9.2 第 5 条的历史表述（"`max_file_bytes` 已知 CL 时拒绝"）未改动：该文档是当期的方案记录，现行口径已由本文档 + `sys_design/ai_batch.md` §6 覆盖。
- 部署后建议在 `integration-test` 仓库复跑 `SC1302-TC015`（issue 原始失败用例）做最终确认，并回写 `ai-gateway-api#256`。