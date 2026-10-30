# BFE Issue #1478 修复方案：客户端流式中断按已转发内容估算计费

- Issue: https://github.com/bfenetworks/bfe/issues/1478
- 缺陷：客户端在流式输出途中主动断开（如用户点"停止生成"）时，后端已生成并转发了一部分内容，网关对这次请求**零扣费**。实测 3 次均为：后端处理输入 37 token、生成 19 token 后随客户端断开停止，按定价约 0.000075 元，扣费都是 0。
- 关联：#1352（现行"客户端中断零扣费"设计，本方案在其安全侧改向）、#1398/#1414（估算与零计费信号链路）、#1474（上游断流计费，本方案与其口径对齐）。**本方案仅改 bfe，ai-gateway-api 不需要修改。**

## 一、现状与根因

### 1.1 现行行为：abort 分支无条件零扣费

`tokenRequestFinishHandler`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:423-439`）：

```go
aborted := isClientAbortErr(req.ErrCode)
if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
    m.state.AbortBillingSkipped.Inc(1)
    ctx.aiBasicInfo.SetUsageSource(bfe_basic.UsageSourceAborted)
    // ... RMB 计划：SetZeroCostReason(aborted) + BillingZeroCost ...
    ctx.deducted = true
    return            // 零扣费，不再进入通用计费路径
}
```

该分支是 issue #1352 的刻意设计：客户端断开时上游不会再返回最终 usage，而按"**全量请求**估算"会把后端未生成/未交付的部分也计费，因此干脆零扣费。

### 1.2 关键立论：按"已转发内容"估算不在 #1352 禁止的范围内

#1352 真正禁止的是**全量估算**（按整个请求的 prompt + 预期输出估算，会把未交付内容计费）。而本缺陷的测试建议是**按已转发内容估算**：

- 输入侧：请求体已被后端完整处理（实测 37 token 的输入是真实消耗）；
- 输出侧：`mod_body_process` 的 `QuotaUsageProcessor` 在流式过程中**逐 chunk 累加已转发事件的内容长度**（`content_quota_usage.go:111-122`），中断时这份累加值恰好等于"已转发给客户端的内容"——它天然是交付侧的估计，不含未生成部分。

也就是说，实现上**数据早已就绪**（鉴权期 prompt 播种 + 逐 chunk completion 累加都在共享 `TokenUsage` 上），只是 abort 分支把统一入口拦死为零。修复不需要新的估算数学，也不需要新标记——`isClientAbortErr(ErrCode)` 已是现成的判定依据（#1474 的 `upstreamDropped` 标记是因为上游断流与客户端中断在拷贝错误上不可区分，本问题不存在该问题）。

## 二、修复方案（仅 bfe）

### 步骤 1（必改）：abort 分支改为"可计费则下沉"

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`。

把现在的"abort 直接返回零扣费"改为前置分流：

```go
aborted := isClientAbortErr(req.ErrCode) && !ctx.aiBasicInfo.IsFinalUsageSeen()
if aborted && !ctx.aiBasicInfo.IsAllowEstimateToken() {
    // 估算关闭：保持现行零扣费 + 信号（#1414 语义不变）
    ... 现有分支体 ...
    return
}
// 估算开启：继续走通用计费路径，estimateBillable 纳入 abort 维度
```

计费门控（当前 `:464-466`）：

```go
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() &&
    (ctx.aiBasicInfo.IsResponseCompleted() ||
        ctx.aiBasicInfo.IsUpstreamDropped() ||
        aborted)
```

其余链路完全复用：`:521-528` 的 `CalcReqUsedQuota(prompt估算, completion累加)`（只含已转发内容）与 `:524` 起的估算镜像（`UsedQuota` 写回共享对象，pb3 `ai_total_tokens = in + out`，顺带消除 abort 行 `total=0` 断链——问题8 的"中止行 total 仍为 0"随之收口）。

### 步骤 2（必改）：信号分账

1. **计数**：新增 `AbortBillingEstimated`（估算计费的中断请求数，alertable）；`AbortBillingSkipped` 语义收窄为"中断且估算关闭、确实零扣费"——既有看板的"中断率"口径会下降，需同步调整（与 #1414 拆分 `aborted` 时的迁移模式一致）。
2. **usage source**：保持 `ai_usage_source=aborted`（它描述的是**断开原因**，不是计费结果；计费结果由 `ai_cost_value` 表达）。零扣费候选集的判定改用 `ai_zero_cost_reason=aborted`——该原因只在未计费分支置位，语义不再有歧义。
3. **zero cost reason**：计费成功的中断请求不置 `ai_zero_cost_reason`（与其它正常计费行一致）。

### 步骤 3：边界与明确不改的点

- **中断前已见最终 usage**：现状即按实际 usage 计费（`:424` 条件不命中），不变；
- **Non-2xx / AI cache 命中 / batch 契约**：各自分支在 abort 判定之前返回，不受影响；
- **双端同时断开**（客户端恰在上游异常时点停止）：按 #1474 的归因规则仍判为客户端中断（`clientClosed` 优先），但计费口径与上游断流一致——**只要已转发内容存在就按估算计费**。两个问题的口径由此完全对齐："非 2xx 不收费，2xx 且已交付内容按已交付部分收费"。
- **不计费的部分**：后端已生成但**未转发**给客户端的内容（中断后上游多出的 token）不计费——#1352 的实质保留。

## 三、ai-gateway-api：无需修改

- **扣费**：`QuotaPlan.Deduct` 由 bfe 直写 Redis（`:585` 起的扣减循环），API Key 详情的 used = `quota − remaining` 自动反映；
- **报表**：聚合为 `SUM(ai_total_tokens)` / `SUM(ai_cost_value)` 的直接求和（`bfe_ai_metrics_1m_job.sql:54`、`storage/mysqlreport/job.go:534`），对 `ai_usage_source` 无枚举白名单；`ai_usage_source`/`ai_zero_cost_reason` 均为字符串字段（pb3 764/765），**无 proto 升级、无 api 侧改动**；
- 可选增强（另立卡片，与本修复无关）：控制台按 `ai_usage_source=aborted` 提供"中断请求（计费/未计费）"的展示与筛选。若产品要求把口径做成**集群级可配开关**（而非默认生效），才需要在 ai-gateway-api 增加配置字段 + 校验 + 编译器下发（形态同 #1474 的 `StreamTruncatedError`）——属实现方式选择，见 §六。

## 四、兼容性与迁移影响

- **行为变更（默认生效）**：所有 `EstimateToken=true` 的集群上，客户端中断请求从"零扣费"变为"按已转发内容估算扣费"。这是本修复的目的本身，但属**产品口径变更**——上线前需产品确认（用户点"停止生成"后为已消费内容付费是否符合定价预期，见 §六）。
- **SC12（client-abort-billing）已改造**：原 5 个 TC 在 `EstimateToken = true` 下断言零扣费，修复后翻转——
  - TC-01 重写为计费臂（`TestTC01_ClientAbortBilledByForwardedContent`）：无 usage 帧序列 → 精确金额断言（prompt 播种 + 逐事件 `len/4`，含 #1474 注入的错误事件帧，见下）+ `AbortBillingEstimated` 断言；
  - TC-04 切到 `EstimateToken=false` 对照臂（新增 `newTestEnvWithEstimateToken(t, false)`：Build 后改写 bfe.conf），保留零扣费 + `AbortBillingSkipped` + `BillingZeroCost` 断言并增加 `AbortBillingEstimated` 不增；
  - TC-02（已见最终 usage）与 TC-03（完整流）不变。
- **实施细节（估算口径）**：客户端中断经 `cancelOnClientClose` 关闭上游 body 后，#1474 的 `streamTruncationInjector` 会观察为"中途断流"并补发一个协议错误事件帧；该帧进入 `mod_body_process` 的读取路径并被计入 completion 估算（约 33 token）。与 bfe SC29 TC-07/08 口径一致，属"按转发字节估算"的既定近似，TC-01 的精确金额已显式预算该帧。
- **计数基线**：`AbortBillingSkipped` 数值回落（只剩估算关闭集群），新计数 `AbortBillingEstimated` 接管；`BillingZeroCost` 相应不再包含计费成功的中断行。
- **无 DDL、无模块顺序调整、无配置迁移**；回滚 = revert 步骤 1-2 的置点。

## 五、回归测试

### 单元测试（`mod_ai_token_auth`）

1. abort + EstimateToken=true + 无最终 usage ⇒ 按 `CalcReqUsedQuota(prompt播种, 已转发累加)` 扣减；`ai_usage_source=aborted`；计 `AbortBillingEstimated`；`AbortBillingSkipped` 不增；共享 `TokenUsage.UsedQuota` 镜像非 0；
2. abort + EstimateToken=false ⇒ 零扣费 + `AbortBillingSkipped` + `ai_zero_cost_reason=aborted`（既有语义不变）；
3. abort + 已见最终 usage ⇒ 按实际 usage（既有语义不变）；
4. abort 与上游断流共存（同时标记）⇒ 计费口径一致、计数不双记；
5. 非 2xx / cache 命中 / batch 各分支不受影响。

### 集成测试

6. SC12 改造（见 §四）后全绿，含两条对照臂；
7. 相邻回归：SC03（RMB 扣减）、SC29（估算计费快照，含 #1474 TC-07~09 与 clean-EOF TC-04~06）、SC46（integration-test 的上游断流端到端）；
8. 端到端补一条 issue 原用例的复刻：vLLM/`ignore_eos` 流式读取 N 块后 RST，断言扣费 ≈ 已转发内容估算、`ai_usage_source=aborted`。

验证命令：`cd bfe && go test ./bfe_modules/mod_ai_token_auth/ ./bfe_server/` 与 `go test ./tests/integration/implementation/scenario-SC12-client-abort-billing/ -v`；提交前 `make test`。

## 六、残余风险与待决事项

- **产品口径（前置决策）**：#1352 的零扣费是刻意设计，本方案是**有意改向**——需产品确认"客户端中断按已转发内容计费"符合定价预期。若产品倾向保留零扣费，则本卡转为"控制台呈现 `ai_usage_source=aborted` 请求"的可观测性增强，不改计费。
- **计费方式选择**：默认生效 vs 集群级开关。建议默认生效（复用 `EstimateToken` 作为事实开关：估算开 = 计费；避免新增配置面）。若要求灰度，再评估 api 侧配置字段。
- **估算精度**：completion 为逐 chunk `len(data)/4` 的启发式估计（与 #1398/#1474 同一口径，非精确 tokenizer），与已转发内容线性相关，属既定近似。
- **中断后上游继续生成的部分**：不计费（无法观测、也不应计费），与 #1352 的实质一致。## 七、实施记录（2026-10-30）

已按本方案实施，两处与方案的偏差在下方标注。

| 文件 | 改动 |
| --- | --- |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | ① abort 前置分流：`abortedNoFinalUsage && !IsAllowEstimateToken()` 才走零扣费（原分支体保留，计 `AbortBillingSkipped` + `ai_zero_cost_reason=aborted`）；估算开启时下沉到通用计费路径。② `estimateBillable` 增加 `abortedNoFinalUsage` 维度。③ usage source switch 新增 `aborted` 计费分支（`AbortBillingEstimated` 计数）。④ 模块 state 新增 `AbortBillingEstimated`（反射初始化，无需注册） |
| `bfe_basic/request_ai_basic.go` | 仅注释更新：`UsageSourceAborted`（原因标记 ≠ 计费结果）、`ZeroCostReasonAborted`（仅未计费分支置位）；常量值不变 |
| `mod_ai_token_auth_test.go` | 3 处旧语义测试更新：`ClientAbortNoFinalUsage`（估算开→估算扣费；关→零扣费+原因）、`ObservedDispositionKeepsLogView` 与 `ClientAbortCounted`（切到估算关闭臂保留其原锁定语义） |
| `mod_ai_token_auth_client_abort_test.go`（新增） | `ClientAbortBilledMirrorsEstimate`（计费 + `UsedQuota` 镜像 + 无 zero-cost 原因）、`ClientAbortWithDropFlagsCountsOnce`（断流/中断标记共存只计一次） |
| `tests/integration/.../scenario-SC12-client-abort-billing/` | 新增 `newTestEnvWithEstimateToken(t,false)`（Build 后改写 bfe.conf）；TC-01 重写为计费臂（新函数 `TestTC01_ClientAbortBilledByForwardedContent`，无 usage 帧序列、精确金额 + `AbortBillingEstimated` 断言）；TC-04 切到估算关闭臂并断言 `AbortBillingEstimated` 不增；TC-02/03 不变 |
| `tests/integration/.../scenario-SC29-usage-snapshot-billing/` | TC-03 由"中断零扣费 + 日志保留分项"改为"中断按已转发内容估算计费 + 日志镜像估算（`in/out/total` 与 `ai_usage_source=aborted`）"——原前提（零扣费）在估算开启下已不存在，零扣费臂由 SC12 TC-04 覆盖。**偏差**：方案原写"SC12 保留零扣费覆盖需新增对照臂"，实施中将 SC29 TC-03 也一并改造为该口径的日志级断言 |
| `docs/zh_cn/configuration/bfe.conf.md` / `docs/en_us/configuration/bfe.conf.md` | `EstimateToken` 行为说明更新（三情形可估算：正常完成 / 上游断流 #1474 / 客户端中断 #1478；按已转发内容估算） |
| `docs/zh_cn/sys_design/rmb_quota.md` | 8 处：代码走查片段、§6.4 计费语义、§7.5 判定表拆两行、计数器说明、usage source 取值、中止分支拆分段、§10 测试建议 |
| `tests/integration/测试设计文档/`（SC12、SC29、总体说明） | TC-01/TC-03 文档重写并改名、TC-04 改为估算关闭臂、TC-02 补注、场景说明与总体说明同步 |

验证结果（2026-10-30）：

| 范围 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 全量单元测试（`go list ./... \| grep -v tests/integration`） | 全部通过 |
| `mod_ai_token_auth`（含新增 2 例） | 通过 |
| 集成 SC12 | 4/4 通过：TC-01 计费臂精确金额（14×452 + 57×2262）、TC-02/TC-03 原样通过、TC-04 零扣费臂双形态 + `AbortBillingEstimated=0` |
| 集成 SC29 | 6/6+3 通过（TC-03 改造后：估算计费 49500 单位、日志 `in=18/out=49/total=67`、`ai_usage_source=aborted`） |
| 回归 SC03 / SC05 | 通过 |

实施中发现并记录：客户端中断经 `cancelOnClientClose` 关闭上游 body 后，issue #1474 的 `streamTruncationInjector` 会将其观察为"中途断流"并补发一个协议错误事件帧（132 字节 → 33 token）；该帧进入 `mod_body_process` 读取路径并被计入 completion 估算。与 SC29 TC-07/08 口径一致，属"按转发字节估算"的既定近似，本卡测试的精确金额已显式预算该帧；如需剔除，应在估算器侧跳过协议错误事件（跨卡行为调整，不在本卡范围）。