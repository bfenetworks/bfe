# BFE Issue #1415 修复方案：流式 clean-EOF 无终止事件时按估算计费

- Issue: https://github.com/bfenetworks/bfe/issues/1415
- 缺陷：流式（SSE）请求**已把完整响应体交付给客户端**（HTTP 200、`err_code` 空、写客户端正常、`ai_stream_truncated=false`），但上游未发终止事件（OpenAI `[DONE]` / Anthropic `message_stop`）也未发可解析的 usage 事件时，`MarkResponseCompleted()` 永不置位 ⇒ 收尾时 `estimateBillable = allow && completed` 恒假 ⇒ 已批准的估算兜底（`Server.EstimateToken`）无法生效 ⇒ 计费副本按 0 入账，确定产生了真实推理消耗却按 0 扣费。生产实测（2026-10-12 13:00–13:59 CST，BFE 172.19.1.166）：51 行/小时呈完整断链签名（`ai_input_tokens == req_body_len/4`、`ai_output_tokens ≈ res_body_len/4 × 0.93`、`total=0 且 cost=0`）。
- 代码库：`bfe/`（行号基于当前 HEAD `92b2397c`，2026-10-13 #1414 提交，逐条核实）
- 关联：同族已闭环 #1398（清零块/估算镜像）、#1401（Kimi 嵌套 usage）、#1406（非流式 gzip usage）、#1410/#1414（零计费可观测信号）。**本卡只补「流式完成判定」一环：clean-EOF 收尾**，不动 usage 解析、不动清零守卫、不动适配器终止事件语义。

## 一、根因

### 1.1 完成标记的唯一事件源：协议终止事件

`MarkResponseCompleted()` 在生产代码中只有两个调用点：

| 调用点 | 位置 | 覆盖 |
| --- | --- | --- |
| `QuotaUsageProcessor.Process` on `IsTermination` | `bfe_modules/mod_body_process/content_quota_usage.go:48-50` | SSE 事件级，依赖 `adapter.IsStreamTerminal(ev)` |
| `tokenReadResponseHandler`（非流式整体解析） | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:299-301`（门控 `isCompleteNonStreamBody`，问题 #1406 扩宽） | 非流式完整 body |

而 `IsStreamTerminal` 只认终止事件：

- openai：`message_stop` / `response.completed` / `[DONE]`（`bfe_model_protocol/openai/stream.go:32-36`）
- anthropic：`message_stop` / `[DONE]`（`bfe_model_protocol/anthropic/stream.go:28-30`）

上游干净 EOF 但不发终止事件的流（issue 桶A 51 行的形态：Anthropic 协议、omit usage、HTTP chunked 干净收尾）两个调用点都够不到 ⇒ `IsResponseCompleted()` 恒 false。

### 1.2 gemini「EOF 兜底」先例的真实形态（口径对齐依据）

issue 引用 gemini 接入（`2026-09-09-gemini-protocol-support`）确立的「流 EOF 兜底」先例，**真实落地位置是截断标记而非计费完成标记**：

- `bfe_model_protocol/gemini/errors.go:38-43`：`StreamEndsAtEOF() bool` 能力位；
- `bfe_server/reverseproxy_ai_error.go:451-466`：`aiErrorStreamFilter.finalize()` 在响应 body 读至 EOF 时，若未见过终止事件**且**适配器未声明 `StreamEndsAtEOF`，置 `aiMeta.StreamTruncated = true`；gemini 声明后豁免该标记。

也就是说：gemini 的「EOF 兜底」豁免的是"无终止事件 = 截断"的判定，**并没有**在 EOF 处调 `MarkResponseCompleted()`（全仓生产代码仅上述两个调用点）。gemini 实际不漏计费的原因是其每个 chunk 都带 `usageMetadata` ⇒ `IsFinalUsageSeen` 必然置位（`content_quota_usage.go:45-47`），完成门控根本走不到。**同一产品线对"流已完整结束"存在两种口径**：gemini = 终止事件或 EOF（截断语义层面），openai/anthropic = 只认终止事件（计费完成语义层面）——本修复把计费完成口径对齐为「完整交付即完成」，与 `rmb_quota.md` "正常完成无 usage + EstimateToken → 按估算计费"的批准口径一致。

### 1.3 收尾门控恒假 ⇒ 计费清零（断链最后一环）

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:441-443`：

```go
billingUsage := *tokenUsage
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
unrecognized := !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable
```

对桶A形态：`allow=true`（估算前段已生效，`ai_input_tokens == req_body_len/4` 逐字节相等 44/44）且 `completed=false` ⇒ `unrecognized=true` ⇒ 计费副本全字段清零（`:444-455`）。而共享对象上：

- 鉴权期估算预置 prompt 已生效（`tokenFoundProductHandler`，`mod_ai_token_auth.go:643-671`）；
- chunk 估算累加已生效（`content_quota_usage.go:111-122`，SSE 事件 data 长度 /4 逐 chunk 累加进 `CompletionTokens`）；
- 估算兜底 `:486` `CalcReqUsedQuota` 与镜像 `:489-494` 均 gated 在 `estimateBillable` 上，全部不触发。

⇒ 日志呈现 `in/out > 0 且 total=0 且 cost=0` 的断链签名，配额无扣减。同窗口 45 行**非流式**同形态请求 `total==in+out>0` 正常扣费（非流式由 `tokenReadResponseHandler` 读毕置完成标记），证明镜像链路完好、断点专属流式终止判定。

### 1.4 现有 TC 为何不覆盖

SC29 TC-02（估算兜底回归，`tests/integration/implementation/scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go:394`）的 mock 发送 `content chunk + [DONE]` —— **终止事件 present + usage 缺失**路径；「clean EOF 无终止事件」路径无任何 TC 触达（SC12 中断族只覆盖客户端中断）。issue 所述 `mock-success-no-usage-sse` 若同样发 `[DONE]`（待与 api 仓 e2e 确认），SC1401-TC022 亦只覆盖终止事件 present 路径。

### 1.5 部署差距说明

issue 实测窗口（10-12）的 166 构建同时缺 #1410/#1414 的信号链（`UsageUnrecognized` 计数 + warn + `BillingZeroCost` + `ai_usage_source`，HEAD `mod_ai_token_auth.go:457-468, 521-541`），故"零计数器、零 warn"在 HEAD 已部分缓解；但**功能性断链（本卡主体）在 HEAD 依然成立**，修复仍然必要。

## 二、修复步骤

设计原则：**完成是传输层事实（完整交付），终止事件是协议层事实，二者解耦**。`MarkResponseCompleted()` 的语义即"响应体已完整交付给客户端"——它对终止事件流是冗余置位（幂等），对 clean-EOF 流是补位。不改适配器 `IsStreamTerminal`（避免牵动 SC27 截断语义与 SC12 中断语义），不依赖可选特性开关。

### 步骤 1（必改）：响应完整交付成功时置完成标记

文件：`bfe_server/reverseproxy.go`，`ServeHTTPForAI` 的 `send_response` 块（`:1459-1477`）：

```go
send_response:
	basicReq.Stat.ResponseStart = time.Now()

	if !isRedirect && res != nil {
		err = p.sendResponse(rw, res, resFlushInterval, cancelOnClientClose)
		if err != nil {
			// ... 既有 ErrClientWrite 处理（:1466-1475）...
		} else if res.StatusCode == bfe_http.StatusOK && res.IsSse && aiMeta != nil &&
			aiMeta.Mode != bfe_basic.ModeFile && aiMeta.Mode != bfe_basic.ModeBatch {
			// issue #1415: the full SSE body was delivered and the upstream
			// closed cleanly (copyResponse returned nil). Streams whose
			// upstream omits the protocol termination event never reach
			// MarkResponseCompleted via the event path; clean EOF is the
			// completion signal — the same "ends at EOF" yardstick the
			// gemini adapter declares via StreamEndsAtEOF, now applied to
			// the billing completion gate for every protocol.
			aiMeta.MarkResponseCompleted()
		}
	}
	return
```

置位条件的依据，逐条：

- **`err == nil`（`copyResponse` 成功）**：`io.CopyBuffer`（`reverseproxy.go:1039`）返回 nil 当且仅当上游 body 被**完整**读出——chunked 响应必须读到终止 chunk、定长响应必须凑齐 `Content-Length` 字节，中途断流一律是读错误。这是全链路最强的"完整交付"信号，且**恒真可得**（不依赖任何模块/开关）。
- **200**：非 2xx 在收尾 `tokenRequestFinishHandler` 早退不计费（`:357-363`），此处一并跳过保持语义干净。
- **`res.IsSse`**：把改动限制在流式形态（issue 主体）；非流式完整 body 已由 `tokenReadResponseHandler` 覆盖（含 #1406 扩宽后的 chunked JSON）。
- **排除 `ModeFile` / `ModeBatch`**：与 `mod_body_process` 的处理范围一致（`mod_body_process.go:124-128`），二进制/批量流量不进估算计费。

#### 为什么选这个挂载点（备选方案及排除）

| 备选 | 排除原因 |
| --- | --- |
| `aiErrorStreamFilter.finalize()`（`:451-466`） | 过滤器仅当集群开启 `NormalizeUpstreamError.StreamEnabled` 才安装（`:58-61` 早退）——计费完成判定不能依赖一个可选的错误归一特性开关；且该过滤器语义是错误归一/截断标记，塞入计费标记是错误耦合 |
| `BodyProcessor` EOF（`body_process.go:163-226`） | `mod_body_process` 需产品规则匹配才安装处理器（`mod_body_process.go:118-121` 无规则直接放行），非恒真路径；且未匹配规则时 chunk 估算累加（`content_quota_usage.go:111-122`）同样不会发生 |
| 适配器补 `StreamEndsAtEOF` 式完成位 | 完成是传输层事实，与协议无关；openai/anthropic/gemini/unknown 全部同一判定，无需按协议分叉 |

#### 与既有语义的兼容性（本修复的安全边界）

- **SC12 客户端中断（#1352）不受影响**：客户端中断 ⇒ `copyResponse` 返回写错误 ⇒ `ErrClientWrite`（`:1471`）⇒ 不置位；SSE 响应恒 `cancelOnClientClose=true`（`:1404`），CloseWatcher 关闭源后读必然报错。收尾 abort 分支（`:407-423`）照旧零扣费。
- **上游中途截断不受影响**：中途断流 = 读错误 ⇒ `copyResponse` 失败 ⇒ 不置位。SC27 截断标记（`ErrStreamTruncated`）语义不变——开启 StreamEnabled 的集群上，无终止事件的 clean-EOF 流仍会被标记 `ai_stream_truncated=true`（协议层"没发终止事件"的观测信号），与传输层"完整交付"的完成标记并存，互不矛盾。
- **终止事件流不受影响**：事件路径已在流内置位，此处幂等重复置位。
- **mod_ai_cache 命中**：`AiCacheHit` 在收尾早于守卫返回（`:390-398`），标记无害。

### 步骤 2（观测配套，随本卡一并落地）

- `rmb_quota.md` §7.4 完成判定来源表新增一行「响应体完整交付（`ServeHTTPForAI` copyResponse 成功，仅 SSE）」，并注明与 gemini `StreamEndsAtEOF` 口径对齐；§7.5 同步。
- 发布探针（运营侧）：Kimi/Anthropic 流式无 usage 请求（`mock` 不发 `[DONE]`）后 pb3 `ai_total_tokens` 应为非零估算、`ai_usage_source=estimated`。
- issue 关联 silent-no-signal 范畴（`ai_stream_truncated` 依赖 StreamEnabled 开关、api 仓 e2e `mock-success-no-usage-sse` 确认）不在本卡范围内，记入遗留事项。

### 步骤 3（部署侧，代码外）

- 按 10-09 建议以 tag 干净全量构建替换 166（issue 修复方向 B，与本卡代码修复正交：即便上游实际发了 `message_stop` 而 166 构建未处理，本修复的 EOF 置位仍成立，因为 EOF 在终止事件之后必然到达且幂等）。
- 抓一条桶A 流的上游字节裁定 issue 根因链第 5 环（上游确实没发终止事件 vs 构建未处理）——两种裁定下本修复均成立。

## 三、回归测试

### 单元测试

`bfe_server` 新增（将被测判定抽为小函数便于直测，例如 `shouldMarkAIStreamCompleted(res, aiMeta, copyErr) bool`，`send_response` 块内调用）：

1. `TestShouldMarkAIStreamCompleted` 形态矩阵：
   - copy 成功 + 200 + SSE + chat 模式 ⇒ true；
   - copy 错误（`ErrClientWrite` / 任意读错误）⇒ false（SC12 语义）；
   - 非 200（500/4xx）⇒ false；
   - 非 SSE（JSON 定长 / chunked JSON）⇒ false（非流式路径专属）；
   - `ModeFile` / `ModeBatch` ⇒ false；
   - `aiMeta == nil` ⇒ false。
2. 既有 `bfe_server` 单测全绿（尤其 `reverseproxy_ai_error_test.go` 截断语义、`reverseproxy_stream_usage_test.go` 注入逻辑）。

### 集成测试（SC29 扩展，`tests/integration`）

mock 能力已够：`common/mock_backend.go:70-81` 的 `SSERaw` 逐字写帧、handler 返回即 chunked 干净 EOF，**不改 mock_backend.go** 即可构造「无终止事件 clean EOF」形态。

3. `sc29_usage_snapshot_billing_test.go` 新增：
   - **TC-04 OpenAI clean-EOF 无 `[DONE]`**：`SSERaw` 发 content chunk（无 usage、无 `[DONE]`），`EstimateToken=true` ⇒ 断言 RMB 按估算扣减（`promptEst = 请求体字节/4`，`completionEst = 各事件 data 字节/4 之和，**不含** `[DONE]` 帧）；pb3 日志 `ai_total_tokens = promptEst+completionEst > 0`、`ai_usage_source=estimated`。
   - **TC-05 Anthropic clean-EOF 无 `message_stop`**：`SSERaw` 发 `message_start`（**不带 usage 字段**，模拟 issue 桶A 的 omit-usage 形态）+ content deltas，干净 EOF ⇒ 同 TC-04 断言（这是 issue 生产实证的精确复刻）。
   - **TC-06 clean-EOF + `EstimateToken=false`** ⇒ 零扣费，但 pb3 `ai_usage_source=unrecognized`（信号路径，#1410/#1414 口径）——证明开关关闭时形态仍不可静默。
   - **回归**：TC-01~TC-03 原样通过（尤其 TC-03 客户端中断零扣费——其 backend 本就不发 `[DONE]`，天然覆盖"无终止事件流上的中断不得误标完成"）。
4. 全量回归：SC03（RMB 扣减）、SC05（日志字段）、SC12（中断计费）、SC27（截断标记，含 gemini EOF 豁免）、SC28（batch）、SC20/SC23（缓存命中不扣费）、SC30（嵌套 usage）。

验证命令：`cd bfe && go test ./bfe_server/ ./bfe_modules/... ` 与 `go test ./tests/integration/implementation/scenario-SC29-usage-snapshot-billing/ -v`；提交前 `make test`。

## 四、修复后正确行为（目标链路）

桶A 形态（Anthropic SSE、omit usage、无 `message_stop`、干净 EOF、`EstimateToken=true`）：

1. 鉴权期估算预置 prompt（既有，`tokenFoundProductHandler`）；
2. 流式过程中 `QuotaUsageProcessor` 逐 chunk 累加 completion 估算（既有，`:111-122`）；
3. 上游 chunked 终止 chunk 到达 ⇒ `copyResponse` 成功 ⇒ `MarkResponseCompleted()`（本修复）；
4. 收尾：`estimateBillable = true && true` ⇒ `unrecognized=false` ⇒ 不清零 ⇒ `billingUsage.UsedQuota = CalcReqUsedQuota(prompt估算, completion累加)`（`:486-488`）⇒ 镜像回共享对象（`:489-494`）⇒ `calcCostUnits` 按估算入账、RMB 扣减发生；
5. pb3 日志：`ai_input_tokens = 请求体/4`、`ai_output_tokens = 流字节累加/4`、`ai_total_tokens > 0`、`ai_cost_value > 0`、`ai_usage_source=estimated`——与同窗口 955 行真实 usage 流、45 行非流式 omit-usage 请求的口径一致。

## 五、边界与残余风险

- **`response.incomplete`（Responses API 命中 max_tokens）等行为变化**：该流无 `response.completed`、上游干净 EOF 时，修复前 = unrecognized 零扣费（有信号），修复后 = 按估算计费（`IsFinalUsageEvent` 白名单不含 `response.incomplete`，`IsFinalUsageSeen` 不置位）。响应体已完整交付，按 `rmb_quota.md` 批准口径属估算兜底适用范围；SC29/SC03 的 Responses API 用例回归确认无意外。
- **网关 200-SSE 错误响应**：未开启上游错误归一的集群，若上游以 200 SSE + 错误事件 + 干净 EOF 收尾（无终止事件），修复后按估算计费。与上一条同一口径；#1414 的 `BillingZeroCost`/warn 信号不再覆盖该形态（不再零计费），属预期迁移。
- **`mod_body_process` 未加载/无匹配规则**：完成标记仍会置位，但 chunk 估算累加未运行 ⇒ 计费退化为仅 prompt 估算。部署上 AI 产品均加载 mod_body_process（AGENTS.md 既定前提）；文档注明该依赖。
- **`ai_stream_truncated` 观测仍依赖 StreamEnabled**：无终止事件的 clean-EOF 流在开启归一的集群上 `truncated=true`、未开启集群无此信号（issue 实证 2113 行恒 false 的部分原因）。这是 issue 关联 silent-no-signal 卡的范畴，建议后续将截断检测与错误归一开关解耦；功能修复（本卡）不依赖该位。
- **客户端在完整交付后 RST**：`copyResponse` 已成功则置位完成（err_code 空），收尾不视为中断——响应确已完整交付，与 #1352「未拿到最终 usage 不估算」的语义边界一致（拿到了完整响应，只是没有 usage）。
- **无新增配置项、无模块顺序调整、无转发字节变化**；置点为纯增量，回滚 = revert 单点。

## 六、实施记录（2026-10-13）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | `ServeHTTPForAI` 的 `send_response` 块在 `sendResponse` 成功后经 `shouldMarkAIStreamCompleted`（新增，纯判定函数便于单测）置位 `aiMeta.MarkResponseCompleted()`；条件：copy 成功 + 200 + `res.IsSse` + 非 `ModeFile`/`ModeBatch`（issue #1415） |
| `bfe_server/reverseproxy_stream_completion_test.go` | 新增 `TestShouldMarkAIStreamCompleted` 9 形态矩阵（clean-EOF 置位 / 中断与断流不置位 / 非 200 / 非 SSE / file/batch / nil 保护） |
| `tests/integration/.../scenario-SC29-usage-snapshot-billing/sc29_usage_snapshot_billing_test.go` | 新增 TC-04（OpenAI 无 `[DONE]` 干净 EOF 按估算计费）、TC-05（Anthropic 无 `message_stop` omit-usage 生产桶A复刻）、TC-06（`EstimateToken=false` 零扣费 + `unrecognized` 信号）；配套：`sendAnthropicStream`、`newTestEnvWithEstimateToken` + `rewriteEstimateToken`（生成 conf 改写 `EstimateToken = false`）、AIConf 增 `ModelProtocols: ["openai","anthropic"]` 与 `claude-3` ModelMapping、Anthropic omit-usage 帧数据 |
| `docs/zh_cn/sys_design/rmb_quota.md` | §6.4 `responseCompleted` 字段注释与来源列表新增「流式 clean-EOF 兜底」；§7.4 完成状态小节新增 issue #1415 说明段 |
| `tests/integration/测试设计文档/scenario-SC29-usage抹零修复与include_usage注入/` | 场景说明更新（背景/AIConf/TC 列表/关键实现细节），新增 TC-04/05/06 三份设计文档 |

实施中发现并修正：TC-05 首次运行被 BFE 400 拒绝——SC29 的 AIConf 未声明 `ModelProtocols`，空值默认仅 OpenAI（`clusterSupportsAuthStyle`），anthropic 请求被拒；已在场景 AIConf 声明 `["openai","anthropic"]`（mock 后端同时服务两种协议形态，声明正确）。

验证结果（2026-10-13）：

| 范围 | 结果 |
| --- | --- |
| `go build ./bfe_server/`、`go vet ./bfe_server/`、SC29 包 vet | 通过 |
| 全量单元测试（`go list ./... | grep -v tests/integration`，94 包） | 全部通过（exit 0，无 FAIL） |
| 集成 SC29（TC-01~TC-06） | 6/6 通过：TC-04 按估算扣减精确值、`ai_usage_source=estimated`；TC-05 Anthropic 生产桶A形态按估算扣减；TC-06 零扣费 + `unrecognized` 信号，warn 含 `response_completed=true allow_estimate=false`（证明完成标记独立于估算开关置位）；TC-01~03 回归通过 |
| 回归 SC03（RMB 扣减）、SC05（日志字段）、SC12（中断计费）、SC27（截断标记）、SC28（batch）、SC30（嵌套 usage） | 全部通过 |
| gofmt | 新增/改动文件全部干净；`reverseproxy.go` 有一处 HEAD 既有的 doc 注释格式差异（:1523 附近，与本次改动无关），未顺带重排以避免无关 churn |

未跑：`make test` 全仓（会拉起全部 ~30 个集成场景）；已覆盖本次改动的影响面（bfe_server 全包单测 + 计费/日志/中断/截断/batch/嵌套 usage 六个直接关联场景）。

## 七、运营遗留事项（代码外）

- **损失回补**：评估 2026-10-12 及历史 clean-EOF 零扣费记录（pb3 断链签名：`in>0 && out>0 && total=0 && truncated=false && err 空` 可 SQL 圈定）的回补策略。
- **上游裁定**：抓包确认 deepseek anthropic-compat 端点是否发送 `message_stop`/usage（issue 根因链第 5 环两分支）；若上游实际发送而 166 未处理，按步骤 3 干净重建并复测。
- **api 仓 e2e**：确认 `mock-success-no-usage-sse` 是否发 `[DONE]`；若发，补「omit_usage + 干净 EOF 无终止事件」形态，防止 SC1401-TC022 回归盲区。
- issue-resolution-ledger 登记：`affected_tc_ids` 现有为空，修复后补 SC29 TC-04/05/06。
