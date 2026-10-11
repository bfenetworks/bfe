# BFE Issue #1410 修复方案：usage 未识别 / 计费跳过的可观测性信号

- Issue: https://github.com/bfenetworks/bfe/issues/1410
- 缺陷：BFE 在「上游 usage 未识别（无 final usage）且不满足估算计费条件」时，按既定口径清零计费副本并跳过 RMB 扣减——**跳过分支本身全链零信号**：无计数器维度、无 Warn 日志，访问日志只能以 `ai_total_tokens=0` 表达，与"真实零用量请求"同形；当估算镜像生效（`tokenUsage.UsedQuota` 被回写估算值）时，"漏计费"与"估算计费"在日志上亦不可区分。结果是计费旁路只能靠带外人工对账脚本发现（生产实测单日缺口 95.5%，736 行无任何产品侧信号）。
- 代码库：`bfe/`（行号基于当前 HEAD，2026-10-11 逐条核实）
- 重要说明：issue 描述的"仅 4 个计数器"基于诊断时点代码，**已过时**。当前 HEAD 在 #1408 / #1409 后已有 7 个计数器（`ReqTotal`/`ReqAuth`/`ReqAuthFail`/`PriceLookupMiss`/`PriceFallbackUsed`/`Non2xxEstimateDropped`/`Non2xxRealUsageKept`），且 #1409 已建立两条可直接复用的信号模式：
  - **计数器 + Warn**：`dropUnbilledEstimate`（`mod_ai_token_auth.go:420-463`）对"非 2xx 估算残留"做了 `Inc(1)` + `log.Logger.Warn`（含 status/cluster/key_id）；
  - **逐请求日志标记**：`ai_price_miss`（`MarkPriceMiss()`/`IsPriceMiss()`，`request_ai_basic.go:171-181`；proto 字段 763，`request_log.go:482-488`）。
- 关联：#1398 / SC1401-TC022（missing usage 必须可观测的产品条款，本方案的信号形态落地）、#1406（gzip 漏计费实例）、#1408（价格缺失信号先例）、#1409（disposition 机制与非 2xx 信号先例）、#1352（客户端中止零扣费的既定口径）。本 issue 是 P2 观测性/结构性缺口：**不修改计费口径**，只补齐"该分支必须可观测"（issue 明确不主张不可估算分支应当计费）。

## 一、根因（当前 HEAD 逐条核实）

### 1. 守卫清零分支体内无信号

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:341-364`：

```go
billingUsage := *tokenUsage
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    billingUsage.PromptTokens = 0
    ... // 11 个计费字段全部置 0（计费副本）
    billingUsage.UsedQuota = 0
}
if billingUsage.UsedQuota <= 0 && estimateBillable {
    billingUsage.UsedQuota = CalcReqUsedQuota(...)
}
if estimateBillable && tokenUsage.UsedQuota <= 0 && billingUsage.UsedQuota > 0 {
    tokenUsage.UsedQuota = billingUsage.UsedQuota // 估算镜像回写共享对象
}
```

守卫体内（`:343-355`）既无 `m.state.*.Inc` 也无 `log.Logger.Warn`——"因 usage 未识别且不可估算而跳过计费"不可计数、不可告警。对 `bfe_modules/mod_ai_token_auth/` 全量检索 `state\..*Inc|log\.Logger\.(Warn|Errorf)` 逐条核对，命中点仅为：鉴权失败、扣减失败（`:386/:393`）、价格缺失/回退（#1408）、非 2xx 残留清理（#1409），**均不覆盖本分支**。

### 2. 计数器全集无该维度

`mod_ai_token_auth.go:47-74` 的 `ModuleAITokenAuthState` 共 7 个计数器，无任何"usage 未识别 / 计费跳过"维度。计数器经 `m.metrics.Init(&m.state, ...)`（`:89`）反射自动导出到 `monitorHandlers`（`:515-521`），因此**新增一个 state 字段即同时获得监控读数与 `.diff` 增量接口**，无额外接线成本。

### 3. 访问日志无法区分三种形态

`bfe_modules/mod_access_pb3/request_log.go:441-498`：

- disposition 只有 `none`（非 2xx 抑制，#1409）与 `observed`（2xx 照实报告）两态，**没有 usage 来源维度**——"未识别 / 已估算 / 已解析"三者同形；
- `AiTotalTokens = usage.UsedQuota`（`:454`）：未识别分支下共享对象 `UsedQuota` 为 0（镜像只在 `estimateBillable` 时回写 `:359-364`），日志与真实零用量请求同形；
- 镜像生效时 `total` 记估算值（>0），"漏计费"行与"按估算计费"行同形；
- 既有矛盾签名检测 `usageTripleInconsistent`（`:593-601`）**仅在 `UsedQuota > 0` 时才判**，`total=0 且 in/out>0` 这一唯一剩余判据检测不到；
- 日志中没有任何字段声明 usage 来源，解读跨字段矛盾需要外部先验（鉴权期按请求体播种 `PromptTokens` 的规则）。

### 4. 判据来源已存在，缺的只是出口

`bfe_basic/request_ai_basic.go` 已有三个独立布尔状态：`IsFinalUsageSeen()`（`:167`）、`IsResponseCompleted()`（`:160`）、`IsAllowEstimateToken()`。守卫判定 `:343` 完全由它们组合而成——**"识别结果"本身可取，缺的是把它暴露为信号**。这决定了本方案不需要任何新的运行时检测逻辑，只做"映射"。

### 5. 结构前提

`calcChatCost`/`calcCostUnits` 按子 token 字段计费、不读 `UsedQuota`（`mod_ai_token_auth.go` 计费函数族），故 `UsedQuota` 只剩展示值与（镜像前的）隐式哨兵两重语义（issue A-02）。内部判定已用显式的 `finalUsageSeen`，真正撕裂发生在**日志侧**：一个取值要同时表达"展示值"与"是否被识别"。

## 二、修复步骤

设计原则：

1. **复用 #1408 / #1409 已建立的两条信号模式**（计数器 + Warn；逐请求日志标记 `ai_price_miss` 同构的 `ai_usage_source`），新信号与既有先例形态一致，监控接入与对账脚本改造成本最低。
2. **不改计费口径**——守卫清零与跳过的既定行为保持原样（issue 明确不主张该分支计费）。
3. **必须能区分"未识别"与"真实零用量"**：真实零用量请求会走解析成功路径（上游返回 usage 全 0 仍会被解析并 `MarkFinalUsageSeen()`），与守卫分支互斥——这是本方案区分性的结构性保证。

### 步骤 1（必改）：守卫块计数器 + Warn

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`。

`ModuleAITokenAuthState`（`:47-74`）新增：

```go
// UsageUnrecognized counts 2xx requests whose upstream usage was never
// recognized (no final usage parsed) and which are not billable by
// EstimateToken (estimate disabled, or response not completed). Their
// billing fields are zeroed and no RMB deduction happens. Together with
// the per-request ai_usage_source access-log marker this is the
// product-side signal for the billing bypass (issue #1410); a genuine
// zero-usage request parses successfully and never lands here.
UsageUnrecognized *metrics.Counter
```

守卫块 `:343-355` 体内追加（与 `dropUnbilledEstimate` 的 warn 形态对齐）：

```go
m.state.UsageUnrecognized.Inc(1)
log.Logger.Warn("%s: usage unrecognized and not estimatable, billing skipped: cluster=%s key_id=%s model=%s response_completed=%v allow_estimate=%v",
    m.name, req.Route.ClusterName, ctx.aiBasicInfo.ClientKeyId, ctx.aiBasicInfo.TargetModel,
    ctx.aiBasicInfo.IsResponseCompleted(), ctx.aiBasicInfo.IsAllowEstimateToken())
```

要点：

- 计数器**只在判定成立处**自增，与计费跳过严格同源，不存在"计了费但报跳过分支"或反向分叉的可能；
- Warn 携带 `response_completed` / `allow_estimate` 两个子判据，直接区分"估算未启用"与"响应未完成（如客户端中止、流式中断）"两种成因，免回源码即可初判根因；
- 计数器经 `metrics.Init` 反射自动导出（含 `.diff`），可立即配阈值告警。

### 步骤 1b（建议，同模式扩面）：客户端中止跳过计费分支

`:319-323` 的既定口径（#1352）：`aborted && !IsFinalUsageSeen()` → 置 `deducted` 直接返回，**同样零扣费、零信号**。该形态属于"usage 未识别且不可估算"的一种（成因固定为客户端中止），建议在同一改动中按同模式补 `UsageUnrecognized.Inc(1)` + Warn（warn 中已有 `aborted` 语义可加注）——issue 已将此形态列为辅助观测点。若担心 warn 量随客户端中断率放大，可对该分支只计数、不 warn（counter-only），由计数器承担趋势观测。

### 步骤 2（必改）：usage 来源标记 `ai_usage_source`（访问日志）

目标：使"未识别 / 已估算 / 已解析"三态在日志上成为一等公民字段，对账不再依赖跨字段矛盾签名或外部先验。

**2.1 `bfe_basic/request_ai_basic.go`**：新增与 `usageDisposition`（报告/抑制开关，#1409）**正交**的来源维度（不扩展 disposition 取值，避免改动 #1409 语义与既有测试）：

```go
// Usage sources for the access log (issue #1410): where the reported
// token usage came from. Orthogonal to UsageDisposition (which decides
// whether usage is reported at all).
const (
    UsageSourceParsed        = "parsed"        // final usage parsed from upstream response
    UsageSourceEstimated     = "estimated"     // EstimateToken estimate, billing-approved
    UsageSourceUnrecognized  = "unrecognized"  // no final usage and not estimatable: reported values are auth-time seeding residue, billing was skipped
)

func (aiinfo *AiBasicInfo) SetUsageSource(source string) { aiinfo.usageSource = source }
func (aiinfo *AiBasicInfo) UsageSource() string          { return aiinfo.usageSource }
```

**2.2 `mod_ai_token_auth.go` `tokenRequestFinishHandler`**：在 `:341-364` 判定区一次设置（与守卫判定同源）：

- `IsFinalUsageSeen()` → `UsageSourceParsed`（含真实零用量：解析成功即 parsed，与守卫分支结构性互斥）；
- `!final && estimateBillable` → `UsageSourceEstimated`（镜像回写分支，`:359-364`）；
- `!final && !estimateBillable` → `UsageSourceUnrecognized`（守卫分支，与步骤 1 计数同点）。

非 2xx 出口由 `dropUnbilledEstimate` 置 `UsageDispositionNone`，访问日志整组字段缺席（#1409），`usageSource` 保持 unset、不写出——`unrecognized` 因此不会与非 2xx 形态混淆。

**2.3 `bfe-access-pb`（独立仓库）**：`bfe_access_pb/bfe_access.proto` 新增字段（763 之后空闲号，AI 计量字段段 761-790 内连续）：

```proto
// Usage source marker (issue #1410): "parsed" (final usage from upstream
// response), "estimated" (EstimateToken estimate approved for billing) or
// "unrecognized" (no final usage and not estimatable; billing skipped, the
// reported values are auth-time seeding residue). Absent when the upstream
// call never succeeded (non-2xx). Makes billing bypasses and estimate-billed
// rows directly queryable in reconciliation.
optional string ai_usage_source = 764;
```

同步 regenerate `bfe_access.pb.go`、`docs/protobuf.md` 字段表、`CHANGELOG.md`（与 `ai_price_miss = 763` 的登记方式一致）。

**2.4 `bfe_modules/mod_access_pb3/request_log.go`**：在 disposition == observed 分支（`:443` 起）写出：

```go
if source := aiInfo.UsageSource(); source != "" {
    reqLog.AiUsageSource = proto.String(source)
}
```

`unrecognized` 行**照常报告数值字段**（#1398 口径：missing usage 可见，绝不静默抹除），但带 `ai_usage_source=unrecognized`——原矛盾签名（`total=0 且 in/out>0`）仍在，但不再需要外部先验即可识别。`usageSuppressed`（非 2xx）行保持字段缺席，新字段亦缺席。

对账查询直接化：`ai_usage_source='unrecognized'`（或 `IN ('unrecognized') AND ai_cost_value IS NULL/0`）即漏计费候选集；`'estimated'` 与 `'parsed'` 区分估算计费与真实计费。

### 步骤 3（本 issue 不实施，仅记录）：`UsedQuota` 双语义的彻底消解

issue 候选③（`UsedQuota` 退化为纯展示值、另设独立哨兵）经核实现状后**降级为不必要**：

- 内部判定早已使用显式哨兵 `finalUsageSeen`（守卫 `:343`），不读 `UsedQuota`；
- 镜像回写造成的日志侧歧义（A-02 的实害）已由步骤 2 的 `ai_usage_source` 显式维度消解；
- 动 `TokenUsage` 结构（计费函数族、共享对象的所有读写方）收益不抵风险。

如未来再出现新哨兵语义需求，再做独立重构，不在本 issue 范围。

## 三、回归测试

单元测试（`testing`，放在被测代码旁）：

1. `bfe_modules/mod_ai_token_auth`（新增于 `mod_ai_token_auth_test.go` 或独立文件）：
   - `TestFinish_UsageUnrecognized_NotEstimatable`：2xx、`!IsFinalUsageSeen()`、`!estimateBillable` → 断言 `UsageUnrecognized == 1`、计费副本全 0、不扣减、`UsageSource() == Unrecognized`；
   - `TestFinish_UsageUnrecognized_RealZeroUsageNotCounted`：2xx、final usage 全 0（真实零用量）→ 断言计数器**不增**、`UsageSource() == Parsed`（区分性的正向断言，对应 issue 判据要求）；
   - `TestFinish_UsageUnrecognized_EstimateBilledNotCounted`：`!final`、`estimateBillable` → 断言计数器不增、`UsageSource() == Estimated`、镜像回写发生；
   - `TestFinish_ClientAbortCounted`（若实施 1b）：`ErrCode=ErrClientReset` + `!final` → 断言计数器 +1（既定跳过口径不变，只新增信号）；
2. `bfe_modules/mod_access_pb3/request_log_test.go`：
   - source 为 `parsed`/`estimated`/`unrecognized` 三态 → `AiUsageSource` 写出且取值正确；
   - `UsageDispositionNone` → `AiUsageSource` 缺席（与非 2xx 抑制一致）；
   - unset（未走 token 鉴权的老请求）→ 字段缺席，旧消费方无感。
3. `bfe-access-pb`：proto 生成物与 `docs/protobuf.md` 字段号/类型一致。

集成测试（`bfe/tests/integration`，SC03 计费场景，对应 issue"修复后应补的 TC"要求）：

4. `sc03_rmb_quota_test.go` 新增 `TestTC23_RMBQuotaDeduction_UsageUnrecognizedSignal`：
   - 构造 2xx 但 usage 不可解析且不可估算的请求（如：EstimateToken 关闭 + 无 usage 字段的上游响应），断言：RMB 不扣减（既定口径）、`mod_ai_token_auth.UsageUnrecognized` 计数 +1（经 monitor 接口读数或模块 state）、访问日志（SC05 场景可复用断言）`ai_usage_source=unrecognized`；
   - 回归：`TestTC22_..._ChunkedGzip` 等既有用例保持通过（修复不触碰计费口径）。

验证命令：

```bash
cd bfe
go test ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_basic/
go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ -v
```

## 四、修复后正确路径（目标行为）

2xx、usage 未识别且不可估算的请求：

1. `tokenRequestFinishHandler`：守卫判定成立 → `UsageUnrecognized.Inc(1)` + Warn（含 cluster/key_id/model/response_completed/allow_estimate）→ 计费副本 11 字段清零 → 无 RMB 扣减（**口径不变**）；
2. `SetUsageSource(UsageSourceUnrecognized)`；共享 `TokenUsage` 保留观测值（#1398 口径）；
3. 访问日志：`ai_usage_source=unrecognized`，数值字段照实报告（矛盾签名仍在，但有一等公民字段，无需外部先验）；
4. 监控：`mod_ai_token_auth.UsageUnrecognized` 及 `.diff` 可直接配阈值/突增告警——计费旁路首次成为**产品自身可发现**的状态（A-01/A-03 满足）；已识别请求（`source=parsed`，含 `completion_tokens=0` 的零输出 final 事件）与估算计费（`source=estimated`）、计费跳过（`source=unrecognized`）三态可区分（A-02 满足；usage 全 0 对象的分类受限见第五节已知限制）。

## 五、边界与残余风险

- **usage 全 0 对象的分类受限（诚实边界，实施中确认）**：识别层的 `isguess` 判定为值-based（`UsedQuota > 0`，`mod_body_process` 的 `GetQuotaUsage` 与 `tokenReadResponseHandler` 的 `UsedQuota > 0` 门槛同构），usage 对象全部计数为 0 的响应无法与"无 usage"区分，此类行会计入 `unrecognized`（计费为 0 属正确结果，仅分类语义受限）。含零值分量但总量 > 0 的 final 事件（如 `completion_tokens=0`）由 #1398 认可为 final，正常归入 `parsed`。识别层 presence 检测（按 usage 键存在性判定）是独立的后续改造，不在本 issue（观测性）范围。
- **客户端中止 warn 量**（步骤 1b）：客户端中断率高的场景 warn 量上升，属观测收益而非故障信号；如实测噪音过大，降级为该分支 counter-only。
- **proto 依赖**：`ai_usage_source = 764` 需 `bfe-access-pb` 仓库先行合入并同步依赖；字段为 optional string，向后兼容，旧消费方与旧日志解析无感。
- **Warn 不采样**：高并发下 warn 逐条打印有 IO 成本；与 #1409 的 `dropUnbilledEstimate` warn 同一量级模型，可接受。若后续需要可加采样，但**计数器必须不采样**（对账依赖精确计数）。
- **不改变对外语义**：无新增配置项、无模块顺序调整、无计费金额变化、无转发行为变化；`observed`/`none` disposition 行为保持原样，仅新增计数器维度与一个新日志字段。
- **残余盲区（本 issue 范围外）**："usage 已识别却零扣费"（#1408 窗口期形态）仍不在本信号覆盖内——其判定点在配额计划状态而非 usage 识别，需该 issue 自身闭环后评估是否补"识别后停扣"维度。

## 六、实施记录（2026-10-11）

已按上述方案实施（Phase 3–7 全流程：sys_design 文档 → 代码 → 测试设计文档 → 集成测试 → 回归验证）。变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `usageSource` 字段 + `UsageSourceParsed/Estimated/Unrecognized` 常量 + `SetUsageSource/UsageSource` |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | state 新增 `UsageUnrecognized` 计数器（带注释）；守卫块判定抽为 `unrecognized` 变量、体内计数 + Warn（含 cluster/key_id/model/response_completed/allow_estimate）；**同一判定点**三态设置 usageSource；客户端中止分支（#1352 口径）counter-only 计数 |
| `bfe_modules/mod_access_pb3/request_log.go` | observed 分支写出 `AiUsageSource`（source 为空不写出；disposition none 行随整组 usage 字段缺席） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 新增 4 例：未识别信号、真实零用量不计数（区分性正向断言）、估算计费不计数 + 镜像回写、客户端中止计数 |
| `bfe_modules/mod_access_pb3/request_log_test.go` | 新增 `TestReqAiInfoGenUsageSource` 5 子例（三态写出 / unset 缺席 / non-2xx 抑制） |
| `bfe/go.mod` / `go.sum` | `require github.com/bfenetworks/bfe-access-pb v0.3.15`（实施期曾用临时 `replace => ../bfe-access-pb`，tag 进入模块代理后已移除） |
| `docs/zh_cn/sys_design/rmb_quota.md` | §7.5 新增 issue #1410 说明段（两类同源信号 + 三态语义 + 已知限制）；变更记录新增第 14 条 |
| `bfe-access-pb`（独立仓库，已推送 origin，commit `4a511d5`，tag `v0.3.15`） | `ai_usage_source = 764`（proto + 重新生成 pb.go + protobuf.md + CHANGELOG v0.3.15） |
| `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/TC-24-usage未识别跳过计费信号.md` | 新增（TC-23 已被 #1408 占用，故编号 TC-24）；含三臂设计与已知限制 |
| `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/场景说明.md`、`测试场景总体说明.md` | SC03 用例数 23→24，登记 TC-24（P0） |
| `tests/integration/implementation/scenario-SC03-rmb-quota/sc03_rmb_quota_test.go` | `testEnv` 捕获 monitor 端口；新增 monitor 计数器读取辅助（SNAKE_CASE 键）；新增 `TestTC24_RMBQuotaDeduction_UsageUnrecognizedSignal`（三臂：chunked text/plain 未识别复现 / 零输出对照 / 正常计费对照） |

实施中确认并回写文档的三点偏差（详见 TC-24 设计文档）：

1. **`Server.EstimateToken` 出厂默认 true**（`bfe_config/bfe_conf/conf_basic.go:120`）——"无 usage 的 JSON 200"会被估算计费（`source=estimated`）而非落入守卫分支；TC-24 复现臂改为 chunked + `text/plain`（不可整体解析、无终止事件 → `responseCompleted=false` → 不可估算）。
2. **usage 全 0 对象的识别层局限**——`isguess` 判定为值-based（`UsedQuota > 0`，`GetQuotaUsage` 与 `tokenReadResponseHandler` 同构），全 0 usage 无法与"无 usage"区分，归入 `unrecognized` 为已知限制（计费为 0 正确，仅分类受限）；对照臂采用 #1398 认可的零输出 final 事件（`completion=0, total=100`）。识别层 presence 检测为独立后续改造。
3. **Warn 无法文件断言**——集成框架 `StartBFE` 将 BFE stdout 直挂测试进程、模板无 `[log]` 段（无 exception.log）；集成层以计数器为信号基准，Warn 由运行日志可见（实测输出含 `response_completed=false allow_estimate=true` 全子判据）。

验证结果（2026-10-11）：

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（受影响包） | 通过，无告警 |
| 单测：`bfe_basic` / `mod_ai_token_auth`（含新增 4 例）/ `mod_access_pb3`（含新增 5 子例）/ `mod_body_process` / `mod_ai_route` / `bfe_util` / `bfe_server` | 全部通过 |
| 集成 SC03 全量（TC01–TC24） | 通过 |
| 集成回归：SC02 / SC05 / SC07 / SC12（abort 分支改动）/ SC29 / SC30 | 全部通过 |

未跑：`make test` 全仓（会拉起全部 ~30 个集成场景）；TC-24 的 `ai_usage_source` 日志字段断言由单测 `TestReqAiInfoGenUsageSource` 覆盖（SC03 未加载 `mod_access_pb3`），如需集成层断言可在 SC05 补用例。

残余待办：

- ~~`bfe/go.mod` 的临时 `replace`~~ 已处理：v0.3.15 进入模块代理后移除 replace 并升级 require（2026-10-11 当日完成，`go get` + `go mod tidy` 通过，构建与单测复验通过）。
- 识别层 presence 检测（usage 全 0 对象的分类）作为独立 issue 跟进。
- Git 提交与推送：**未执行**——按流程需人工确认提交内容后另行授权（`bfe` 仓库的变更尚未 commit）。
