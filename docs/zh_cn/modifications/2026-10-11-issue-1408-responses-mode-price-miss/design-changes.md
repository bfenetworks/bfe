# BFE Issue #1408 修复方案：/responses 入口 mode 无价格条目导致静默 0 计费

- Issue: https://github.com/bfenetworks/bfe/issues/1408（2026-10-11 人工提交，版本：bfe 1.8.8.2；issue 内容已更新为**已确认根因**）
- 缺陷：生产 pb3 访问日志（CST 2026-10-10 全日 8175 行）中，`ai_total_tokens > 0`（usage 识别正常）但 `ai_cost_value = 0` 的行共 285 行；逐行归因后仅 **13 行**是产品缺陷——全部来自 `api-key-13`、`cluster=bailian`、`ai_target_model=qwen3.7-plus`、`origin_uri=/responses`（OpenAI Responses API 入口）：13/13 为 HTTP 200、`ai_stream=1`、usage 完整（`total == in + out`）、RMB 币种，但 0 扣费；当日该 (cluster, model, mode) 组合 13 次请求 0 次计费，而**同集群其它模型的 chat 流量计费完全正常**（deepseek-v4.1-flash 2361 行 / 33.7358 元、qwen3.8-max 79 行 / 2.7726 元）——缺陷只落在 **mode 维度**。
- 代码库：`bfe/`（行号基于当前 HEAD：`v1.8.8` `d04b83a6`；该版本含 #1406/#1409 修复，issue 文中 1.8.8.2 的行号会比本文靠前约 20 行）
- 关联：**#1406**（gzip/chunked usage 断裂 total=0，不同根因，勿合并）；**#1382**（provider-native 路径 mode 识别，明确保留了「miss 即 0」验收决定）；SC3201-TC024 / SC1401-TC010
- 目录原名 `...-rmb-zero-billing-window-fix` 中的「window」源于 issue 初稿的「两个停计费窗口」推断，该归因已被撤回（见第三节），实际不存在时间窗口机制，故目录已更名为 `...-responses-mode-price-miss`。

## 一、285 行归因总表（issue 更新后的实锤数据）

| 分类 | 行数 | 性质 |
|---|---|---|
| `origin_uri` 含 `/count_tokens` | 39 | 按设计不计费（计数接口不进入计费收尾） |
| `err_code = CLIENT_WRITE`（客户端中断，`ai_output_tokens=0`） | 6 | 按设计不计费（bfe#1352） |
| 全部早于 RMB 定价配置生效时刻（2026-10-09 22:28:57 UTC 前） | 227 | 配置时点，非缺陷（定价文件 mtime 当日 21:55–21:58 +0800） |
| `POST /responses`（`ai_mode=responses`） | **13** | **本缺陷** |

当日全量：计费 6866 行 / 62.8700 元；`total>0 && cost=0` 共 285 行 = 39 + 6 + 240（其中 227 行配置时点 + 13 行本缺陷）。同一数据集内 3092 行（api-key-1）金额与 DeepSeek 闲时价逐行恒等，证明**价格配置与计费公式本身无缺陷**——缺陷是特定 (model, mode) 组合取不到价格。

## 二、确认的根因链（五环，全部源码/配置实锤）

1. **收尾只在「计划可扣」时算价**：`tokenRequestFinishHandler`（`mod_ai_token_auth.go:361`）`if billingUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) { billingUsage.UsedCost = m.calcCostUnits(...) }`，随后 `tokenUsage.UsedCost = billingUsage.UsedCost` 写回共享对象并进入扣减循环（RMB 分支仅 `costUnits > 0` 才 `plan.Deduct`）。⇒ 成本为 0 时全链路静默：不扣费、不报错、不写日志字段。
2. **价格是 (model, mode) 精确匹配、无回退**：`calcCostUnits`（`:696`）→ `LookupModelPrice(table, targetModel, mode)`（`cluster_conf_load.go:1563-1571`，`return idx[mode]`，索引按 `priceIndex[model][mode]` 构建）。`mode` 由请求路径推导（`/responses` → `ModeResponses`，`bfe_basic/openai_endpoint.go:42`）。
3. **部署的定价配置没有任何 responses 条目**：仓库 5 个 `model-price-*.yaml` 的 mode 取值只有 `chat`（bailian 16 个模型、kimi 1 个模型同）。⇒ `LookupModelPrice(qwen3.7-plus, "responses")` 必然返回 nil。
4. **miss 被显式设计为按 0 计费**：`:726` `m.state.PriceLookupMiss.Inc(1)` + `log.Logger.Warn("model price not found for cluster %s model %s mode %s", ...)` + `return 0`；`:53` 注释自陈这是 #1382 的验收决定（"never fall back to a wrong-mode price"），计数器被指定为唯一监控信号。
5. **访问日志无法区分「未定价 0」与「合法 0」**：`AiTotalTokens` 照常写（`mod_access_pb3/request_log.go:454`），`AiCostValue` 仅 `UsedCost > 0` 时写（`:480`）。⇒ 生产对账只能靠「有 usage 无 cost」反向特征事后发现。

净效应：一个网关实际注册并成功服务的入口（`/responses`，200 + 完整 usage + RMB 币种），每次请求都落在「计算出 0 成本」分支——既不扣费也不留可关联信号，且因 #1382 的有意设计**不会自愈**。

## 三、对初稿推断的撤回记录（防重复排查）

| 初稿推断 | 结论 |
|---|---|
| ① 窗口期该 Key 的 RMB 计划不可见 ⇒ `hasRMBPlan` 为假 | **撤回**：api-key-1 计费行与未计费行的 `ai_auth_hit_quota_plans` 完全相同（均 `["api-key-1"]`）；227 行刚好全在定价生效边界之前。仅对 api-key-13 单独看时仍未排除（故 issue 复现步骤给了「判定对照臂」） |
| ② `plan.Deduct` 失败被吞 ⇒ cost=0 | **撤回**：`UsedCost` 在扣减循环**之前**就已写回（`:361` 之后），Redis 扣减失败只会表现为「有 cost 余额未减」，永不表现为 `ai_cost_value=0` |
| ③ 两个停计费窗口（16:00–16:23 / 21:25–22:00） | **撤回**：那是定价生效边界（北京时间 06:28:57）之前仅有的两段有流量时段；边界后不存在「停计费窗口」。13 行真缺陷是 16:54–22:00 的一次性突发 |
| ④ `calcCostUnits` 返回 0 可能性低（因金额恒等） | **撤回**：金额恒等只约束 `(kimi, kimi-for-coding, chat)` 组合，对其它 (model, mode) 是否取得到价格无约束；**价格缺失正是已确认机制** |
| 保留 | `total>0 && cost=0` 等价于「成本从未算出」；`ActiveTierName` 未命中回落默认价，分时段 tier **不是**本缺陷机制 |

## 四、修复方案（已决策：方案 2「语义回退」，配套方案 1 观测性标记）

> issue 将修复方向明确为产品决策三选一。**本方案选定方案 2（语义回退）**，并以方案 1 的观测性标记作为必做配套（回落不得静默）。方案 1（仅标记）、方案 3（4xx 拒绝）未选，备查见文末。

### 步骤 1（必做，配套）：观测性标记——回落必须可被发现

- `log.Logger.Warn("model price not found ...")` 提升为带请求标识（apikey_id / cluster / model / mode / request id）的 WARN，确保能按请求关联；
- 保留 `PriceLookupMiss` 计数器（它记录的是「价格表配置缺口」，回落后仍应计数，作为运营补齐价格表的信号）；建议新增 `PriceFallbackUsed` 计数器记录「实际发生回落计费」的请求数；
- 访问日志落可区分标记 `ai_price_miss`（新增 pb3 字段，置于 `bfe-access-pb` 的 `RequestLog`，按该仓字段扩展流程：proto 变更 → 生成 → `mod_access_pb3/request_log.go` 写入 → 消费方升级）：

```go
// mod_access_pb3/request_log.go（示意，字段号按 access-pb 规范分配）
if aiInfo.PriceMiss { // mod_ai_token_auth 经 AiBasicInfo 带出标记
    reqLog.AiPriceMiss = proto.Bool(true)
}
```

- BFE 侧需在 `bfe_basic.AiBasicInfo` 新增 `PriceMiss bool` 字段，由 `calcCostUnits` 在「(model,mode) miss 但回落成功」与「miss 且无回落」两种情况下置位，供 access-log 阶段读取；
- 效果：对账 SQL 直接 `WHERE ai_price_miss = true` 即可列出全部「非显式定价」的计费行，持续暴露价格表缺口。

### 步骤 2（选定，治本）：(model, mode) 无条目时回落同 model 的 chat 价

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`calcCostUnits`（`:696`）的 `LookupModelPrice` miss 分支：

```go
entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, mode)
priceMissed := false
if entry == nil {
    // 语义回退：mode 无条目时回落同 model 的 chat 价（产品决策：未单独定价的 mode 与 chat 同价）。
    // 仅当 mode 本身非 chat 且 chat 条目存在时回落；chat 自身 miss 维持原行为（按 0 计费 + 计数 + WARN）。
    if mode != bfe_basic.ModeChat {
        if chatEntry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, bfe_basic.ModeChat); chatEntry != nil {
            entry = chatEntry
            priceMissed = true
        }
    }
    if entry == nil {
        m.state.PriceLookupMiss.Inc(1)
        log.Logger.Warn("model price not found for cluster %s model %s mode %s", clusterName, targetModel, mode)
        return 0
    }
}
if priceMissed {
    m.state.PriceFallbackUsed.Inc(1)
    aiMeta.PriceMiss = true
    log.Logger.Warn("model price fell back to chat price: cluster=%s model=%s mode=%s",
        clusterName, targetModel, mode)
}
```

要点：
- **产品决策已记录**：本改动**显式反转 #1382 的「never fall back to a wrong-mode price」验收决定**；采用后产品口径为「未单独定价的 mode 与 chat 同价」。若未来某 mode 需差异化定价，应为其在价格表补条目——回落是缺条目时的兜底，不是定价方案。
- **成本公式不受影响**：`calcCostUnits` 随后的 `switch mode` 按原 mode 分派（responses → `calcResponsesCost` → `calcChatCost`），回落只替换**价格条目**，输入/输出/缓存等价格键从 chat 条目按既有规则（含 `ActiveTierName` 分时段价）取价——分时段价对回落流量同样生效，无需额外处理。
- **回落不静默**：置 `PriceMiss` 标记 + `PriceFallbackUsed` 计数 + 带标识 WARN。
- `chat` 自身 miss 维持原行为（按 0 计费），由 `PriceLookupMiss` 暴露——chat 是该 model 的基准价，基准缺失属配置事故，应保持 fail-loud 而非自动归零之外的兜底。

### 未选方案备查

- **方案 1（仅标记不改金额）**：只消除静默（A-03），对 A-01「有 usage 必扣费」无直接作用，/responses 仍免费——不满足本次修复目标，未选；其观测性内容已并入步骤 1。
- **方案 3（未定价入口 4xx 拒绝）**：把 /responses 从 200 改为 4xx，可用性影响大且 issue 数据已证明入口有真实流量；未选。若未来出现「必须与 chat 区分定价且不能回退」的 mode，优先补价格表条目而非拒绝。

### 实现落点清单

| 仓库 | 文件 | 改动 |
|---|---|---|
| bfe | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `calcCostUnits` 回落逻辑 + `PriceFallbackUsed` 计数器 + WARN |
| bfe | `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `PriceMiss bool` |
| bfe | `bfe_modules/mod_access_pb3/request_log.go` | 写 `AiPriceMiss` |
| bfe-access-pb（跨仓） | `RequestLog` proto | 新增 `ai_price_miss` 字段（按字段扩展流程） |

## 五、回归测试

1. **mode 维度集成回归（SC03 或新场景，确定性 on-demand）**：临时 Cluster 的 `ModelTable` 只配 `{model: M, mode: "chat", prices: {...}}`，同一对照 Key（绑 RMB 有限计划，已验证 chat 可正常扣费）分别请求：
   - `POST /v1/chat/completions` → 断言扣费 = 用量 × 价格；
   - `POST /responses`（stream）→ **断言按 chat 价扣费**（扣减额 = 声明用量 × chat 价，同口径 `ai_cost_value`），且访问日志 `ai_price_miss=true`、`PriceFallbackUsed` +1；
   - 对照：`ModelTable` 完全无该 model 任何条目时 chat 请求仍 0 计费（`PriceLookupMiss` +1）——锁定「chat 基准不兜底」边界。
2. **对照臂（防误伤）**：`ai_auth_hit_quota_plans` 相同的前提下，同 Key 的 chat 请求计费不受回落改动影响（金额与改动前一致）。
3. **单元测试**：
   - `cluster_conf`：`LookupModelPrice` 精确匹配无回退回归锁定；
   - `mod_ai_token_auth`：`(responses, 无条目) → 回落 chat 价`；`(image, 无条目) 回落后仍取不到图片价格键 → 0 + 标记`；`(chat, 无条目) → 不回落、0 + PriceLookupMiss`；标记置位与计数器；
   - `mod_access_pb3`：`ai_price_miss` 序列化/反序列化用例。
4. **回归命令**：

```bash
cd bfe
go test ./bfe_config/bfe_cluster_conf/... ./bfe_modules/mod_ai_token_auth/... ./bfe_modules/mod_access_pb3/...
go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ -v   # mode 维度新臂 + 既有 TC01~TC22
```

## 六、边界与残余风险

- **#1382 验收的显式反转（已决策）**：行为由「miss 即 0」改为「miss 且同 model 有 chat 条目 → 按 chat 价计费」。产品上等同声明「未单独定价的 mode 与 chat 同价」；若某 mode 实际应差异化定价，必须补价格表条目，回落仅兜底。
- **以次数计价的 mode 回落无效**：image/video 等成本公式使用 `output_cost_per_image` / `output_cost_per_video` 等键，回落到 chat 条目取不到这些键仍会算出 0（带标记）。这类 mode 的正确做法是补价格表条目；`ai_price_miss` 标记 + `PriceLookupMiss` 告警可持续暴露缺口。
- **错价风险**：若某 mode 实际报价 ≠ chat 价，回落会造成系统性错价（方向取决于真实价差）；缓解 = 标记与计数器让缺口持续可见，运营需定期对账 `ai_price_miss` 行并补齐条目。
- **任何新增 mode 入口**：价格表未跟进时不再漏计（按 chat 价兜底），但仍需补条目以正确计价；监控 `PriceLookupMiss` / `PriceFallbackUsed` 突增。
- **跨仓依赖**：`bfe-access-pb` proto 变更与消费方升级需排期；落地前可先上「回落 + 带标识 WARN + `PriceFallbackUsed` 计数器」（BFE 仓内），访问日志标记随后补齐。
- **与 #1406 的边界**：#1406 是 usage 断裂（`total=0`）漏计，本缺陷是 usage 完整（`total==in+out`）漏计，日志签名不同（`total>0`），互不覆盖，需各自回归。
- **非缺陷部分不应被改动**：/count_tokens、客户端中断、定价生效前的 0 计费均属设计/配置行为；回落逻辑只作用于「`calcCostUnits` 价格查找」这一环，不改变上述路径。
- **初稿中曾列的其它零计费路径**（tier 价配 0、`quotaPlanFileListCheck` 循环副本缺陷、`hasRMBPlan` 兜底语义）经本轮数据**均非本缺陷根因**；其中 `token_rule_load.go` 的副本缺陷是真实存在的加载期隐患，建议另行开卡修复，不与本 issue 合并。

## 七、实施记录（2026-10-11）

已按「方案 2 + 观测性配套」实施（分支 `v1.8.8`），变更文件：

| 仓库 | 文件 | 改动 |
| --- | --- | --- |
| bfe-access-pb | `bfe_access_pb/bfe_access.proto` | 新增 `ai_price_miss`（field 763） |
| bfe-access-pb | `bfe_access_pb/bfe_access.pb.go` | `build.sh` 重新生成（纯增量：字段 + getter） |
| bfe-access-pb | `docs/protobuf.md` / `CHANGELOG.md` | 字段文档；新增 `v0.3.14` 变更记录；tag `v0.3.14` 已打并推 fork |
| bfe | `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `priceMiss` 字段 + `MarkPriceMiss()` / `IsPriceMiss()` |
| bfe | `mod_ai_token_auth/mod_ai_token_auth.go` | 新增 `PriceFallbackUsed` 计数器；`calcCostUnits` miss 分支：非 chat miss 且有 chat 基线 → 回落 chat 价 + 置标记 + 计数 + 带标识 WARN；无回落 → 0 + `PriceLookupMiss` + 置标记；`PriceLookupMiss` 注释同步修正 |
| bfe | `mod_access_pb3/request_log.go` | `IsPriceMiss()` 时写 `AiPriceMiss`（763） |
| bfe | `go.mod` / `go.sum` | `bfe-access-pb v0.3.6 → v0.3.14`（正式依赖；`v0.3.14` 已随 PR #18 合入上游并打 tag，本地 `replace` 已移除） |
| bfe | `docs/zh_cn/sys_design/rmb_quota.md` | §11.3 增加 mode 缺失回落条款；新增 §11.13 计费影响说明 |
| bfe 测试 | `mod_ai_token_auth_test.go` | 重写 `TestCalcCostUnits_PriceLookupMissCounter`（回落/命中/无基线三态） |
| bfe 测试 | `mod_access_pb3/request_log_test.go` | 新增 `TestReqAiInfoGenPriceMiss` |
| bfe 集成 | `scenario-SC03/sc03_rmb_quota_test.go` | 新增 `TestTC23_RMBQuotaDeduction_ModePriceFallback`（chat 对照 / responses 回落 / 无基线三臂） |
| bfe 集成设计文档 | `测试设计文档/`（SC03 场景说明、总体说明、`TC-23-*.md`） | 新增 TC-23，SC03 计数 23 |

验证结果（2026-10-11，分支 `v1.8.8`）：

| 范围 | 结果 |
| --- | --- |
| `go build ./...` / `go vet`（受影响包） | 通过 |
| 单测：`bfe_basic` / `mod_ai_token_auth` / `mod_access_pb3` / `bfe_config/bfe_cluster_conf/...` | 全部通过（含新增/重写用例） |
| 集成 SC03 全量（TC01~TC23） | 23/23 通过 |
| 集成 SC02 / SC05 / SC06 / SC07 / SC09 / SC11 / SC12 / SC29 / SC30 | 全部通过 |

未跑：`make test` 全仓（约 30 个集成场景）。

遗留事项：
1. ~~`go.mod` 的 `replace`~~ 已解决：`v0.3.14` 已合入上游 `bfenetworks/bfe-access-pb` 并打 tag，`replace` 已移除、`go.sum` 已按正式依赖更新。
2. `ai-gateway-api` 侧建议同步「base>0 且 tier=0 拒绝」的校验与 unlimited 导出排查（本卡范围外）。
3. `token_rule_load.go` 的 `quotaPlanFileListCheck` 循环副本缺陷另行开卡。