# BFE Issue #1480 修复方案：总 token 按输入+输出记录，失败请求也记录用量

- Issue: https://github.com/bfenetworks/bfe/issues/1480
- 缺陷：数据报表明细中，流式请求与返回错误（非 2xx）的请求，`ai_input_tokens` / `ai_output_tokens` 有值而 `ai_total_tokens` 记为 **0**；按总 token 汇总用量时这部分整块漏掉。
- 现状（`v1.9.0-dev`，commit `39c15e10`）：issue #1398 已让"流式 + 估算计费"与 #1474/#1478 的断流/中止行把估算镜像进 `UsedQuota`（total = in+out），但仍有两条形态断裂：
  1. **流式 unrecognized 行**（无最终 usage 且不可估算）：`UsedQuota` 从未被赋值，total 仍为 0；
  2. **非 2xx 行**：issue #1409 的决策是**整组抑制** token 字段（`UsageDispositionNone`），报表侧连 in/out 都看不到。
- 已确认口径（本卡采用）：
  - **A**：`ai_total_tokens` 一律补齐为 `ai_input_tokens + ai_output_tokens` 口径；
  - **B-记录**：非 2xx 请求**照实记录** in/out/total（不再整组抑制），并以 `ai_usage_source=failed` 与上游真实 usage（`parsed`）区分。
- 关联：#1409（非 2xx 抑制，本卡反转其口径）、#1398（估算镜像 total）、#1410/#1414（`ai_usage_source` / `ai_zero_cost_reason`）、#1401（`CompletionTokens=-1` 哨兵 clamp）、#1474/#1478（断流/中止估算）。
- 影响面：仅本仓日志口径 + 一个模块计数器改名；不改 `bfe-access-pb`（字段齐备）、不改 `cluster_conf` / `mod_*.conf` / Redis key、无 DDL。

## 一、现状盘点与根因

### 1.1 字段来源与断裂三元组

`mod_access_pb3` 的 AI 字段全部直读 `AiBasicInfo` 上的共享 `TokenUsage`（`bfe_modules/mod_access_pb3/request_log.go:518-579`）：

| pb 字段 | 取值 | 说明 |
| --- | --- | --- |
| `ai_input_tokens` | `usage.PromptTokens` | 鉴权期按请求体长度播种（`req_body_len/4`），或上游 usage 解析值 |
| `ai_output_tokens` | `usage.CompletionTokens`（负值 clamp 成 0，`:526-528`） | 上游 usage、或 `mod_body_process` 逐帧累加 |
| `ai_total_tokens` | `usage.UsedQuota`（`:532`） | **`UsedQuota` 只有三条赋值路径**：上游 usage 解析（`mod_ai_token_auth.go:283,289`）、估算计费（`:368`）、批任务结算（`batch_quota.go:326`） |

于是凡"有 in/out、没走上面三条路径"的请求，就得到 `in>0 / out≥0 / total=0` 的断裂三元组——正是 issue 现场看到的「输入 48、输出 226、总 token 为 0」。

### 1.2 流式 unrecognized 行

`mod_ai_token_auth.go:495-520`：`EstimateToken` 关闭或响应未完成/中止时，只把**计费副本** `billingUsage` 清零（#1398 起有意不动共享对象，避免抹掉统计），共享对象的 `UsedQuota` 从未被赋值为 `PromptTokens+CompletionTokens`。pb3 因此写出 total=0（`ai_usage_source=unrecognized`，`ai_zero_cost_reason=unrecognized` 已能标记原因，但**汇总仍漏量**）。

### 1.3 非 2xx 行（#1409 的刻意决策）

`mod_ai_token_auth.go:392-397`：`res == nil || res.StatusCode != 200` → `dropUnbilledEstimate`（`:666-708`）把共享 usage 的 `PromptTokens` / `CompletionTokens` / cache / audio / image / video / `UsedQuota` **全部清零**，并置 `UsageDispositionNone`；pb3 的门控（`request_log.go:521`）于是整组不写，只计 `AiUsageSuppressed`。

#1409 的理由是"非 2xx 行里的 in/out 是未经计费口径批准的估算残留，属统计污染"。代价是失败请求的用量彻底丢失，与本 issue 的验收标准直接冲突。而残留本可被行级标记区分（#1410/#1414 已建 `ai_usage_source`），抑制并非唯一手段。

### 1.4 既有不变式检查器

`usageTripleInconsistent`（`request_log.go:793-812`）判据为 `UsedQuota != PromptTokens + outputTokens`（image/video 与 `unrecognized`/`aborted` 旁路），计数器 `AiUsageInconsistent`。本 issue 的 A 口径与该不变式同向，可作为回归信号。

## 二、修复方案

### 步骤 1（必改）：`ai_total_tokens` 计算补齐（A）

文件：`bfe_modules/mod_access_pb3/request_log.go`，token 写法块（`:521-532`）。

```go
// A-01（issue #1480）：total = in + out，除非 UsedQuota 另有语义。
// UsedQuota 在 image_generation / video_generation 模式下是 image_count /
// video_count（mod_ai_token_auth.go:276-286），批任务由 batch_quota.go 单独结算，
// 这两类行必须保留原值；其余形态上一旦 UsedQuota 为 0 而 in/out 有值，
// 宁可写 in+out 也不要留下断裂三元组。
totalTokens := usage.UsedQuota
if totalTokens == 0 {
    totalTokens = usage.PromptTokens + outputTokens // outputTokens 为 clamp 后的值
}
reqLog.AiTotalTokens = proto.Int64(totalTokens)
```

要点：

- **不能无条件改成 in+out**：会破坏 per-image / per-video 的 count 语义与批任务结算。
- `outputTokens` 复用既有 clamp（`:526-528`），杜绝负数 total。
- 不变式检查器 `usageTripleInconsistent` 的**判据改为接收已写出的 `totalTokens`**（原为 `usage.UsedQuota`）：A 之后 `UsedQuota == 0` 而行内 total 为 in+out 是合法形态，若仍比对 `UsedQuota` 会把每一行补齐都计成不一致（`AiUsageInconsistent` 暴涨）。副作用是"in>0 而 total=0"这一旧形态不再由检查器捕获——它已被 A 按构造消灭；检查器继续守住"`UsedQuota` 非 0 但与 in+out 对不上"的真实断裂。

### 步骤 2（必改）：非 2xx 记录用量（B-记录）

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`。

1. **`dropUnbilledEstimate`（`:666-708`）改名 `keepNon2xxUsage`，语义反转**：
   - 不再清零共享 `TokenUsage`（in/out/cache/audio/image/video/`UsedQuota` 原样保留，供 pb3 写出）；
   - `SetUsageDisposition(bfe_basic.UsageDispositionObserved)`；
   - `SetUsageSource(bfe_basic.UsageSourceFailed)`；
   - `UsedQuota` 保持 0（非 2xx 不计费），由步骤 1 的 in+out 补齐；
   - **真实 usage 落在非 2xx 上**（既有 `IsFinalUsageSeen()` 分支 `:679-687`）：保留观测并照实写出，`SetUsageSource(UsageSourceParsed)`，`Non2xxRealUsageKept` 计数保留；
   - **无残留可写**（`usage == nil`，或 `PromptTokens==0 && CompletionTokens<=0 && UsedQuota==0`）：保持字段不写（"从未参与模型调用"与"记录为 0"仍可区分），不做任何标记。
2. **计数器改名**：`Non2xxEstimateDropped` → `Non2xxUsageReported`（`:82-87` 定义与注释、`:706` 自增点）；语义从"丢弃估算残留"变为"记录了非 2xx 用量"。发布说明单列指标名变更（无外部看板依赖可接受；如需零中断可保留旧名 + 新增语义计数器，二选一，推荐改名）。
3. **`UsageDispositionNone` 保留**：它现在只表达"没有可上报的用量"——非 2xx 且既未播种也未累加（如鉴权期就被拒的 401/429）仍置 `None`，访问日志据此保持三字段缺席，"未参与模型调用"与"记录为 0"仍可区分。`AiUsageSuppressed` 计数据此保留原语义（计这类行），**不 deprecated**。
4. **调用点注释同步**：`tokenRequestFinishHandler`（`:392-397`）改为"非 2xx 记录用量但不计费"。

### 步骤 3（必改）：新增 `ai_usage_source=failed` 常量

文件：`bfe_basic/request_ai_basic.go`，`UsageSource*` 常量块（`:400-429`）：

```go
// UsageSourceFailed means the upstream call never succeeded (non-2xx,
// including BFE-generated rejections): the reported tokens are the
// auth-time prompt seeding plus the accumulated forwarded content, i.e.
// an estimate kept for reporting but never billed (issue #1480). A non-2xx
// response that does carry a real usage object is reported as parsed.
UsageSourceFailed = "failed"
```

### 步骤 4（必改）：pb3 的 cost 门控（实现时补充）

文件：`bfe_modules/mod_access_pb3/request_log.go`。

token 块的门控（`usage != nil && 处置 != None`）**保持不变**——步骤 2 已把"有用量"的非 2xx 行翻成 `Observed`，无需改门控。但同一块内 `reqLog.AiCostValue = proto.Int64(usage.UsedCost)` 原本对"进入该块的每一行"都写，非 2xx 行进入后就会写出 `ai_cost_value=0`，把"从未计费"伪装成"计费 0"。因此补一条 2xx 判定：

```go
// issue #1480: 非 2xx 行永远不计费，cost 必须缺席；
// 2xx 行沿用 #1414 的"每个观测行都写 cost（含 0）"口径。
if res == nil || (res.StatusCode >= 200 && res.StatusCode < 300) {
    reqLog.AiCostValue = proto.Int64(usage.UsedCost)
}
```

`res == nil` 时保持写（与改动前一致，避免影响无响应对象的既有形态）。

## 三、兼容性与发布顺序

- **纯日志口径变更**：无配置字段、无 Redis key、无 DDL、无 pb 升版；bfe 单独发布即可，不需要 ai-gateway-api / log-reader / 报表侧配合（报表聚合均为 `SUM(ai_total_tokens)`，写出口径一改即正确）。
- **`ai_cost_value` 口径**：非 2xx 行现在明确**不写** cost（步骤 4），因此"失败行"在报表侧可用 `ai_cost_value IS NULL` + `res_status_code` 识别；MySQL 侧 log-reader 对数值列零值直通仍会落 0，属既存分叉（见 §五.3）。
- **报表口径影响**：
  - 非 2xx 行从"三字段缺失/全 NULL"变为"in/out/total 有值、`ai_cost_value` 仍缺失"——按 total 汇总会**上升**，属本 issue 的预期效果；
  - unrecognized / 中止行 total 从 0 变为 in+out；
  - 报表若需"仅真实用量"，过滤条件为 `res_status_code=200`（`ai_usage_source` 目前**只存在于 pb 日志**，四个报表后端无该列，不能作为筛选维度）。
- **与 #1409 的关系**：本卡**反转** #1409 的"非 2xx 抑制"决策，理由与代价见 §五；`2026-10-11-issue-1409-non2xx-estimate-token-residue-fix` 的历史文档不改写，仅在本目录记录映射（仓内既有约定）。
- **回滚**：版本回退即可；已写出的行不回改，口径切换点需在报表说明中标注。

## 四、回归测试

### 单元测试

`bfe_modules/mod_access_pb3/request_log_test.go`：

| 用例 | 处理 |
| --- | --- |
| `TestReqAiInfoGenSuppressesUsageOnNon2xx`（`:990`） | 改写为 `TestReqAiInfoGenRecordsUsageOnNon2xx`：非 2xx 行写出 in/out、`total=in+out`、`ai_usage_source=failed`、`ai_cost_value` 缺失、`usageSuppressed=false` |
| `TestReqAiInfoGenUsageSource`（`:1085`） | 表驱动中 `"non-2xx suppresses the source too"` 一行改为 `"non-2xx reports source=failed"` |
| `TestReqAiInfoGenDetectsInconsistentUsageTriple`（`:1039`）、`TestReqAiInfoGenDetectsInconsistentTripleWithoutTotal`（`:1156`） | 保留；补"total 由 in+out 补齐后不再触发 `AiUsageInconsistent`"断言 |
| **新增** `TestReqAiInfoGenTotalFallsBackToInPlusOut` | 表驱动：`{UsedQuota:0,in:10,out:4,unrecognized}`→14；`{UsedQuota:0,in:10,out:-1}`→10（哨兵 clamp）；`{UsedQuota:150,in:100,out:50}`→150（不变）；`{ImageCount:2,UsedQuota:2}`→2；`{VideoCount:3,UsedQuota:3}`→3 |
| **新增** `TestReqAiInfoGenNon2xxWithoutResidueKeepsFieldsAbsent` | in/out/UsedQuota 全 0 → 三字段仍不写 |

`bfe_modules/mod_ai_token_auth/`：

| 用例 | 处理 |
| --- | --- |
| `TestTokenRequestFinishHandler_Non2xxDropsEstimateResidue`（`mod_ai_token_auth_test.go:2918`） | 改名 `..._Non2xxKeepsEstimateResidue`：共享 usage 未清零、`UsageDisposition=Observed`、`UsageSource=failed`、`Non2xxUsageReported` +1 |
| `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation`（`:2962`） | `UsageSource=parsed`、`Non2xxRealUsageKept` +1、`Non2xxUsageReported` 不增 |
| **新增** `TestTokenRequestFinishHandler_Non2xxNoResidueNoCounter` | 非 2xx 且 usage 全 0 → 不清零、不标记、不计数 |
| 回归 | `TestTokenRequestFinishHandler_ObservedDispositionKeepsLogView`、`TestTokenRequestFinishHandler_ClientAbort*`（#1478）、`TestUpdateCtxByUsage*` |

### 集成测试（`bfe/tests/integration`）

| 场景 / 用例 | 现状断言 | 本卡处理 |
| --- | --- | --- |
| **SC05 TC19** `TestTC19_Non2xxNoUsageFields` | arm A（BFE 生成 404，有播种残留）、arm B（上游 404）断言 token 字段**缺失** | 改写为 `TestTC19_Non2xxUsageRecordedWithFailedSource`：两臂断言 `ai_input_tokens>0`、`ai_output_tokens≥0`、`ai_total_tokens=in+out`、`ai_usage_source=failed`、`ai_cost_value` 缺失；arm C（200 估算）保持 total=in+out |
| **SC05 TC21** `TestTC21_ZeroCostRowsExplicitCostAndReason` | 非 2xx 臂只断言 `ai_cost_value` 缺失 | 补 `total=in+out`、`source=failed`、`zero_cost_reason` 缺失 |
| **SC29 TC06** `TestTC06_CleanEofEstimateTokenOffStaysZeroWithSignal` | `ai_total_tokens == 0` | 改为 `total=in+out`，仍断言**未扣费**、`source=unrecognized` |
| **SC29** 上游断流臂（`sc29_upstream_drop_test.go:173-200`） | `source=unrecognized`、零计费 | 补 `total=in+out` |
| SC29 TC01~TC05 | 已断言 `total=promptEst+completionEst`（#1398 镜像） | 原样回归（确认 A 未改变已修形态） |
| **SC28**（batch）、SC30（nested choice）、SC11（billing fix）、SC12（client abort）、SC03（RMB 配额） | 计费与 total 相关断言 | 全量回归；SC28 专门锁定 per-image/per-video/batch 的 total 语义不变（A 的护栏） |
| SC21（traffic mirror）、SC22（intent） | 明细行断言 | 冒烟回归 |

运行建议：SC05 → SC29 → SC12/SC11 → SC28/SC30 → SC03；跑前删除 `.integration-test-bin` 缓存。

## 五、边界与残余风险

1. **口径反转需产品确认（已在本卡确认）**：非 2xx 行的 `ai_input_tokens` 是**估算残留**（`req_body_len/4`），不是计费用量。本卡后"按 total 汇总"的含义是"含失败行估算残留"；需要严格真实用量时按 `res_status_code=200` 过滤。
2. **`ai_usage_source` 未入仓**：新增的 `failed` 仅存在于 pb 日志（报表四后端无该列），因此报表侧区分失败行只能靠 `res_status_code` / `ai_cost_value` 缺失 / `err_code`。若需按来源筛选，另开卡加列（api DDL + log-reader + Doris/StarRocks/ClickHouse + 前端）。
3. **MySQL 侧 `ai_cost_value` 落 0**：log-reader 对该列是数值直通（零值写 0），"未计费"在 MySQL 明细表现为 0 而非 NULL（与 Doris 的 NULL 存在既存分叉）；不影响本卡 SUM 口径，可在 log-reader 侧独立处理。
4. **客户端中止 + `EstimateToken=false`**：属 unrecognized 分支，本卡后报表出现"未计费但有用量"的行；需与 #1478 的"中止按估算计费（`aborted` + 有 `ai_cost_value`）"在文档口径上区分。
5. **计数器改名**：`Non2xxEstimateDropped` → `Non2xxUsageReported` 会打断既有看板/告警（若已配置）；发布说明须单列。
6. **`AiUsageSuppressed` 语义收窄**：该计数现在只覆盖"非 2xx 且无任何可上报用量"的行（鉴权期即被拒的 401/429 等）。原先它还覆盖"有残留的非 2xx 行"，这部分行现在计入新增的 `Non2xxUsageReported`；两个计数之和大致等于改动前的 `AiUsageSuppressed`，看板/告警阈值需按此调整。

## 六、部署与配置说明

- 无新增/变更配置项；`EstimateToken` 的既有语义不变（仍决定"能否按估算计费"，与"是否记录 token"解耦）。
- 灰度建议：先在一个集群的 bfe 实例上升级，观察 `AiUsageInconsistent` 不上升、`AiUsageSuppressed` 降至近 0、`Non2xxUsageReported` 有量且与 4xx/5xx 请求数同量级，再全量。

## 七、实施记录（2026-10-31）

### 7.1 改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | 新增 `UsageSourceFailed = "failed"`（`UsageSource*` 常量块） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `dropUnbilledEstimate` → `keepNon2xxUsage`（保留残留、真实 usage 照实上报、`UsageSource=failed/parsed`、`Non2xxUsageReported` 计数）；计数器 `Non2xxEstimateDropped` → `Non2xxUsageReported`；调用点与注释同步 |
| `bfe_modules/mod_access_pb3/request_log.go` | `ai_total_tokens` 在 `UsedQuota == 0` 时回退为 `in + clamped(out)`；`ai_cost_value` 仅 2xx 写出；`usageTripleInconsistent` 改为比对已写出的 total；门控与 `usageSuppressed` 分支保留（语义收窄） |
| `bfe_modules/mod_access_pb3/mod_access_pb3.go` | `AiUsageSuppressed` 注释改为"无任何可上报用量的行" |
| `bfe_modules/mod_access_pb3/request_log_test.go` | 改写 `TestReqAiInfoGenSuppressesUsageOnNon2xx` → `...RecordsUsageOnNon2xx`；新增 `...Non2xxWithoutResidueKeepsFieldsAbsent`、`...TotalFallsBackToInPlusOut`（6 个表驱动子用例）；`TestReqAiInfoGenDetectsInconsistentTripleWithoutTotal` → `...TripleWithoutTotalIsFixedByFallback`；`UsageSource` 表补 `failed` 行 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | `..._Non2xxDropsEstimateResidue` → `..._Non2xxKeepsEstimateResidue`；更新 `..._Non2xxKeepsRealUsageObservation` 的处置/source/计数断言；新增 `..._Non2xxNoResidueNoCounter` |
| `bfe/tests/integration/.../SC05` | `TestTC19_Non2xxNoUsageFields` → `TestTC19_Non2xxUsageRecordedWithFailedSource`；新增 `assertFailedRowUsage` 辅助；TC21 的 arm 3 改用该辅助 |
| `bfe/tests/integration/.../SC29` | TC06 与上游断流臂补 `total = in + out` 断言（TC06 为 `EstimateToken=false`，三元组仍为 0/0/0，只断言内部一致） |

### 7.2 验证结果

单测（非 integration 包）：

```
go test ./bfe_basic/... ./bfe_modules/... ./bfe_config/... ./bfe_http/...         ./bfe_server/... ./bfe_util/... ./bfe_route/... ./bfe_balance/... -count=1   → 全部通过
go vet ./bfe_basic/... ./bfe_modules/...                                              → 通过
```

集成 SC（`bfe/tests/integration`，逐个场景）：

| 场景 | 结果 |
| --- | --- |
| SC05 access-log-ai-fields | ok（26.1s，含改写后的 TC19/TC21） |
| SC29 usage-snapshot-billing | ok（18.1s，含 TC06 与断流臂） |
| SC30 nested-choice-usage-billing | ok（3.9s） |
| SC11 ai-token-auth-billing-fix | ok（2.8s） |
| SC12 client-abort-billing | ok（9.3s） |
| SC28 ai-batch-support | ok（13.1s，per-image/per-video/batch 的 total 语义未变） |
| SC03 rmb-quota | ok（26.7s） |
| SC21 traffic-mirror / SC22 ai-intent-routing | ok（10.5s / 6.2s，冒烟） |

### 7.3 实施期间与初稿的偏差（已在正文同步）

1. **`usageTripleInconsistent` 必须改签名**：初稿写"不改"，实测发现它比对的是 `usage.UsedQuota`；A 之后 `UsedQuota==0` 而 total=in+out 是合法形态，会逐行报不一致。已改为比对已写出的 total（§二.步骤 1）。
2. **`ai_cost_value` 需要 2xx 门控**：token 块现在会被非 2xx 行进入，若照旧写 cost 会输出 `ai_cost_value=0`（把"未计费"伪装成"计费 0"）。已补 `res` 2xx 判定（§二.步骤 4）。
3. **`UsageDispositionNone` 与 `AiUsageSuppressed` 保留**：初稿打算让 `None` 失去生产者，但"非 2xx 且无任何用量"的行仍需要一个"无用量"标记，否则 token 字段会被写成 0/0/0。最终只把"有残留"的非 2xx 行从 `None` 翻成 `Observed`，计数语义收窄（§二.步骤 2、§五.6）。
4. **SC29 TC06 的 `in > 0` 断言不成立**：该场景 `EstimateToken=false`，鉴权期不播种 prompt，三元组本就是 0/0/0；只断言 `total == in + out` 且未扣费，A 的正面用例由 pb3 单测与 SC05 非 2xx 臂承担。