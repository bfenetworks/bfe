# BFE Issue #1494 修复方案：bytes_body 缓冲记账生命周期——重放保留期内不得提前减账

- Issue: https://github.com/bfenetworks/bfe/issues/1494（系统测试 SC3101-TC006）
- 缺陷：启用 fallback 的 AI 请求把请求体缓冲为 `bytes_body` 供重放时，`total_bytes_body_buffer` 监控指标在**主 Target 请求体写完的瞬间**就被减账（`bytes_body.Close()`），但 buffer 仍被保留供 fallback 重放（`Rewind()`）。后果有二：(1) 监控指标只在上传/写主 Target 的几十毫秒内可见，主 Target 响应头超时（现场 12 秒）的整个保留期内归零，少报真实常驻内存；(2) `TotalBodyBufferSize > 0` 时，`prepareRequestBodyForRetry()` 的限额判断读到的是已减账的值，保留中的 buffer 不占用额度，新请求仍被允许包装，容量保护失效。
- 代码基线：`bfe/`，`v1.9.0-dev`（含 issue #1493 修复，tip `14fcdb8d`）；行号以当前工作树为准。issue 现场部署构建 `eb4ed4a4` 与本方案所读源码同链（issue 已用 `go version -m`/`strings` 排除二进制差异）。
- 关联：#1317（fallback 状态码白名单）、#1387、#1482、#1416、#1440/#1441/#1442（key 选择/错误契约）、#1426（body model 改写）、#1406/#1468（body 读写链）。与计费/日志口径（#1398/#1409/#1480/#1493）无关，本卡不动 `TokenUsage`。
- **产品裁定点（issue 已声明「需要产品裁定：是」）**：`bytes_body` 的「活跃」生命周期。**已于 2026-11-08 裁定：「仍可能被 fallback/key 重放重放的 buffer 即活跃 buffer」**（计为活跃），与本方案推荐一致，依据是 issue 自己的验收判据 1/2（在途窗口内 gauge 必须上升、保留额度必须阻挡后续包装）以及 `bfe.conf.md:37`「所有**活跃** bytes_body buffer 占用内存之和的上限」+ `mod_ai_route_bfe_changes.md:738`「达到上限后不再包装 body」的语义闭环。SC3101-TC006 据此解除 DEFERRED，待发布后重跑判分。

## 一、口径依据（Expected 的来源）

1. `docs/zh_cn/configuration/bfe.conf.md:37`——`Server.TotalBodyBufferSize`：「所有活跃 bytes_body buffer 占用内存之和的上限……达到上限时，AI Gateway fallback 不再对请求体做缓存封装，即不重传」。
2. `docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md:530`（§4.3.5）——「当全局 bytes_body 缓冲区大小达到 `TotalBodyBufferSizeLimit()` 限制时，不再包装，fallback 被禁用」；`:738`（§8.3）——「全局 bytes_body 缓冲区受 `TotalBodyBufferSizeLimit()` 限制，达到上限后不再包装 body，fallback 会被禁用」。
3. **条款推论（issue 根因链第 4 环已给出）**：fallback 设计要求 buffer 在主 Target 之后仍可 `Rewind()`，因此该 buffer 在请求终态（最后一次 attempt 结束、不再可能被重放）之前仍属「活跃 bytes_body buffer」，必须计入 gauge；否则限额门槛与文档定义不一致。
4. **条款缺口（本方案补写）**：现有文档未写明记账释放点。本方案在落地时同步补充：「bytes_body buffer 从被包装（或成为可重放 body）起计入 `TotalBytesBodyBuffer`；在仍可能被 fallback/key 重放重放期间保持活跃；仅在请求的最后一次 attempt 结束（不再可能 Rewind）后释放记账」。

issue 的验收判据（原文 1–5）在本裁定下全部可判：在途窗口 gauge 上升 ≥768KiB（判据 1）、保留额度阻挡第二次包装（判据 2）、2.5MiB 超限请求 503/fallback 禁用/命中差分 1/0（判据 3）、终态归零无泄漏（判据 4）、fallback 行为不回归（判据 5）。

## 二、根因链（源码直读，当前工作树）

### 2.1 加账点：`newBytesBody()` 读完全量 body 才加账

`bfe_http/transfer.go:950-957`：先 `io.ReadAll(io.LimitReader(src, maxSize))` 读完整包 body，随后 `addTotalBytesBodyBuffer(len(bb))`。因此限速上传的 1MiB body，加账发生在上传结束（现场约 10.5s）——这一点本身符合预期（buffer 此时才真实存在），不是问题。

### 2.2 包装点：`prepareRequestBodyForRetry()` 在进入 attempts 循环前包装

`bfe_server/reverseproxy.go:1490-1501`（cluster fallback 路径）与 `:2205-2221`（key-level retry 路径）：`GetBodyAccessor()`（`bfe_http/request.go:981-1007`）把 body 替换为 `*bytes_body`（`NewBytesBody`），持有全量 body 供后续 `Rewind()`。限额检查在包装前：`TotalBytesBodyBuffer() >= limit` 则拒绝（`:2601-2605`）。

### 2.3 提前减账点：`transferWriter.WriteBody()` 写完后端立即 `Close()`

`bfe_http/transfer.go:205-229`：body 写完后端后 `t.BodyCloser.Close()`。`bytes_body.Close()`（`:878-891`）**同时做两件事**：关源（`src.Close()`）+ 减账（`subTotalBytesBodyBuffer(len(b.buf))` 并置 `released=true`）。此时 fallback 尚未决策，buffer 仍被 `b.buf` 引用保留。

### 2.4 重放不加账：`Rewind()` → `SetBytes()` 在 `released=true` 后跳过加账

`bfe_http/transfer.go:931-940`（`Rewind`）→ `:911-925`（`SetBytes`）：`if !b.released { sub(old); add(new) }`——`released=true` 时**完全不触碰 gauge**。于是主 Target 超时后 fallback 重放实际使用的保留 buffer，永远不再出现在 gauge 上。`bfe_server/reverseproxy.go:2522-2543`（`resetRequestForRetry` → `rewindRequestBody`）与 `:2263`（key 重放 rewind）都走这条路径。

### 2.5 暴露点与后果

`bfe_server/server_status.go:191` 把包级 atomic 暴露为 `total_bytes_body_buffer`。结合 2.3/2.4：监控窗口 = 「body 上传完成 → 写完后端」的几十毫秒（现场对照请求实测 34.9ms spike），而不是「包装 → 最后一次 attempt 结束」的完整保留窗口（现场 1MiB hold 请求约 24 秒全程为零）。`prepareRequestBodyForRetry()` 的限额判断（`:2602`）读同一 atomic，保留中的字节不可见 ⇒ 限额可被绕过（`TotalBodyBufferSize>0` 的部署）。

### 2.6 排除项（issue 已取证，本方案沿用其结论）

监控口与数据面同进程（`ss -ltnp` 同 pid）；部署二进制含记账符号；17ms 级外部采样仍抓不到上升——窗口确实只有几十毫秒。功能 fallback 语义未坏（2.5MiB 请求 503、命中差分 1/0、hold 请求经 fallback 200 均通过），本卡只覆盖观测/容量记账缺陷。

## 三、修复方案（推荐）

设计原则：

1. **「活跃」重定义**：bytes_body buffer 从包装起计入 gauge；只要仍可能被重放（fallback/key retry 未走完），就保持计入；**最后一次 attempt 结束、不再可能 `Rewind` 时释放记账**。这正是产品裁定的结论（2026-11-08：计为活跃），与验收判据 1/2 一致。
2. **拆开「关闭源」与「释放记账」**：`Close()` 的语义（HTTP 惯例：消费完毕关源）不变；记账释放改由 reverseproxy 在请求终态显式结算。
3. **最小侵入**：不改动 `newBytesBody()` 的加账时机、不改 `prepareRequestBodyForRetry()` 的限额判断逻辑（保留中的 buffer 被计入后，该判断自动恢复语义）、不动响应侧 body（响应 body 一次性消费，`Close()` 减账语义不变）。
4. **恰好一次**：每个 bytes_body 的 gauge 加/减恰好一次（`released` 标志保证），幂等的 `Retain`/`Release` 允许多个层级重复调用。

### 步骤 1（必改，核心）：`bfe_http/transfer.go` —— `bytes_body` 记账生命周期

新增 `retained` 字段并改 `Close()`，新增 `RetainForRetry()`/`Release()`：

```go
type bytes_body struct {
    src       io.ReadCloser
    buf       []byte
    all       bool
    srcClosed bool
    released  bool // gauge 已减账（活跃定义：!released）
    retained  bool // 为 fallback/key 重放保留：Close 只关源、不减账，
                   // 记账由 Release 在请求终态结算（issue #1494）
    r         *bodyReader
    err       error
}

func (b *bytes_body) Close() error {
    if b.srcClosed {
        return nil
    }
    b.srcClosed = true
    if !b.retained && !b.released {
        b.released = true
        subTotalBytesBodyBuffer(int64(len(b.buf)))
    }
    if b.src == nil {
        return nil
    }
    return b.src.Close()
}

// RetainForRetry marks the buffer as retained for replay: Close() stops
// releasing the gauge; the caller MUST pair it with Release() at the
// request's terminal state (issue #1494). Idempotent.
func (b *bytes_body) RetainForRetry() { b.retained = true }

// Release settles the gauge accounting once the buffer can no longer be
// rewound (last attempt settled). Idempotent; also closes the source if
// still open (defensive, e.g. early-exit paths where WriteBody never ran).
func (b *bytes_body) Release() {
    if !b.released {
        b.released = true
        subTotalBytesBodyBuffer(int64(len(b.buf)))
    }
    if !b.srcClosed {
        b.srcClosed = true
        if b.src != nil {
            b.src.Close()
        }
    }
}
```

`SetBytes()` 补防御分支：`released=true` 时改为 `add(new)` 并置 `released=false`（重放/改写重激活记账）。保留流程下 `released` 在 Rewind 前始终为 false（Close 不再置位），原有 `sub(old)+add(new)` 净额不变；该分支兜住「Release 后又被改写」的异常时序。

**实施期修订 1（相对初稿，重要）——记账一致性优先，限额只在文档闸门 enforcement**：初稿保留「`newBytesBody()` 无视 `addTotalBytesBodyBuffer` 拒绝返回值」为既存软行为（§五残余风险），实施中改为：`addTotalBytesBodyBuffer` **不再带拒绝语义**（gauge 永远反映真实持有的字节，允许超过上限），限额 enforcement 唯一保留在文档定义的闸门——`prepareRequestBodyForRetry()` 的包装前检查（`TotalBytesBodyBuffer() >= limit`）。理由（实施实测）：

- **硬拒绝不可行**：`GetBodyAccessor()` 先赋值后判错（`request.go:1002-1006`），`NewBytesBody` 失败会把 `r.Body` 置 nil，而 body 已被 `io.ReadAll` 消费、ContentLength 非零，请求继续流转会在 `chunk_writer.go:358` 补读残余 body 时 nil panic（实施时实测 BFE 崩溃）；
- **软拒绝（不计数但创建成功）更糟**：复现本卡修复的「少报」，且终态 `Release` 减账会把 gauge 推负；
- 上限语义不变：「达到上限后不再包装」仍由 prepare 闸门执行（判据 2 的「拒绝包装/禁用 fallback」即此）；gauge 超上限只是如实反映并发保留的真实内存。

**实施期修订 2——发现「prepare 前包装」事实**：AI 路径在每个请求进入 `ServeHTTPForAI` 之前就包装 body（`http_conn.go:573` `extractClientModel` → `condition.ReqBodyJsonFetch` → `HttpReqBodyJsonGet` → `GetBodyAccessor`），因此**当前请求自己的 body 在 prepare 限额检查时已计入 gauge**。这带来两个推论：(a) 限额语义是「全局真实占用」，修复后更精确（旧代码下这部分字节在 Close 后即消失，限额外低估）；(b) 集成臂 B 的参数必须满足 limit ∈ (1×body, 2×body]——A 检查时需要 gauge(=A 自己) < limit，B 检查时需要 gauge(=A 保留 + B 自己) ≥ limit（实施时先用 limit=1×body 实测：A 被自己的 early wrap 顶到 limit 而被拒；该行为本身证明 early wrap 计入生效）。

### 步骤 2（必改）：`bfe_http` 导出类型断言助手

`bytes_body` 类型未导出，`bfe_server` 无法直接引用，新增（`transfer.go` 或 `request.go`）：

```go
// RetryBuffer is implemented by request bodies whose buffer memory is
// retained for replay and must stay accounted until the request settles
// (issue #1494).
type RetryBuffer interface {
    RetainForRetry()
    Release()
}

func RetainBodyBufferForRetry(body io.ReadCloser) {
    if rb, ok := body.(RetryBuffer); ok {
        rb.RetainForRetry()
    }
}
func ReleaseBodyBuffer(body io.ReadCloser) {
    if rb, ok := body.(RetryBuffer); ok {
        rb.Release()
    }
}
```

### 步骤 3（必改）：`bfe_server/reverseproxy.go` —— 保留与终态结算

1. `prepareRequestBodyForRetry()`（`:2599-2617`）：在既有成功路径末尾（body 已确认为可重放）追加 `bfe_http.RetainBodyBufferForRetry(req.Body)`。两处调用点（`:1497` cluster fallback、`:2215` key-level retry）自动获得保留语义；幂等，重复调用无害。限额判断（`:2601-2605`）**不动**——保留中的 buffer 被计入后，该判断自然恢复「达到上限不再包装」的文档语义。
2. `ServeHTTPForAI`：在 prepare 代码块（`:1490-1501`）之后立即注册终态结算：

```go
// The request body buffer stays accounted for as long as any attempt may
// still rewind it; settle the gauge when ServeHTTPForAI returns (no more
// Rewind is possible after the attempts loop). No-op when nothing was
// retained (Close already released the gauge) (issue #1494).
defer bfe_http.ReleaseBodyBuffer(basicReq.HttpRequest.Body)
```

   关键点：`aiClusterInvoke` **只有** `ServeHTTPForAI` 一个调用点（`:1519`），key-level retry 的保留发生在其内部（`:2215`），随 `aiClusterInvoke` 返回到外层后不再发生 `Rewind`，因此**单一 defer 覆盖两条保留路径**，`aiClusterInvoke` 内不加 defer（避免首个 attempt 返回时提前减账）。
3. 结算窗口说明：defer 在 `ServeHTTPForAI` 返回时执行，即「最后一次 attempt 结束 + 响应写出完成」。gauge 会把响应写出阶段也计入——比「attempt 循环收敛即释放」略保守，但与验收判据 1（在途窗口含 12s 超时必须可见）与判据 4（请求结束后回落）完全兼容，且对 `goto response_got` 等所有出口天然安全。若产品希望更紧的「循环收敛即释放」，把 defer 换成循环后显式 `Release` + 保留 defer 作兜底即可（两步都幂等），实施时可二选一，推荐 defer 单挂。

### 步骤 4（不改）：监控与配置

- `server_status.go:191` 不动（暴露同一 atomic，语义修正由记账侧完成）。
- `bfe_server/bfe_server.go:175`（`SetTotalBodyBufferSizeLimit` 装配）不动；无新配置项。

### 步骤 5（必改）：文档补写「活跃」定义（条款缺口）

- `docs/zh_cn/configuration/bfe.conf.md:37`：`TotalBodyBufferSize` 补充描述追加——「『活跃』指 buffer 被包装起至请求最后一次转发 attempt 结束（不再可能被 fallback/key 重放重放）为止；期间即使请求体已写完上游，只要仍可能被重放，buffer 继续计入」（issue #1494）。
- `docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md` §4.3.5 与 §8.3（`:530`/`:738`）各补一句记账生命周期说明 + issue #1494 交叉引用。

### 步骤 6（必改）：单测

`bfe_http/transfer_test.go`（沿用既有 `resetTotalBytesBodyBuffer` 惯例）：

| 用例 | 断言 |
| --- | --- |
| `TestBytesBodyRetainForRetryCloseDoesNotRelease` | NewBytesBody → gauge=size；`RetainForRetry()`；`Close()` → **gauge 仍为 size**（复现 SC3101 在途窗口）；`Release()` → gauge=0 |
| `TestBytesBodyRetainRewindKeepsAccounting` | Retain → `Close()` → `Rewind()`（`SetBytes` 同尺寸换入）→ gauge 恒 = size → `Release()` → 0（钉住 2.4 环修复） |
| `TestBytesBodyReleaseIdempotentAndCloseAfterRelease` | `Release()` 两次、`Close()` 再调用 → gauge 只减一次、不返负 |
| `TestBytesBodySetBytesReactivatesAfterRelease`（防御分支） | `Release()` 后再 `SetBytes(new)` → gauge=new size（重激活），再 `Release()` → 0 |
| `TestBytesBodyUnretainedCloseStillReleases`（回归） | 不 Retain：现行 `TestBytesBodyBufferAccounting` 等保持通过（响应侧/普通路径语义不变） |

`bfe_server` 单测（既有 reverseproxy 测试基建）：

| 用例 | 断言 |
| --- | --- |
| `TestPrepareRequestBodyForRetryRetainsBuffer` | `prepareRequestBodyForRetry` 成功后 body 实现 `RetryBuffer` 且 `RetainForRetry` 生效：`Close()` 后 `TotalBytesBodyBuffer` 不减；`ServeHTTPForAI` 返回后归零 |
| `TestTotalBodyBufferLimitBlocksSecondWrap` | `SetTotalBodyBufferSizeLimit(1.5MiB)`：请求 A 走 hold 后端（响应头超时）在途保留 1MiB 时，请求 B 的 `prepareRequestBodyForRetry` 返回 false（fallback 禁用）——验收判据 2 的单测形态 |

### 步骤 7（必改）：集成测试（fallback 场景 + 监控断言）

在既有集群 fallback 集成场景（`tests/integration/implementation/` 下 fallback 相关 scenario）新增 TC 或在现有 fallback TC 追加两臂：

- **臂 A（gauge 可见性，验收判据 1 复刻）**：`TotalBodyBufferSize=0`；主 Target hold（`TimeoutResponseHeader=12000`）+ Fallback 200；1MiB body 限速上传；在途期间轮询 `/monitor/server_stat` 的 `total_bytes_body_buffer`（50-200ms 间隔、≥40 采样）→ 相对基线上升 ≥768KiB；请求结束后回落基线 ±256KiB。
- **臂 B（限额语义，验收判据 2 复刻）**：`TotalBodyBufferSize=1.5MiB`；请求 A（hold 主 Target）在途保留 1MiB 时，请求 B（同 fallback 配置）→ B 的 fallback 被禁用（`bfe` 日志出现 `request body is not rewindable, disable fallback`），主 Target 命中 +1、Fallback 命中 +0。
- **臂 C（对照，验收判据 3）**：2.5MiB（> AccessibleBodySize）请求 → 主 Target 503、fallback 禁用、命中差分 1/0、gauge 出现短 spike 后归零。注意：修复后该 spike 窗口会**略微变宽**（保留期延伸到 fallback 决策点，因 `:1491` 处 prepare 已成功保留、Rewind 失败才中止），仍远小于在途窗口，判据 3 不变。
- **回归臂（判据 5）**：4xx 不触发 fallback、5xx 触发、全部失败返回最后响应或 500——沿用既有 fallback TC 断言。
- 测试设计文档同步（对应 scenario 的 `场景说明.md` 与总体说明计数）。

## 四、备选方案与决策点

| 方案 | 做法 | 取舍 |
| --- | --- | --- |
| **A（推荐，已采用）** | 保留标志 + `Close()` 条件减账 + reverseproxy 终态 `Release()`（§三） | 恰一次、幂等、对响应侧/普通路径零影响；产品已裁定「保留即活跃」（2026-11-08） |
| B | `Rewind()`/`SetBytes()` 在 `released=true` 后重新加账，每轮 attempt 写完后 `Close()` 减账 | 不解决「主 Target 写完 → fallback 决策」的保留窗口不可见（gauge 仍只覆盖写 body 瞬间）；限额判断仍低估；**不满足验收判据 1/2**，否 |
| C | `newBytesBody()` 创建时加账、由 GC/finalizer 减账 | 释放点不确定，监控语义不可判；否 |
| D | 在 attempt 循环收敛点（而非函数返回）显式 `Release` | gauge 更早回落，但需覆盖所有 `goto`/early-return 出口；可作为方案 A 的步骤 3 变体（幂等， defer 兜底） |

**决策点 1（产品裁定）**：~~保留给重放的 buffer 是否计为活跃~~ **已裁定（2026-11-08）：计为活跃（保留即活跃），与本方案一致，不再阻塞发布**；SC3101-TC006 解除 DEFERRED，发布后重跑即可判分。
**决策点 2（实现选择，不阻塞）**：释放点取「`ServeHTTPForAI` 返回」（含响应写出窗口，推荐）或「attempt 循环收敛」（更紧，需额外出口覆盖）。两者均满足验收判据。

## 五、边界与残余风险

- **2.5MiB 超限请求的 spike 窗口变宽**（§三.7 臂 C）：保留延伸到 Rewind 失败点，量级仍为毫秒级；判据 3 的 503/禁用/命中差分不受影响（SC01 TC09 回归通过）。
- **超上限时的 gauge 会如实超过 limit**（实施期修订 1）：`addTotalBytesBodyBuffer` 不再拒绝加账——gauge 是观测口径，必须等于真实占用；enforcement 只在 prepare 闸门。并发大 body 在途时 `total_bytes_body_buffer` 可能短时高于 `TotalBodyBufferSize`，属设计内的软上限语义（`bfe.conf.md:37`「达到上限时不再**包装**」），不是泄漏；终态必须归零（单测与 TC13 臂 A drain 断言钉住）。
- **「prepare 前包装」使限额检查包含当前请求自己的 body**（实施期修订 2，`http_conn.go:573`）：单请求 body ≥ limit 时，该请求自己的 fallback 会被禁用（实测：limit=1×body 时请求在 prepare 处被拒）。语义上等于「真实占用达上限即不再给重试配额」，与文档一致；但若产品期望「当前请求自己的 body 不计入对自己的判断」，需另开卡调整（把检查改为 `TotalBytesBodyBuffer() - ownBody >= limit` 需要 bytes_body 反向上报自身尺寸，复杂度不值，且会重新打开低估漏洞）。
- **恰好一次依赖 defer 执行**：`ServeHTTPForAI` 的所有出口（含 `goto response_got`、panic 展开）都会执行 defer；若极端路径下 body 被替换（`req.Body` 指针在 prepare 后被改写），defer 捕获的是 prepare 后的指针——与保留对象一致；若中途被第三方换成新的非 bytes_body，旧 buffer 的 Close 链（src.Close 级联）仍会减账（未保留路径）或等待 defer 释放（已保留路径）。替换 body 的模块应自行处理旧 body，属既有约定。
- **key-level retry 与 cluster fallback 叠加**：外层单一 defer 覆盖两条保留路径（`aiClusterInvoke` 唯一调用点已核实 `:1519`）；幂等保证多次 Retain/Release 安全。
- **响应侧 `bytes_body`**（`response.go:310` 等）：`retained` 默认 false，`Close()` 减账语义不变（`reverseproxy.go:557` 等关闭点不受影响）。
- **不改变的行为**：fallback 状态码白名单（`aiFallbackStatusCodes:2414-2421` + 5xx）、batch 直通禁用 fallback/重试（`:1492`/`:2207`）、`AccessibleBodySize` 截断语义、`prepareRequestBodyForRetry` 的判断顺序、`addTotalBytesBodyBuffer` 对 limit=0（默认）的行为（恒不加拒绝）。
- 部署侧 `TotalBodyBufferSize=0`（现场）仅获得正确的观测；`>0` 的部署恢复容量保护（此前限额可被绕过，修复后 hold 类请求会真实占用额度，大并发下可能出现「fallback 被禁用」增多——这是限额语义恢复的预期表现，需在发布说明中提示运维关注 `total_bytes_body_buffer` 与 fallback 禁用日志。

## 六、实施记录（2026-11-08，v1.9.0-dev）

| # | 项 | 状态 |
| --- | --- | --- |
| 1 | `bfe_http/transfer.go`：`bytes_body` 增 `retained` 字段；`Close()` 条件减账；新增 `RetainForRetry()`/`Release()`；`SetBytes()` 防御重激活分支；`addTotalBytesBodyBuffer` 改记账一致性语义（见实施期修订 1） | 已实施 |
| 2 | `bfe_http` 导出 `RetryBuffer` 接口 + `RetainBodyBufferForRetry`/`ReleaseBodyBuffer` 助手 | 已实施 |
| 3 | `bfe_server/reverseproxy.go`：`prepareRequestBodyForRetry()` 成功路径保留；`ServeHTTPForAI` prepare 块后 defer 释放 | 已实施 |
| 4 | 单测：`transfer_test.go` 6 例（保留不清除/Rewind 保持/幂等/重激活/foreign no-op/超上限计数）+ `reverseproxy_body_buffer_test.go` 2 例（保留结算/限额阻挡）；`TestAddTotalBytesBodyBufferLimit` 随语义修订改写为 `...CountsBeyondLimit` | 已实施 |
| 5 | 集成：SC01 新增 `TestTC13_RetainedBodyBufferGaugeLifecycle`（臂 A gauge 在途可见+终态回落、臂 B 保留额度阻挡第二次包装）；落点为 SC01（既有 `GetBFETotalBytesBodyBuffer` 助手与 fallback 基建所在）；TC08/09/10 回归通过 | 已实施 |
| 6 | 文档：`bfe.conf.md:37`「活跃」定义补写；`mod_ai_route_bfe_changes.md` §4.3.5/§8.3 补记账生命周期 | 已实施 |
| 7 | 验证：见下方结果 | 见下 |
| 8 | 产品裁定「保留即活跃」（**2026-11-08 已裁定：计为活跃**）+ 发布后重跑 SC3101-TC006（A-07 rise/drain 断言转 PASSED） | 裁定已归档；重跑待 owner 执行 |

**实施期间实测发现（已在正文同步）**：

1. **prepare 前包装事实**（修订 2）：`extractClientModel` 在每个 AI 请求进 reverseproxy 前包装 body——限额检查含当前请求自己的字节；臂 B 参数需 limit ∈ (1×, 2×] bodySize。
2. **硬上限拒绝不可行**（修订 1）：`GetBodyAccessor` 先赋值后判错 + body 已被消费 → nil Body + 非零 ContentLength 在 `chunk_writer.go:358` 补读时 panic（实测）；软拒绝会少报/推负 gauge。最终采用「记账一致 + 闸门 enforcement」。
3. **TC10 的预置 env 钩子**（`BFE_TEST_INITIAL_TOTAL_BYTES_BODY_BUFFER`，`transfer.go:769-779` init 读取）不受影响。

**验证结果（2026-11-08，本机实跑）**：

| 范围 | 结果 |
| --- | --- |
| `go build ./...`、`go vet`（bfe_http / bfe_server / SC01 测试包） | 通过 |
| 单测 `bfe_http`（含新增 6 例与改写例）、`bfe_server`（含新增 2 例） | 全部通过 |
| 集成 SC01 全量（含新增 TC13 两臂；TC08/09/10 对照） | 通过（12.6s） |
| 集成 SC05 / SC08 / SC28 / SC12 / SC03 / SC29 / SC27 回归 | 全部通过（29.5 / 1.5 / 23.6 / 14.8 / 38.4 / 20.2 / 18.7s） |
