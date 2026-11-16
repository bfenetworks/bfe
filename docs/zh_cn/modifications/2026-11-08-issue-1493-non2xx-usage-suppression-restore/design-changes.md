# BFE Issue #1493 修复方案：未成功调用上游的响应不得在访问日志记录估算 token 残留

- Issue: https://github.com/bfenetworks/bfe/issues/1493（系统测试 SC3201-TC011）
- 缺陷：使用「仅含无路由模型、无路由绑定」的 API Key 请求该模型时，BFE 在路由匹配前本地返回 404（未触达上游、无 upstream request_id、后端 /hits=0），但 pb3 访问日志行仍写入 `ai_input_tokens=43`（≈请求体长度/4）与非零 `ai_output_tokens`，`ai_total_tokens=0`、`ai_cost_value=0`——违反字段 706/707/708「数据源是上游真实 usage；未成功调用上游的响应不存在 usage，token 字段不得填鉴权估算值或字节估长残留」的条款定义，污染按访问日志聚合的 input/output token 报表。
- 代码基线：`bfe/`，`v1.9.0-dev`（tip `904ceab1`）；行号以当前工作树为准。issue 现场部署提交 `751f26f5f9c5` 不在本仓库中（见 §二.4 取证项）。
- 核心结论：**本卡的本质是把运行时隔离开回与规范文档一致——issue #1480 曾在运行时反转 #1409 的「非 2xx 抑制」决策（`ai_usage_source=failed` 照实记录），但并未同步修订两处规范文档；SC3201-TC011 以规范条款为 Expected，因此运行时须恢复 #1409 语义**（非 2xx 且无真实解析 usage ⇒ 清除估算残留 + 字段缺席）。
- 关联：#1409（非 2xx 抑制，本卡恢复其语义）、#1480（total=in+out 补齐 + 非 2xx 记录 failed 用量，**其步骤 2 被本卡反转**，其步骤 1 的 2xx total 补齐保留）、#1398（日志/计费视图隔离，不变）、#1401（`-1` 哨兵夹取，不变）。

## 一、口径依据（Expected 的来源）

issue 明确「是否需要产品裁定：否」，Expected 直接取自产品设计条款：

1. `docs/zh_cn/modifications/2026-08-19-update-ai-access-log-fields/design-changes.md` §10（字段 706/707/708 的语义补充约定，normative）：
   - `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` **只在成功调用上游（2xx）的请求上输出**，值来自响应真实 usage 或 `EstimateToken` 的批准口径估算；
   - **非 2xx 响应（未成功调用上游，如无 AI 路由的 404、上游 404 直通、归一化后的 404）不存在模型用量**：请求结束时清除鉴权播种的估算残留，`mod_access_pb3` **不写**三个字段（字段缺席，与「用量为 0」区分），`ai_cost_value` 同样缺席。
2. `docs/zh_cn/sys_design/rmb_quota.md:697`（计费判定表）与 `:703`（#1409 说明段）：同样写明「非 2xx … 访问日志不输出 token 字段（issue #1409）」「估算残留会从共享 `TokenUsage` 中清除」。
3. **关键事实：#1480 落地时没有改动上述两处规范文档**（`:697` / `:703` 至今仍是 #1409 语义）。即当前仓库处于「规范文档 ↔ 运行时代码」互相矛盾的状态：代码按 #1480 输出 `source=failed` 用量，文档按 #1409 要求字段缺席。SC3201-TC011 以文档条款判定运行时行为失格，本卡消除该矛盾（让代码回到文档，而非改文档迁就代码——条款用语「不得填鉴权估算值或字节估长残留」是禁止性规定，且 issue 已声明无需产品裁定）。

由此，非 2xx 行的 Expected（与 #1409 §一 相同）：

- **B-01**：未成功调用上游且无真实 usage ⇒ `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` **字段缺席**（或 0），`ai_cost_value` 缺席；
- **B-02**：若上游在错误响应中确实返回了真实 usage 对象（`IsFinalUsageSeen()`），属「上游真实 usage」数据源，**保留观测**（disposition `observed` + `source=parsed`，不计费）——这是 #1409 即确立的例外，条款不禁止；
- **B-03**：报告用量的行必须自洽：`ai_total_tokens = ai_input_tokens + ai_output_tokens`（图片/视频按次计费行例外）。

## 二、现状盘点与根因

### 2.1 当前运行时行为（#1480 之后）：本地 404 行照实写出估算残留

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:394-404`：非 2xx → `keepNon2xxUsage`（`:672-719`）：

- `!IsFinalUsageSeen()` 且共享 `TokenUsage` 有播种残留（`PromptTokens = req_body_len/4`）→ **不清零**，置 `UsageDispositionObserved` + `UsageSourceFailed`（`:711-715`），`Non2xxUsageReported` 计数；
- `bfe_modules/mod_access_pb3/request_log.go:530-552`：disposition 非 `none` 即写出 in/out；`UsedQuota==0` 时 total 回退 `in+out`（#1480 步骤 1，`:548-551`）；cost 仅 2xx 写出（`:581-583`）。

于是当前 tip 上 SC3201-TC011 场景（本地 404、`EstimateToken=true`）会产出：`ai_input_tokens=43`、`ai_output_tokens=0`（哨兵夹取）、`ai_total_tokens=43`、`ai_usage_source=failed`、`ai_cost_value` 缺席。`in/total` 非 0 即已违反条款（43 是鉴权估算值），无论 total 是否补齐。

### 2.2 残留的产生链（与 #1409 §二 相同，前三环未变）

1. **鉴权播种**：`mod_ai_token_auth.go` `HandleFoundProduct` 阶段 `IsAllowEstimateToken()` 时 `SetTokenAuthContext(req, tok, req_body_len/4, ...)`，把估算 `PromptTokens` 写入共享 `TokenUsage`——当时完全不知道最终响应状态；
2. **响应侧累加**：`mod_body_process/content_quota_usage.go` 在未识别 usage 时按事件字节 `/4` 累加 `CompletionTokens`（`:117-121`）；创建门槛 `NewQuotaUsageProcessor` 仅 200（`:27-31`）；
3. **日志直读共享对象**：`request_log.go` 直接取 `usage.PromptTokens/CompletionTokens` 写字段 706/707，不判来源；
4. **#1480 新增的第四环**：`keepNon2xxUsage` 有意保留并向日志层交付残留（`source=failed`），替代了 #1409 的清除动作。

### 2.3 现场形态（in=43 / out≠0 / total=0）与 tip 源码不一致，须部署取证

现场报告（product commit `751f26f5f9c5`）的 404 行为 `ai_output_tokens` **非零**、total=0。而当前分支源码上：

- 非 2xx 的唯一 `CompletionTokens` 累加点被 `:28` 的 200 门槛拦住（全部累加点检索：`content_quota_usage.go:121`、`mod_ai_batch/resp_capture.go:208`，后者仅批任务）——**tip 上本地 404 的 `ai_output_tokens` 只可能是哨兵夹取值 0**；
- total=0 且 in/out 非 0 是 #1480 之前的断裂形态，#1480 后 total 会回退为 43。

故现场二进制必与本分支源码不一致（最可能是私有补丁/旧版 `NewCalcCompletionQuota(req)` 无门槛变体，同 #1409 §3.2 的结论）。**取证项（不阻塞修复）**：核对部署镜像 `ghcr.io/rainway-ai-gateway/bfe:*`（`ai-gateway/VERSIONS.yaml`）的实际构建 commit 与 `mod_body_process` 是否含 200 门槛、是否有私有补丁全量 diff。

**本修复不依赖取证结论**：无论残留来自播种还是响应侧累加，只要请求结束时对「非 2xx 且无真实 usage」的共享对象清除残留并置 `none`，日志层即天然缺席；即便部署补丁绕过了 200 门槛，清除动作同样覆盖。

### 2.4 为什么不是改文档而是改代码

#1480 的动机是「`SUM(ai_total_tokens)` 不丢失败行用量」，属报表诉求；它以「本卡确认口径」反转 #1409，但反转的是**运行时行为**，从未修订 §一.1/§一.2 两处 normative 文档。SC3201-TC011 的判定依据是文档条款本身（issue 明确「字段数据源已由产品设计条款明确，运行时行为可直接对照判定」「是否需要产品裁定：否」）。在「代码违反成文条款」与「条款过时」之间，issue 已裁定为前者，故改代码。

## 三、修复方案

设计原则：

1. **运行时回到规范**：非 2xx 且无真实解析 usage ⇒ 清除估算残留 + disposition `none` ⇒ 字段 706/707/708 与 cost 缺席（恢复 #1409 行为）；
2. **保留 #1480 中不冲突的部分**：2xx 行的 total=in+out 回退（`:548-551`）对 unrecognized/估算行仍然需要（B-03 自洽性），**不动**；`ai_cost_value` 的 2xx 门控（`:581-583`）不动；
3. **真实 usage 例外不变**：上游错误响应携带真实 usage 对象时仍按 `parsed` 观测上报（#1409 的 `Non2xxRealUsageKept` 分支原样保留）；
4. **不改计费**：非 2xx 本就零扣费（`:395-404` 早返回），本卡全程不触碰计费副本与 Redis 扣减；
5. 对部署侧可能的私有补丁鲁棒（§2.3）：清除发生在共享对象上，与残留来自哪条写入路径无关。

### 步骤 1（必改，核心）：`keepNon2xxUsage` 恢复「清除残留 + 置 none」

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:672-719`：

- **`IsFinalUsageSeen()` 分支**（`:687-697`）：原样保留——`Observed` + `UsageSourceParsed` + `Non2xxRealUsageKept`，真实 usage 观测不受本卡影响（B-02）；
- **无残留分支**（`usage==nil` 或全 0，`:699-709`）：原样保留——置 `none`，字段缺席；
- **估算残留分支**（`:711-718`）：**改回 #1409 的清除语义**——清除共享 `TokenUsage` 的全部估算字段（`PromptTokens` / `CompletionTokens` / `CacheReadTokens` / `CacheWriteTokens` / `CacheWriteTokens1h` / `AudioInputTokens` / `AudioOutputTokens` / `ImageCount` / `ImageInputTokens` / `VideoCount` / `UsedQuota`；置 `UsageDispositionNone`；**不写** `UsageSource`（字段随 token 块整体缺席）；计数器 `Non2xxUsageReported` 改为清除语义并**改名回 `Non2xxEstimateDropped`**（见步骤 2）；Warn 日志保留（语义改为「dropped estimated usage of non-2xx response」）。清除以 `!IsFinalUsageSeen()` 为前提，真实观测值不被触碰（#1398 契约）。
- `tokenRequestFinishHandler` 的调用点注释（`:396-401`）同步改回「非 2xx 未成功调用上游，不报告模型用量（issue #1409/#1493）」。

伪代码：

```go
usage := aiBasicInfo.GetTokenUsage()
if usage == nil || (usage.PromptTokens == 0 && usage.CompletionTokens <= 0 && usage.UsedQuota == 0) {
    aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionNone) // 保持字段缺席
    return
}
// 估算残留（鉴权播种 + 字节估长累加）：条款禁止进入日志，清除并抑制输出
clearEstimateFields(usage) // 除 UsedCost 外的全部估算字段清零；UsedQuota 置 0
aiBasicInfo.SetUsageDisposition(bfe_basic.UsageDispositionNone)
m.state.Non2xxEstimateDropped.Inc(1)
log.Logger.Warn("%s: drop estimated usage of non-2xx response: status=%d cluster=%s model=%s req_body_len=%d", ...)
```

> `UsageSourceFailed` 常量（`bfe_basic/request_ai_basic.go:440-446`）保留不删（字符串常量，无 pb 兼容问题），注释更新为「#1480 引入、#1493 起无生产者；非 2xx 估算残留不再上报」。

### 步骤 2（必改）：计数器改名回 `Non2xxEstimateDropped`

`:82-94`：`Non2xxUsageReported` → `Non2xxEstimateDropped`，语义恢复为「非 2xx 且无真实 usage，删除了估算残留」。这是 #1409→#1480 改名的回退，**第二次打断同名看板**；发布说明须单列（#1480 的改名已打断过一次，预期无稳定外部依赖）。`Non2xxRealUsageKept`、`AiUsageSuppressed`、`AiUsageInconsistent` 均不变。

### 步骤 3（必改）：`mod_access_pb3` 注释与语义说明同步

- 门控 `:530` 本身**无需改**（disposition `none` ⇒ 整块不写字段，#1409 的机制原样可用）；
- `:526-529` 注释更新：「非 2xx 且无真实 usage 的行已由 `mod_ai_token_auth` 清除残留并置 `none`（#1409/#1493）；非 2xx 但携带真实 usage 的行按 `parsed` 观测写出」；
- `:541-547` 的 total 回退注释更新：回退仅服务 **2xx** 的 unrecognized/估算行（B-03），非 2xx 行进不了该块；
- 不变量检查器 `usageTripleInconsistent` 保持 #1480 的签名（比对已写出的 total），不回退。

### 步骤 4（必改）：单测钉住「404 且未调用上游 ⇒ 三元组缺省」

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`：

| 用例 | 处理 |
| --- | --- |
| `TestTokenRequestFinishHandler_Non2xxKeepsEstimateResidue`（#1480 改写版） | **改回** `..._Non2xxDropsEstimateResidue`（#1409 同名用例）：404 + 播种 prompt + 残留 completion → 共享 `TokenUsage` 估算字段全零、disposition `none`、无 `UsageSource`、`Non2xxEstimateDropped` +1、无扣减 |
| **新增** `TestTokenRequestFinishHandler_Local404UpstreamNeverCalled` | **钉住 SC3201-TC011 的断言**：404、`req.Stat.BackendFirst` 为零值（未调用上游的显式信号）、播种 `PromptTokens=43` → 三字段源值全零、disposition `none`；并对照 `BackendFirst` 已置位的用例（模拟上游被调用后失败）——本卡方案下两者**同**为清除（见 §五决策点；若最终采用备选方案一则此用例分裂为两种期望） |
| `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation` | 保留：`parsed`、`Non2xxRealUsageKept` +1、观测值保留、无扣减 |
| `TestTokenRequestFinishHandler_Non2xxNoResidueNoCounter` | 保留：全 0 → 不清零、不标记、不计数 |

`bfe_modules/mod_access_pb3/request_log_test.go`：

| 用例 | 处理 |
| --- | --- |
| `TestReqAiInfoGenRecordsUsageOnNon2xx`（#1480 新增） | **改写回** `TestReqAiInfoGenSuppressesUsageOnNon2xx`（#1409 同名）：404 + disposition `none` + 播种残留 → 706/707/708/761 **全部 nil**、`usageSuppressed=true` |
| `TestReqAiInfoGenNon2xxWithoutResidueKeepsFieldsAbsent` | 保留（该形态本卡后仍成立） |
| `TestReqAiInfoGenTotalFallsBackToInPlusOut` | 保留但收窄：**2xx** unrecognized/估算行（`UsedQuota=0`、in/out 非 0、200）→ total=in+out；非 2xx 子用例删除（改为缺席断言） |
| `TestReqAiInfoGenUsageSource` 表驱动 | `"non-2xx reports source=failed"` 一行改回 `"non-2xx suppresses the source too"`；保留/新增 `"non-2xx with real usage reports source=parsed"` 一行（真实 usage 例外） |
| **新增** `TestReqAiInfoGenNon2xxRealUsageReportedAsParsed` | 500 + disposition `observed` + `source=parsed` → 三字段照写、cost 缺席（B-02 例外钉住） |
| `TestReqAiInfoGenDetectsInconsistentUsageTriple*` | 保留 |

`bfe_modules/mod_body_process/content_quota_usage_test.go`：#1409 固化的 200 门槛用例（`TestNewQuotaUsageProcessorNonOK`、`TestDoResponseProcessNon2xxNoCompletionEstimate`）原样回归，无需改动。

### 步骤 5（必改）：集成测试（SC05 访问日志字段场景）

`bfe/tests/integration/.../scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go`：

- `TestTC19_Non2xxUsageRecordedWithFailedSource`（#1480 改写版）**改写回** `TestTC19_Non2xxNoUsageFields`（#1409 同名），并扩展为四臂：
  - **臂 A（现场主形态 / SC3201-TC011 复刻）**：`noroute.example.org`（host 命中 token 规则、`ai_route` 无规则）→ BFE 本地 404、后端 0 命中、token/cost/source 字段**全部缺席**，即 SC3201-TC011 的 Expected（B-01）；
  - **臂 B**：mock 后端 404 → 字段全部缺席（残留仅播种 prompt，被清除）；
  - **臂 C（对照臂）**：同后端 200 无 usage → 估算口径不变，`total=in+out` 且 >0（#1480 步骤 1 不回退的证明）；
  - **臂 D（新增）**：mock 上游 409（500 在 fallback 白名单内会触发集群级 fallback 改写响应，409 非白名单 4xx 直通）且响应体带 usage 对象 → 字段全部缺席。**实施偏差（相对初稿）**：初稿计划让臂 D 断言"真实 usage 按 `parsed` 观测写出"，但核实全部 usage 解析路径（`tokenReadResponseHandler:337`、`QuotaUsageProcessor` 创建门槛 `content_quota_usage.go:28`）均有 200 门槛，集成环境无法真实构造"错误响应携带真实 usage"的形态；臂 D 改为反向钉住"错误体中的 usage 对象绝不进日志"，`parsed` 例外由单测 `TestReqAiInfoGenNon2xxRealUsageReportedAsParsed` / `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation` 钉住。
- TC21 臂 3（BFE 本地 404 零成本行）同步改回 `assertNoUsageFields`；`assertFailedRowUsage` 助手删除，`assertNoUsageFields` 扩展为同时断言 `ai_usage_source` 缺席；`noRouteHost` / `sendRequestNoKeepAlive` 复用，testdata 无需新增。
- 测试设计文档同步：`测试设计文档/scenario-SC05-AI访问日志字段校验/TC-19-非2xx响应不输出token字段.md` 改写（含臂 D 与 #1480→#1493 口径反复记录），`场景说明.md` 与 `测试场景总体说明.md` 标注同步。

### 步骤 6（必改）：文档同步

- `docs/zh_cn/modifications/2026-08-19-update-ai-access-log-fields/design-changes.md` §10：追加一句「#1480 曾以 `ai_usage_source=failed` 反转本节约定的运行时行为，#1493（SC3201-TC011）依据本条款裁定运行时失格，已恢复本节语义；真实 usage 例外（`parsed`）不变」；
- `docs/zh_cn/sys_design/rmb_quota.md:703` 说明段：补 issue #1493 一句（#1480 反转被恢复、`Non2xxEstimateDropped` 计数器名恢复）；`:697` 判定表行文本本卡后重新成立，标注（issue #1409/#1493）；
- 本目录新文档（即本文）记录与 #1480 文档的映射；按仓内约定，`2026-10-31-issue-1480-*` 历史文档不改写，仅在其步骤 2 旁注「已被 #1493 反转」的可选交叉引用。

### 步骤 7（部署取证，不阻塞修复）

1. 核对部署镜像实际构建 commit 与分支 tip 的差异，重点 `mod_body_process`（200 门槛）与私有补丁 diff（§2.3）；
2. 修复发布后重跑 SC3201-TC011：本地 404 行应 `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` 缺席、`ai_cost_value` 缺席；
3. 灰度观测：`AiUsageSuppressed` 回升（重新覆盖有残留的非 2xx 行）、`Non2xxEstimateDropped` 与 4xx/5xx 量级同阶、`AiUsageInconsistent` 不升、`Non2xxRealUsageKept` 近 0。

## 四、回归测试与验证

单元测试（步骤 4 全部）+ 集成场景全量回归：

```bash
cd bfe
go build ./... && go vet ./bfe_basic/ ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_modules/mod_body_process/
go test -count=1 ./bfe_basic/ ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_modules/mod_body_process/
go test -count=1 ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/
go test -count=1 ./tests/integration/implementation/scenario-SC29-usage-snapshot-billing/
go test -count=1 ./tests/integration/implementation/scenario-SC28-ai-batch-support/ ./tests/integration/implementation/scenario-SC11-ai-token-auth-billing-fix/ ./tests/integration/implementation/scenario-SC12-client-abort-billing/ ./tests/integration/implementation/scenario-SC03-rmb-quota/
go test -count=1 ./tests/integration/implementation/scenario-SC27-upstream-error-normalization/ ./tests/integration/implementation/scenario-SC20-*/ ./tests/integration/implementation/scenario-SC25-*/
```

重点确认：

- SC05 TC19 四臂按 §三.5 通过；
- SC29（#1398 观测视图、SC29 TC-03「200 客户端中断保留估算」契约）**不受影响**——本卡只动非 2xx 分支；
- SC28 per-image/per-video/batch 的 total 语义不受影响（#1480 步骤 1 的护栏仍在）；
- SC27 上游错误归一化产生的 404：残留被清除、字段缺席（本地拒绝与上游失败在清除语义下行为一致，见 §五）。

## 五、决策点与备选方案

**决策点：上游「被调用但失败」（如上游 404 直通、500）的行，是否与非 2xx 本地拒绝同样处理？**

- **本方案（推荐，已按此撰写）**：同样清除抑制。依据是条款原文「**未成功调用上游**的响应不存在 usage」——「未成功」涵盖调用后失败；issue 的修复方向「对『未成功调用上游』的共享 TokenUsage 清除估算残留（或日志层抑制**非 2xx 无 usage 行**的 706/707/708）」也不区分两者；且只有统一清除才能让 §四 SC27 等归一化 404 场景免于个案判定。SC3201 测试套件若对上游失败行另有 TC，本方案同样满足「无 usage ⇒ 缺席」。
- **备选方案一（仅"从未调用上游"抑制）**：以 `req.Stat.BackendFirst` 是否置位（`reverseproxy.go:411-415`，仅真实发起后端调用才赋值；`request_log.go:443-444` 注释明确「auth-reject / no-route / close 时保持零值」）分流——从未调用 ⇒ 清除抑制（SC3201-TC011 通过），调用后失败 ⇒ 保留 #1480 的 `source=failed` 上报（SUM(total) 不丢失败行估算）。**不推荐**：与条款字面冲突，且 #1480 的报表动机（把鉴权估算当用量汇总）正是条款禁止的「估算残留入报表」；若产品确要保留，需先修订 §一.1/§一.2 两处 normative 文档并显式确认口径，再回到本方案的分流实现（`BackendFirst` 信号已核实可用，改动仅一步）。

## 六、边界与残余风险

- **报表口径回落（预期效果，需周知）**：非 2xx 行的 in/out/total 从「有值（`source=failed`）」回到「缺席」。按 `SUM(ai_total_tokens)` 汇总的报表失败行估算量会再次丢失——这是条款要求，且该残留从未可计费（`ai_cost_value` 恒缺席）；需要「严格真实用量」的报表按 `res_status_code=200` 过滤（与 #1480 §五.1 的既有建议一致，只是过滤后口径与条款完全对齐）。
- **字段缺席 vs 0**：proto `optional` 缺席对 `SUM(COALESCE(...,0))` 聚合等价于 0（`ai-gateway/conf/doris/05_create_insert_job.sql` 既定用法）；`COUNT(ai_input_tokens)` 类查询语义反而更正确（只统计真实上报用量的行）。
- **2xx unrecognized/估算行不变**：`source=unrecognized/estimated/aborted/upstream_dropped` 的 2xx 行仍输出 in/out/total=in+out——#1398/#1410/#1474/#1478 的观测契约与本卡条款（针对「未成功调用上游」）正交，SC29 TC-03 等断言不受影响，不在本卡范围。
- **真实 usage 例外（B-02）**：上游错误响应携带 usage 对象的行按 `parsed` 观测输出，属「上游真实 usage」数据源，与条款不冲突；出现频率应极低，`Non2xxRealUsageKept` 告警保留。
- **部署私有补丁（§2.3）**：若部署二进制确实在非 2xx 上累加 completion（绕过 200 门槛），本卡清除动作仍覆盖（清除在共享对象、finish 阶段、日志生成之前，`bfe_modules.go:155` < `:215` 注册顺序保证）；但**补丁本身必须随修复同步纠正**，否则计费/限流侧可能还有其它绕过路径。
- **不改动的部分**：计费金额、Redis 扣减、fallback/重试、`EstimateToken` 语义（限流预估仍用播种值）、`UsageDisposition`/`UsageSource` 常量、pb 字段编号、模块注册顺序、#1480 步骤 1（2xx total 回退）与步骤 4（cost 2xx 门控）。

## 七、实施记录（2026-11-08，v1.9.0-dev）

| # | 项 | 状态 |
| --- | --- | --- |
| 1 | `mod_ai_token_auth.go`：`keepNon2xxUsage` 改回 `dropUnbilledEstimate`（估算残留分支清除全部估算字段 + 置 `none`；真实 usage 分支保留 `observed`/`parsed`）；调用点与函数注释同步；`bfe_basic` `UsageSourceFailed` 注释标记「#1493 起无生产者」 | 已实施 |
| 2 | 计数器 `Non2xxUsageReported` → `Non2xxEstimateDropped`（state 声明与自增点） | 已实施 |
| 3 | `mod_access_pb3`：门控不动；`request_log.go` token 块与 total 回退注释、`mod_access_pb3.go` `AiUsageSuppressed` 注释同步 | 已实施 |
| 4 | 单测：`TestTokenRequestFinishHandler_Non2xxDropsEstimateResidue`（改写回）、新增 `TestTokenRequestFinishHandler_Local404UpstreamNeverCalled`（SC3201-TC011 钉住：404 + `BackendFirst` 零值/已置位两种形态）、`TestReqAiInfoGenSuppressesUsageOnNon2xx`（改写回）、新增 `TestReqAiInfoGenNon2xxRealUsageReportedAsParsed`、UsageSource 表驱动与 total 回退表同步 | 已实施 |
| 5 | SC05 `TestTC19_Non2xxNoUsageFields` 四臂（A/B/D 缺席、C 对照）+ TC21 臂 3 改回 + `assertNoUsageFields` 扩展（含 `ai_usage_source` 缺席）+ `assertFailedRowUsage` 删除；TC-19/场景说明/总体说明设计文档同步 | 已实施 |
| 6 | `2026-08-19-update-ai-access-log-fields/design-changes.md` §10 追加 #1493 说明（含 parsed 例外）；`rmb_quota.md:703` 追加 #1493 说明（含计数器名反复） | 已实施 |
| 7 | `go build`/`go vet`/受影响包单测 + 集成回归 | 已实施，见下方验证结果 |
| 8 | 部署镜像构建 commit 取证（§2.3）+ 发布后重跑 SC3201-TC011 | 待 owner 执行 |

**验证结果（2026-11-08，v1.9.0-dev tip `904ceab1` + 本修复，本机实跑）**：

| 范围 | 结果 |
| --- | --- |
| `go build ./...`、`go vet`（bfe_basic / mod_ai_token_auth / mod_access_pb3 / mod_body_process / SC05 测试包） | 通过 |
| 单测：`bfe_basic`、`mod_ai_token_auth`（含新增 `Local404` 钉住用例）、`mod_access_pb3`（含新增 `Non2xxRealUsageReportedAsParsed`）、`mod_body_process`、`bfe_server` | 全部通过 |
| 集成 SC05（含改写后四臂 TC19、TC21 臂 3） | 通过（23.8s） |
| 集成 SC29（#1398 观测视图 / SC29 TC-03 契约） | 通过（17.8s） |
| 集成 SC28（batch per-image/per-video total 语义护栏） | 通过（15.0s） |
| 集成 SC11 / SC12（token auth 计费 / 客户端中断） | 通过（2.9s / 12.5s） |
| 集成 SC03（RMB 配额） / SC27（上游错误归一化 404） | 通过（39.5s / 14.4s） |
| `gofmt`（本次改动文件） | 干净（`conf_mod_ai_token_auth.go`、`b2log.go` 为既存违规，未触碰） |

**实施期间发现的额外事实（已在正文同步）**：集成臂 D 初稿用 500，实测 500 在 fallback 白名单（`aiFallbackStatusCodes` 400/401/402/403/422/429 + 全部 5xx）内会触发集群级 fallback、最终响应被改写为 200，故改用非白名单 4xx（409）直通。

**与初稿的实施偏差**：集成臂 D 由「parsed 例外正向断言」改为「错误体 usage 不进日志的反向断言」——全部 usage 解析路径均有 200 门槛，集成环境无法真实构造该形态；`parsed` 例外由单测钉住（见步骤 5 与 §三.5）。
