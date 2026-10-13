# Issue #1414 修复方案：usage 未识别导致计费被跳过时全链路零信号（观测性缺口 / UsedQuota 单字段双语义）

- Issue: [bfenetworks/bfe#1414](https://github.com/bfenetworks/bfe/issues/1414)（`SC3201-TC024`，2026-10-12 提交，open）
- 缺陷：BFE 在「上游 usage 未识别（无 final usage）且不满足估算计费条件」时按既定口径清零计费副本、跳过 RMB 扣减。issue 的核心主张是该状态**不得静默**：必须有可区分于"真实零用量请求"的信号；其根因指向 `UsedQuota` 一个字段同时承担访问日志展示值（`ai_total_tokens`）与"usage 是否被识别"哨兵两种语义。
- 代码库：`bfe/`，HEAD `1704ef10`（2026-10-12，"issue #1418"）。**本文行号均按该 HEAD 逐条读码核实**，非引用 issue 原文行号。
- 与 #1410 的关系（前置结论，务必先读）：本 issue 与 [bfe#1410](https://github.com/bfenetworks/bfe/issues/1410) 指向同一缺陷，**#1410 已于 2026-10-11 合入**（commit `1bd8cca9`，方案见 `docs/zh_cn/modifications/2026-10-11-issue-1410-unrecognized-usage-billing-skip-signal/design-changes.md`），交付了 `UsageUnrecognized` 计数器 + Warn + 访问日志 `ai_usage_source`（`bfe-access-pb` 764）。因此 issue 正文引用的两条关键证据（"仅 4 个计数器"、"守卫体内既无 `log.Warn` 也无 `counter.Inc`"）**已过时**，#1414 的 A-01/A-03 在"该守卫分支"这一具体形态上已获满足。
  但 #1410 是**单点补丁**，只覆盖那一条分支；#1414 的三条断言是**结构性**的，剩余部分在 HEAD 上仍未满足。本文即为这部分残余缺口（G1/G2/G3）的修复方案：**不重复 #1410 的工作，只做结构收口**。
- 关联：#1398（missing usage 不得静默记 0 的产品条款）、#1406（gzip 漏计费实例）、#1408（价格缺失信号先例）、#1409（disposition 机制与三元一致性检查）、#1352（客户端中止零扣费的既定口径）、#1418（访问日志字段保留先例）。本 issue 为 **P2 观测性/结构性缺口**：以不改计费口径为默认，只补齐"零计费必须可归因、可检测"。
- **计费口径决策（2026-10-13）：步骤 1B 已确认采纳**——估算兜底由"`UsedQuota <= 0`"改为"**上游 usage 缺失**"，唯一影响是"上游明确返回全 0 usage"的边界请求由"按估算计费"变为"按真实 0 计费"（金额下降，影响面与未验证项见第七节）。除此之外全篇不动计费金额。
- **信号范围决策（2026-10-13）：步骤 5 确认实施 A + B**（中止漏计费行的 `ai_usage_source=aborted` 行级标记 + 独立计数器 `AbortBillingSkipped`，`UsageUnrecognized` 语义收窄为"仅上游未识别"），不追加 Warn。两项均无金额影响；B 属告警基线变更，需同步既有阈值/看板。

## 一、口径定义（Expected 的来源）

| 条款 | 内容 |
| --- | --- |
| `rmb_quota.md` §7.5（issue #1398 说明） | 访问日志与统计读共享 `TokenUsage`，「保留真实解析值/批准口径的估算值，**不再静默记 0**」——即 missing usage 必须可观测 |
| `bfe.conf.md` `Server.EstimateToken` | 估算兜底仅在响应正常完成（`responseCompleted=true`）时可计费；"不可估算"分支客观存在且为**既定口径** |
| `rmb_quota.md` §7.5（issue #1410 说明，已合入） | 未识别且不可估算的守卫分支「计费口径不变，但必须可观测」，落地 `UsageUnrecognized` 计数 + Warn + `ai_usage_source` |

三条断言（issue 原文）：

- **A-01**：未识别 usage 且不可估算的请求，其状态必须可观测，**且与真实零用量请求可区分**；
- **A-02**：同一字段不得同时承担日志展示值与识别哨兵两种语义；
- **A-03**：计费旁路应能被产品自身（监控/告警）发现，而非只能靠带外人工对账。

诚实边界（沿用 issue 声明）：产品规定了"必须可观测"的**意图**，未规定信号形态（计数器名/日志字段/告警通道）。故 A-01/A-03 只主张"存在可区分、可自动检测的信号"；本文提出的字段名与计数器名为**实现选型**，不构成对产品的额外要求。同时，本文不主张"不可估算分支应当计费"（该分支口径不变）；唯一的计费口径变更见步骤 1B——**已确认采纳**（2026-10-13），仅作用于"上游明确报全 0 usage"的边界形态，不改变"usage 缺失走估算"的主流量口径。

## 二、现状核实：issue 断言 vs HEAD 事实

| issue 断言（依据 diagnosis 时点代码） | HEAD 事实 | 判定 |
| --- | --- | --- |
| 「`mod_ai_token_auth` 只注册 `ReqTotal`/`ReqAuth`/`ReqAuthFail`/`PriceLookupMiss` 四个计数器」 | `ModuleAITokenAuthState` 共 **8** 个（`mod_ai_token_auth.go:54-89`），含 `UsageUnrecognized`（`:88`）、`PriceFallbackUsed`、`Non2xxEstimateDropped`、`Non2xxRealUsageKept` | 证据过时 |
| 「清零守卫体内无 `log.Warn`、无 `counter.Inc`」 | `:398-420` 守卫体内有 `UsageUnrecognized.Inc(1)`（`:416`）与 Warn（`:417`，含 cluster/key_id/model/response_completed/allow_estimate） | 证据过时 |
| 「访问日志只能以 `ai_total_tokens=0` 表达，与真实零用量同形；无字段声明 usage 来源」 | `ai_usage_source`（`request_log.go:489-497`，proto 764）= `parsed`/`estimated`/`unrecognized`；对账可直接查询 | 证据过时 |
| 「守卫分支全链零信号」 | 该分支已有计数器 + Warn + 行级标记（`1bd8cca9`） | **A-01/A-03 在该分支上已满足** |
| 「`UsedQuota` 一个字段承担两种语义」 | 仍成立：展示值 `request_log.go:454`，哨兵 `llm_util.go:141`、`body_process.go:458`、`content_quota_usage.go:56/67/107`、`mod_ai_token_auth.go:181/204/287/290/433/436` | **A-02 未消解** |
| 「任何未来的新形态都会以同样静默的方式变成收入损失」 | 仍成立：守卫分支之外至少 14 处零计费路径无信号（见 G3） | **A-03 未结构性满足** |

结论：issue 的**示例**已修，**结构**未修。以下三条残余缺口是本文的修复对象。

## 三、残余缺口（HEAD 逐条核实）

### G1（对应 A-01 的"可区分"）识别层是**值判定**：真实零用量与"未识别"不可区分，并导致两种误处置

1. 解析层没有"usage 是否存在"的概念：`bfe_model_protocol/utils/usage_parse.go:27-39` 的 `UsageFields` 只有数值字段，无 presence 字段；三族解析与链式回退**全部以 `== 0` 为条件**（`:121`、`:130`、`:146`、`:197`、`:210`、`:214`、`:221`、`:266`、`:269`、`:297`）。因此 `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}` 与 `{}`（无 usage）产出**完全相同的 `UsageFields`**。
2. 判定点是值哨兵：`mod_body_process/llm_util.go:140-145` 与 `body_process.go:456-462`（非 SSE 路径）同构：

   ```go
   isguess := true
   if fields.UsedQuota > 0 || fields.ImageCount > 0 || fields.VideoCount > 0 {
       isguess = false
   }
   ```

   `isFinalUsage := !isguess && adapter.IsFinalUsageEvent(...)`（`llm_util.go:166`、`body_process.go:478`）。Gemini 适配器本身是按存在性判定的（`bfe_model_protocol/gemini/stream.go:38-40`，`gjson.Get(ev.Data,"usageMetadata").Exists()`），但与 `!isguess` 相与后，`{"usageMetadata":{"promptTokenCount":0,...}}` 仍然判 false。
3. 后果矩阵（usage 对象存在但全 0）：

   | 场景 | HEAD 行为 | 问题 |
   | --- | --- | --- |
   | 流式 + `EstimateToken=true`（出厂默认，见 `bfe_config/bfe_conf/conf_basic.go:120`） | `content_quota_usage.go:107-114` 按内容长度累加 completion → 终局 `estimateBillable` 成立 → 计费取估算值、`ai_usage_source=estimated` | **上游明确报告 0、BFE 却按估算超额计费** |
   | 流式 + `EstimateToken=false` | 计入 `UsageUnrecognized` | **假阳性**：真实零用量被判为"未识别" |
   | 非流式 2xx | `UpdateCtxByUsage`（`mod_ai_token_auth.go:181/204/216`）两个写入分支均不取 → 共享对象保留鉴权期播种值（`PromptTokens>0`、`CompletionTokens=-1`，见 `:738-752`）→ `:287` 不标记 final usage → `:290` 按 `bodyLen/4` 估算或落入守卫分支 | 同上两种误处置之一 |
4. 代码注释与事实相反（属本 issue 的文档交付项）：
   - `bfe_basic/request_ai_basic.go:306-308`：「parsed（… including genuine zero-usage requests, **which never fall into the unrecognized branch**）」；
   - `mod_ai_token_auth/mod_ai_token_auth.go:423-424` 与 `:81-87` 计数器注释同此断言。
   在 usage 全 0 的形态下该断言不成立（#1410 方案文档第五节已自认此限制，但代码注释未同步）。

> 即 G1 是 A-01"可区分"要求的**唯一未满足点**：`UsageUnrecognized` 计数无法排除真实零用量，"未识别"与"真实零用量"两者在信号上互相污染。

### G2（对应 A-02）`UsedQuota` 双语义仍在，且三元一致性检查在最需要的形态下自我禁用

- 展示值：`mod_access_pb3/request_log.go:454` `reqLog.AiTotalTokens = proto.Int64(usage.UsedQuota)`。
- 哨兵（识别判定）：`llm_util.go:141`、`body_process.go:458`、`content_quota_usage.go:56/67/107`、`mod_ai_token_auth.go:181/204/287/290/433/436` —— 至少 11 处。
- `request_log.go:717-720`：

  ```go
  func usageTripleInconsistent(usage *bfe_basic.TokenUsage, outputTokens int64) bool {
      if usage.UsedQuota <= 0 {
          return false
      }
      ...
      return usage.UsedQuota != usage.PromptTokens+outputTokens
  }
  ```

  该检查（issue #1409 的 A-02 载体，计数 `AiUsageInconsistent`，`request_log.go:79-83`）**恰好在 `total=0 且 in/out>0` 这一形态返回 false** —— 也就是 issue 生产证据里 736 行漏计费行的形态。唯一能自动发现该矛盾的服务端计数器对它恒为 0，这是 A-03"只能靠带外对账发现"的代码级原因。
- 结论：#1410 只是在日志侧加了一层来源标签遮蔽了歧义，`UsedQuota` 作为哨兵的角色（G2 前半）与一致性检查的禁用（G2 后半）都还在。

### G3（对应 A-03 的结构性）守卫分支之外的静默零计费面

`m.state.*` 的全部自增点共 **13 处**（覆盖 8 个计数器）：`mod_ai_token_auth.go:152/374/416/514/537/565/568/830/837`、`model_check.go:56/68/89`、`batch_quota.go:260`。`log.Logger.Warn` 全部 **20 处**：`conf_mod_ai_token_auth.go:56`、`batch_quota.go:167/173/192/206/210/216/236/261/279/284`、`mod_ai_token_auth.go:281/417/463/470/515/538/810/832/839`。据此逐条核对，以下零计费路径**无任何可计数、可归因的信号**：

| # | 零计费成因 | 位置 | 当前信号 |
| --- | --- | --- | --- |
| 1 | `calcCostUnits` 早退：`aiMeta/usage == nil`、`cluster/model` 为空、`serverConf` 快照为 nil | `mod_ai_token_auth.go:791-807` | 无 |
| 2 | cluster 的 ModelTable 缺失/查找失败 | `:809-812` | 仅 Warn（无计数器、无 `ai_price_miss` 标记） |
| 3 | price 条目存在但计费维度价格为 0（image/video 缺 `output_cost_per_*` 等） | `:863-884`（`GetPrice` 缺键返回 0）；input/output 基价为 0 时 `:917-938` | 无（配置只拒绝负价，0 合法） |
| 4 | 缓存维度拆分漏计：`cacheReadPrice>0` 而 `cacheWritePrice==0` 时，cache write token 已从 `normalInput` 扣除、又按 0 计价 | `:948-953` + `:1023-1024` | 无 |
| 5 | 只配 1h cache write 价时 `normalInput` 未扣除 cache write（`:948` 门槛为 false），与 `:1007-1015` 的 1h 计价叠加 | `:948` / `:1007-1015` | 无（反向：重复计费的隐患） |
| 6 | 收口跳过扣减：`costUnits == 0` 时整段 RMB 扣减被跳过 | `:452-465` | 无 |
| 7 | RMB 扣减失败 | `:461-464` | 仅 Warn，**无计数器**（Redis 故障导致的收入损失不可计数、不可告警） |
| 8 | token 扣减失败 | `:468-471` | 同上 |
| 9 | 批处理查表失败静默（与 #2 同条件却无 warn） | `batch_quota.go:252-254` | 无 |
| 10 | 批处理 `costUnits==0` / `totalTokens==0` → 直接释放预留 | `batch_quota.go:277-288` | 无 |
| 11 | 批处理 price miss 未打 `ai_price_miss` 标记（仅计数器 + warn） | `batch_quota.go:258-264` | 部分（行级不可查） |
| 12 | 批处理空 `BatchSettleId` 静默返回；settle 认领已被占用静默返回 | `batch_quota.go:227-229` / `239-245` | 无 |
| 13 | 余额封顶：`math.min(current, cost)` 丢弃欠额 | `token.go:97` / `:128` | 无（RMB 余额永不为负） |
| 14 | 行级不可分：`ai_cost_value` 仅在 `UsedCost > 0` 时写出 | `request_log.go:479-481` | 零成本行与"无成本"行**同形** |
| 15 | 客户端中止分支：计数（`:374`）后即 return，**早于** `ai_usage_source` 的设置点（`:425-432`）；两种子形态（无 usage / 仅初始 usage 使 `UsedQuota>0`）均被同一计数器吸收 | `mod_ai_token_auth.go:367-377`（详见"步骤 5"） | 有计数、无 warn、**无行级标记**，且与"上游未识别"混计 |
| 16 | `/count_tokens` 子串提前返回（连 token 计划也不扣减） | `:331-333` | 无（需确认是否既定口径） |

## 四、修复步骤

设计原则：

1. **默认不动计费口径**：唯一例外是步骤 1B（估算兜底改按 usage 缺失判定），**已确认采纳**（2026-10-13）并在第七节记录影响与回滚方式；除该边界形态外，本方案不改变任何请求的计费金额。
2. **复用既有信号模式**：模块计数器（经 `metrics.Init` 反射导出，含 `.diff`，`mod_ai_token_auth.go:109`、`monitorHandlers` `:592`）+ Warn + 逐请求日志标记（与 `ai_price_miss`、`ai_usage_source` 同构），监控与对账改造成本最低。
3. **先立判据，再加信号**：G1 的 presence 判定是 G3 收口信号能够排除"合法零成本"的前提（否则计数器会被真实零用量请求淹没）。
4. **收口点唯一化**：所有"2xx + RMB 计划 + 零计费"最终在同一处产生信号，新形态只需在早退点埋一个原因码即可被断言捕获，而不是每个新形态重新漏一次。

### 步骤 1（必改，核心）：识别层 presence 判定

#### 1A 分类与信号（**零金额变化**，先落地）

目标：把"上游是否给了 usage"变成显式事实，取代值哨兵。

- `bfe_model_protocol/utils/usage_parse.go`：`UsageFields`（`:27-39`）新增 `UsagePresent bool`；三族解析各自按**对象存在性**置位——OpenAI 家族 `usage` / `response.usage`（`:60-105`、`:112-146`）、Anthropic `usage` / `message.usage`（`:240-274`）、Gemini `usageMetadata`（`:290-302`）；`ParseUsageFieldsCrossProtocol`（`:195-228`）以 OR 组合三段的 presence。链式回退的 `== 0` 条件保持不变（它们解决的是"字段缺失时的取值回退"，与 presence 正交）。
- `bfe_modules/mod_body_process/llm_util.go`：`QuotaUsage`（`:36-57`）新增 `UsagePresent bool`；`SSEEvent.GetQuotaUsage`（`:135-185`）置位，`body_process.go:452-497`（`RawEvent.GetQuotaUsage`）同步。**保留 `IsGuess` 字段与现有取值**（估算路径依赖它），presence 作为新增的正交事实供分类使用。
- 分类判据改为 presence 驱动：
  - `isguess` 语义改为 `!UsagePresent`（usage 对象存在即非猜测），`IsFinalUsage = UsagePresent && adapter.IsFinalUsageEvent(...)`；
  - `finalUsageSeen` 的置位点（`content_quota_usage.go:45-47`、`mod_ai_token_auth.go:265-300` 的 `tokenReadResponseHandler`）由 presence 驱动（全 0 的真实零用量归 `parsed`）；
  - `ai_usage_source` 的判定点同步：`IsFinalUsageSeen()` → `parsed`（含全 0）；`!final && estimateBillable` → `estimated`；其余 → `unrecognized`。该 switch 只覆盖三态，第 4 态 `aborted` 由中止分支单独设置（步骤 5）。
- **实现补充（方案初稿未列）：字段级 presence `PromptPresent`**。Anthropic 流式 final usage（`message_delta`）**只带 `output_tokens`，prompt 字段是"缺席"而不是"报 0"**；若只按对象级 presence 处理，零输出的 `message_delta` 会抹掉 `message_start` 已解析的 prompt（少收）。因此 `UsageFields` / `QuotaUsage` 同时新增 `PromptPresent`（`usage.input_tokens` / `message.usage.input_tokens` / `prefix.prompt_tokens|input_tokens` / `usageMetadata.promptTokenCount` 的键存在性），流式累加的"继承上一事件 prompt"条件由 `PromptTokens == 0` 改为 `!PromptPresent`，并允许全 0 的 final 事件写通（`rquota.UsedQuota > 0 || rquota.IsFinalUsage`）。回归单测：`TestQuotaUsageProcessorProcessAnthropicPromptCarriedOver`（零输出 `message_delta` 仍按 `message_start` 的 100 计费）。
- **实现补充**：非流式 `UpdateCtxByUsage` 改为返回 presence 并按 presence 写通（含全 0），同时**保留 image/video 模式的请求侧计数兜底**（上游 usage 不报 `image_count` 时不抹掉鉴权期从请求体得到的可计费计数，与流式分支一致）；跨协议回退的触发条件保留值判定分支（否则 Anthropic 响应体在 openai 适配器下会漏掉 usage）。
- 效果：`UsageUnrecognized` 不再被真实零用量污染，A-01 的"可区分"成立；`UsedQuota` 退出**分类**角色（A-02 前半落地）。

#### 1B 计费口径切换（**已确认采纳**，2026-10-13）

- 估算兜底由"`UsedQuota <= 0`"改为"**usage 缺失**"：`mod_ai_token_auth.go:290-298`（非流式估算）、`:433-441`（计费估算与镜像）、`content_quota_usage.go:107-114`（流式估算累加）。判据即 1A 引入的 `UsagePresent`。
- 口径影响（已确认接受）：对"上游明确返回全 0 usage"的边界请求，计费由"按估算"变为"按真实 0"，**该形态金额下降**；usage 缺失的主流量行为完全不变（仍按估算计费）。**该形态在现网的频次未做运行期验证**（诚实边界，需在灰度期以 `ai_usage_source=parsed` 且各项计数为 0 的行数实测核对缺口量）。
- 落地顺序要求：1A 先合入并验证（零金额变化），再合 1B；二者可用同一 PR 但必须分提交，以便 1B 的金额变化可单独回滚。
- 与 A-02 的关系：1A 让 `UsedQuota` 退出**分类**角色，1B 让其退出**计费门槛**角色，两者合并后 `UsedQuota` 只剩下"`total_token` 计划的扣减量与 `ai_total_tokens` 展示值"两种语义（A-02 完全落地）。

### 步骤 2（必改）：解除 `usageTripleInconsistent` 的自我禁用

`mod_access_pb3/request_log.go:717-720` 的 `if usage.UsedQuota <= 0 { return false }` 改为按 usage 来源判定，**落地规则**：`ai_usage_source` 为 `unrecognized` / `aborted` 的行豁免（其 token 字段本就是鉴权期残留，且已有各自的计数器），其余行一律比较 `ai_total_tokens == ai_input_tokens + ai_output_tokens`（image/video 计次行保持豁免）。这样 `total=0 且 in/out>0` 真正触发既有 `AiUsageInconsistent` 计数（`:79-83`），让 A-03 具备服务端自检能力（不依赖带外脚本），同时**保留 #1409 对"无来源标记"行（如透传流量）的既有检测能力**——这是相对本方案初稿（"仅 parsed/estimated 参与"）的收窄。

### 步骤 3（必改，结构性兜底）：RMB 零计费收口信号

- 新增模块计数器 `BillingZeroCost`（`mod_ai_token_auth.go:54-89` 区域，带注释；经反射自动导出监控读数与 `.diff`）。
- 新增逐请求标记 `ai_zero_cost_reason`（`bfe-access-pb` 字段 **765**，AI 计量段 761-800 内紧邻 764，当前空闲）取值：
  `unrecognized` / `aborted` / `cache_hit` / `batch_release` / `price_miss` / `price_zero` / `model_table_missing` / `config_missing` / `no_usage_tokens` / `cost_zero`。
- 实现：与 `MarkPriceMiss` 同构，在 `aiBasicInfo` 上新增 `zeroCostReason` 字段 + `SetZeroCostReason/ZeroCostReason`（`bfe_basic/request_ai_basic.go`），在 G3 表中各静默早退点埋原因码；收口点（finish handler 计费结果之后、`batchSettle` 的扣减环之前）判定：

  ```go
  if costUnits == 0 && hasRMBPlan(ctx.Token.QuotaPlans) &&
      ctx.aiBasicInfo.TargetModel != "" && !isBatchManagementMode(ctx.aiBasicInfo.Mode) {
      reason := ctx.aiBasicInfo.ZeroCostReason()
      if reason == "" {
          if noBillableTokens(&billingUsage) { reason = ZeroCostReasonNoUsageTokens } else { reason = ZeroCostReasonCostZero }
      }
      ctx.aiBasicInfo.SetZeroCostReason(reason)
      m.state.BillingZeroCost.Inc(1)
      if zeroCostNeedsWarn(reason) { /* warn 携带 reason */ }
  }
  ```

- **两类形态被有意排除（收口与原因码都不产生，两者保持一致）**：
  1. **AI 缓存命中**：上游从未调用，零成本属设计；早退分支只写 `ai_zero_cost_reason=cache_hit` 供行级查询，**不计入计数器**（否则缓存命中会淹没告警）。
  2. **从未成为可计费模型调用的请求**：`mode` 为 `file` / `batch` 的批量与 files 管理端点（upload/create/get/list/cancel/download，见 `bfe_basic.ClassifyBatchOp`），以及未解析出 `targetModel` 的请求。这些请求绑定 API Key 与 RMB 计划但没有任何模型调用，计入会把每个批量生命周期的告警基数抬高；`calcCostUnits` 对它们也不埋原因码。批量**结算**有自己的收口点（`batchSettle`），不受影响。
- 说明：`no_usage_tokens` 承接"观测本身解释了零成本"的形态（上游明确报零用量、image/video 模式生成数为 0），`cost_zero` 为兜底。`legitZero` 的最初设想（`parsed` 且全 0 则不计）**未落地**——这类行随 `no_usage_tokens` 计入并可通过原因码筛除，比静默更符合"A-03 要可发现"的目标。
- 效果：A-03 由"某一个分支可观测"升级为"**所有可计费的 2xx + RMB 计划 + 零计费都可计数、可归因**"；新增形态只需在早退点埋原因码。
- 计数器为**精确计数、不采样**；Warn 与 #1409/#1410 的 warn 同量级（可按需加采样，计数器不得采样）；已在自身位置告警的形态（未识别守卫、中止分支、价格查找、model table 查找）不在收口点重复告警。

### 步骤 4（必改）：批量路径、扣减失败与 0 成本行显式化

- `batch_quota.go`：查表失败（`cluster/AIConf/ModelTable` 为空）补 Warn + `model_table_missing` 原因码；`serverConf`/cluster 不可用补 `config_missing`；price miss（`:258-264`）补 `aiMeta.MarkPriceMiss()` + `price_miss` 原因码；结算收口点（扣减环之前）在 `costUnits == 0 && hasRMBPlan` 时沿用已埋原因码、否则兜底 `reason=batch_release`，并计入 `BillingZeroCost` 且告警；释放预留的行为与金额不变。
  - **偏差记录**：方案初稿要求"空 `BatchSettleId` 与结算认领已被占用也各打原因码"，落地时**未采纳**——这两种情况表示"本次请求未执行结算"（不是结算计费为 0），打零成本原因码会造成归因错误。
  - **偏差记录（Phase 5 查实）**：`batch_release` **只能由上述结算兜底分支产生**；取消与失败终态的释放走 `BatchRelease`（`batch_quota.go:184-219`），既不进收口点、也不计 `BillingZeroCost`。因此"释放未计费臂"在 SC28 现有用例中没有载体，方案初稿中"release 也从该原因码覆盖"的设想不成立（已在 SC28 TC-11 文档中登记）。
- 新增计数器 `DeductFailed`：在 `mod_ai_token_auth.go:461-464`、`:468-471`、`batch_quota.go:278-280`、`:283-285` 的 err 分支 `Inc(1)`（Warn 保留）。使 Redis 故障导致"扣减丢失"从只有日志变为可告警。
- `request_log.go:479-481`：`ai_cost_value` 在 `UsageDispositionObserved` 的 2xx 行**总是写出**（含 0），使"零成本"与"字段缺席（非 2xx / 未计费）"可区分；非 2xx 行维持整组缺席（沿用 #1409 约定）。旧消费方若以 `ai_cost_value IS NULL` 判"无成本"需同步调整（见第七节）。

### 步骤 5（必改，已确认 A + B）：中止分支的行级标记与计数器拆分

**现状（HEAD 逐条核实）**：`mod_ai_token_auth.go:367-377`

```go
aborted := isClientAbortErr(req.ErrCode)
if aborted && !ctx.aiBasicInfo.IsFinalUsageSeen() {
    m.state.UsageUnrecognized.Inc(1)   // 与"上游未给 usage"的分支共用同一计数器
    ctx.deducted = true
    return bfe_module.BfeHandlerGoOn   // 早于 :425-432 的 ai_usage_source 判定点
}
```

`SetUsageDisposition(Observed)` 已在 `:344` 置位，因此该行**照常输出 token 字段**；但因分支提前 return，`UsedCost`（`:450`）与 `ai_usage_source`（`:425-432`）都不会设置。该分支有两种子形态，**都没有任何可区分的行级信号**：

| 子形态 | 触发（以 Anthropic 流式为例） | 访问日志实际内容 | 当前可见信号 |
| --- | --- | --- | --- |
| **A1 无 usage** | 尚无任何 usage 事件即断连 | `in = 鉴权期播种值 > 0`、`out = 0`（-1 被 clamp）、`total = 0`、`cost` 缺席、`source` 缺席 | 与"上游未识别"行**完全同形**；仅 `UsageUnrecognized` 计数（且与"未识别"混计） |
| **A2 已有初始 usage** | `message_start` 已写入 `UsedQuota`（`content_quota_usage.go:56-104`，Anthropic 的 `message_start` 不是 final 事件，见 `bfe_model_protocol/anthropic/stream.go:36-40`），随后 `message_delta` 之前断连 | `in = 100`、`out = 0`、`total = 100 > 0`、`cost` 缺席、`source` 缺席 | **看起来像已计费的行**（total>0）；`usageTripleInconsistent` 判定 100 == 100+0 亦为"一致"；行级唯一线索是 `ai_cost_value` 缺席，而它要等步骤 4 才成为显式信号 |

A2 使该形态比 issue 正文描述的更难发现：矛盾签名（`total=0 且 in/out>0`）不成立，`ai_usage_source` 亦缺失，且 token 计划也一并跳过扣减（`return` 早于 `:454-475` 的扣减环）。且客户端中断在生产中主要由用户主动停止驱动，频次不低。

**候选方案与决策（口径均不变，只加信号）**：

| 方案 | 内容 | 收益 | 成本 / 影响面 | 决策 |
| --- | --- | --- | --- | --- |
| **A. 行级来源标记** | abort 分支内 `SetUsageSource(UsageSourceAborted)`，新增取值 `aborted`；判定点保留在分支内（不上移） | 对账可直接查询 `ai_usage_source='aborted'` 命中"中止零扣费"全集，含 A2 形态 | **零 proto 成本**：`ai_usage_source` 是 764 已存在的 `optional string`，**新增枚举取值不需要 `bfe-access-pb` 发版**（与 765 `ai_zero_cost_reason` 不同） | **采纳** |
| **B. 计数器拆分** | 新增 `AbortBillingSkipped`，abort 分支不再计入 `UsageUnrecognized` | 监控侧区分"客户端断开"（用户行为，不必告警）与"usage 未识别"（上游/解析问题，需告警）——现状两者混计，`UsageUnrecognized` 的告警会被正常中断率污染 | 属**语义收窄变更**：`UsageUnrecognized` 基线下降，既有阈值/看板需同步；测试断言须改（`mod_ai_token_auth_test.go:2966-2983` 断言 abort 使 `UsageUnrecognized` +1 且 `UsageSource()==""`）；`rmb_quota.md` §7.5 的 #1410 段需同步 | **采纳** |
| C. 追加 Warn | 分支内加 `log.Logger.Warn` | 单请求可见 | warn 量随客户端中断率放大，属观测噪音；#1410 已明确选择 counter-only（`:369-373` 注释） | 不采纳 |
| D. 维持现状 | 不动 | 零成本 | issue 声明的"辅助观测点"不成立；A2 在步骤 4 之前完全不可发现 | 不采纳 |

**决策（2026-10-13）：实施 A + B**。二者均为纯信号增量、无金额影响；A 不引入 `bfe-access-pb` 依赖，可与步骤 1A/2/3/4 同批交付。

**实施要点（A + B 合并后）**：

1. **A**：`bfe_basic/request_ai_basic.go:305-317` 的来源常量由三态扩为四态，新增 `UsageSourceAborted = "aborted"`（注释说明：客户端在 final usage 之前断连，计费按 #1352 口径整笔跳过，token 字段为残留或初始 usage）；abort 分支（`mod_ai_token_auth.go:367-377`）内调用 `SetUsageSource(bfe_basic.UsageSourceAborted)`，判定点保留在分支内，不上移到 `:425-432` 的 switch（避免与"未识别"语义混淆）。
   - `bfe_access.proto` 的 764 无需变更、无需 `bfe-access-pb` 发版；该字段注释（可选）可在下一次 pb 发版时补上 `aborted` 取值说明。
   - 需同步四态说明的位置：`bfe_basic/request_ai_basic.go` 常量注释、`docs/zh_cn/sys_design/ai_access_log_fields.md`、`docs/zh_cn/sys_design/rmb_quota.md` §7.5。
2. **B**：`ModuleAITokenAuthState`（`mod_ai_token_auth.go:54-89`）新增 `AbortBillingSkipped`，注释中显式写明与 `UsageUnrecognized` 的分工；abort 分支改计 `AbortBillingSkipped.Inc(1)`，**不再计 `UsageUnrecognized`**（两种子形态 A1/A2 由同一判定吸收）。
   - 语义收窄需在三处公告：既有告警阈值/看板、`CHANGELOG` 的 `Changed`、`rmb_quota.md` §7.5（#1410 段原文把中止形态列在 `UsageUnrecognized` 之下）。
3. 与步骤 3 的关系：收口信号对 abort 形态同样成立（`BillingZeroCost` +1，reason=`aborted`），三者互补——步骤 3 回答"有没有零计费"，A 回答"是哪一行"，B 回答"监控侧如何区分"。

**衍生问题（不属步骤 5，建议单独开卡）**：A2 形态下 `message_start` 已解析出真实 usage（`UsedQuota > 0`），但按 #1352 既定口径整笔跳过扣减（含 token 计划）。"中止时是否应按已解析的初始 usage 计费"是**计费口径问题**，issue 明确不主张改变它，故本方案只做观测、不碰金额；是否评估请 owner 决定。

### 步骤 6（不在本 issue 范围，建议单独开卡）

- **缓存维度拆分**（`:948-953` 与 `:1023-1024`；`:1007-1015` 的 1h 叠加）：属**计费口径缺陷**（漏计/可能的重复计费），需产品确认后单独修复。本方案只保证它被 `reason=price_zero` 与 `BillingZeroCost` 暴露，不改金额。
- **余额封顶欠额**（`token.go:97`/`:128`）：需产品确认欠额处理口径（记账/告警/拒绝），不在观测性范围内闭环。
- **`/count_tokens` 跳过**（`:331-333`）：确认是否既定口径；若是，补注释与原因码。

## 五、回归测试

单元测试（`testing` + `testify`，放在被测代码旁）：

1. `bfe_model_protocol/utils/usage_parse_test.go`：全 0 usage 对象 → `UsagePresent == true`；`{}` → `false`；三族 + 跨协议组合各一例；"usage 存在但 `total_tokens` 缺失"仍按 prompt+completion 回退取值。
2. `bfe_modules/mod_body_process`：`TestGetQuotaUsage_AllZeroUsageIsPresent`（SSE 与非 SSE 各一）→ `UsagePresent == true`、`IsFinalUsage` 按事件类型成立；缺 usage → `false`。
3. `bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`：
   - 全 0 usage 的真实零用量（流式/非流式各一）→ `UsageUnrecognized` **不增**、`UsageSource()==parsed`、计费 0（按 1B 口径不再估算；反向断言：`EstimateToken=true` 时该用例的 `UsedCost` 必须为 0）；
   - 缺 usage 且响应完成 + `EstimateToken=true` → 仍按估算计费、`source=estimated`（1B 不得改变主流量口径的回归断言）；
   - 缺 usage + `EstimateToken=false` → `UsageUnrecognized` +1、`source=unrecognized`（#1410 既有断言保持通过）；
   - `hasRMBPlan && cost==0` 各成因（unrecognized / price_zero / model_table_missing / serverConf nil）→ `BillingZeroCost` +1 且 `ZeroCostReason()` 正确；`source=parsed` 且全 0 → **不增**（区分性反向断言）；
   - Redis 扣减失败 → `DeductFailed` +1；
   - 中止分支（步骤 5，已确认 A + B）：A1（无 usage）与 A2（先投递 `message_start` 使 `UsedQuota>0`、再置 `ErrCode`）两例 → 计费 0（既定口径不变）、`BillingZeroCost` +1 且 reason=`aborted`、`UsageSource()==aborted`、`AbortBillingSkipped` +1 **且 `UsageUnrecognized` 不增**（既有断言 `mod_ai_token_auth_test.go:2966-2983` 需按新语义改写：断言 abort 使 `AbortBillingSkipped` +1、`UsageUnrecognized` 保持 0、`UsageSource()==aborted`）。
4. `bfe_modules/mod_ai_token_auth`（batch）：`TestBatchSettle_ZeroCost` / `TestBatchSettle_PriceMissMarked`。
5. `bfe_modules/mod_access_pb3/request_log_test.go`：`total=0 且 in>0` → `usageTripleInconsistent` 为 true（步骤 2）；`ai_cost_value=0` 显式写出且非 2xx 行缺席；`ai_zero_cost_reason` 写出/缺席。

集成测试（`bfe/tests/integration`）：

6. SC03 新增 **TC-25**（TC-24 已被 #1410 占用）：`TC-25-真实零用量与未识别的可区分性.md`——双臂：全 0 usage 上游 vs 无 usage 上游（`EstimateToken` 关闭），断言 `ai_usage_source` 分别为 `parsed`/`unrecognized`、`UsageUnrecognized` 仅后者 +1。
7. SC03 追加零计费收口臂：配置 image 模式但价表缺 `output_cost_per_image` → 断言 `BillingZeroCost` +1、`ai_zero_cost_reason=price_zero`（步骤 3）。
8. SC28（ai-batch）：release 未计费臂 → `BillingZeroCost` +1、`reason=batch_release`。
9. SC12（client abort）：断言中止行 `ai_usage_source=aborted`、`AbortBillingSkipped` +1、`BillingZeroCost` +1（reason=`aborted`）、`UsageUnrecognized` 不增（步骤 5）。
10. SC05（访问日志）：`ai_cost_value=0` 与 `ai_zero_cost_reason` 字段断言（SC03 未加载 `mod_access_pb3`，日志字段断言放 SC05）。

**实际交付的测试载体（与上述计划的映射与偏差）**：

| 计划 | 实际交付 | 偏差说明 |
| --- | --- | --- |
| 6. SC03 TC-25（全 0 vs 无 usage） | ✅ SC03 **TC-25**（三臂） | 全 0 臂保留 `EstimateToken` **出厂默认 true**（而非计划中的"关闭估算"）——必须在估算开启下证明 presence 优先；余额不变即为"未被估算"的等价断言。另加正常计费对照臂 |
| 7. SC03 零计费收口臂（image 缺价 → price_zero） | ✅ SC03 **TC-26**（两臂 + 对照） | 改用 SC03 现成 fixture 可稳定构造的 `cluster_no_table`（`model_table_missing`）与"价格表无该 model"（`price_miss`）；`price_zero` 变体由单元测试 `TestTokenRequestFinishHandler_ZeroCostReasonPriceZero` 覆盖（含 0 价条目在 SC03 fixture 中需新增配置，收益不抵成本） |
| 8. SC28 release 未计费臂（batch_release） | ⚠️ **未新增用例**，改为在 SC28 **TC-11** 追加信号断言 | Phase 5 查实 `batch_release` 仅由结算兜底分支产生、`BatchRelease` 不进收口点（见步骤 4 偏差），现有用例无载体；追加的断言为 `BillingZeroCost` 增量 + `PriceLookupMiss` + `ai_price_miss` + `ai_cost_value=0`（并**明确不断言**下载行的 `ai_usage_source`——该路径早于 usage-source 判定点返回） |
| 9. SC12 中止信号 | ✅ SC12 **TC-04**（A1/A2 双形态 + 计数器） | 行级字段（`ai_usage_source=aborted` / `ai_zero_cost_reason=aborted`）在 SC12 不可断言（未加载 `mod_access_pb3`），由单元测试覆盖；A2 的**行级**标记仍列为待补覆盖项（现有单测只覆盖 A1 形态） |
| 10. SC05 日志字段 | ✅ SC05 **TC-21**（三臂） | 臂 2 用"上游显式全 0 usage"（该场景可稳定构造）而非"未识别 usage"，理由记于 TC-21 文档 |
| 3. `DeductFailed` 单测 | ❌ **未交付** | 测试用 mock Redis 无法注入扣减错误（无失败注入设施）；`DeductFailed` 目前仅有代码覆盖，列为待补（需要 mock 支持或故障注入） |
| 4. batch 单测（`TestBatchSettle_*`） | ❌ **未交付** | `batchSettle` 无直接单测载体；batch 侧信号由 SC28 TC-11 的集成断言覆盖 |
| 3. "`source=parsed` 且全 0 → 计数器不增" | ❌ **有意未实现** | 该形态按 `no_usage_tokens` 计入（见步骤 3 说明）：可发现优于静默，运营初期以原因码筛除 |
| 1/2/5（解析层、body_process、日志层） | ✅ 全部交付 | 见上表 1/2/5 条；另补 presence 的 `PromptPresent` 回归用例 |

验证命令：

```bash
cd bfe
go build ./... && go vet ./...
go test ./bfe_model_protocol/... ./bfe_modules/mod_body_process/ ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_basic/
go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ -v
go test ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/ ./tests/integration/implementation/scenario-SC12-client-abort-billing/ ./tests/integration/implementation/scenario-SC28-ai-batch-support/ -v
```

## 六、修复后目标行为（矩阵）

以"usage 存在性 × 估算配置 × 响应完成 × 计费结果"列全（**1B 已确认后的目标态**）：

| usage 存在 | 事件终局 | EstimateToken / 响应完成 | finalUsageSeen | 计费 | `ai_usage_source` | 计数器 |
| --- | --- | --- | --- | --- | --- | --- |
| 存在（含全 0） | 是 | 任意 | 是 | 按真实值（全 0 则 0，**不再估算**） | `parsed` | 无 |
| 缺失 | — | 启用 / 完成 | 否 | 按估算 | `estimated` | 无 |
| 缺失 | — | 关闭 或 未完成 | 否 | 0 | `unrecognized` | `UsageUnrecognized` +1，`BillingZeroCost` +1（reason=`unrecognized`） |
| 缺失 + 客户端中止 | — | — | 否 | 0 | `aborted` | `AbortBillingSkipped` +1、`BillingZeroCost` +1（reason=`aborted`）；`UsageUnrecognized` 不增 |
| 存在（仅初始，非终局）+ 客户端中止 | 否 | — | 否 | 0（token 计划同样跳过） | `aborted` | 同上一行；**修复前该形态行内 `total>0` 却零成本，无任何信号** |
| 任意 | — | — | 任意 | 0 且价表/配置异常 | — | `BillingZeroCost` +1（reason=`price_zero`/`price_miss`/`model_table_missing`/`config_missing`） |
| 任意（非 2xx 上游） | — | — | — | 0 | 缺席 | `Non2xxEstimateDropped`/`Non2xxRealUsageKept`（沿用 #1409） |

## 七、边界与残余风险

- **步骤 1B 的金额影响（已确认接受）**：对"上游明确报 0 usage"的边界请求，计费由估算变为真实 0，**该形态金额下降**。诚实边界：该形态现网频次未验证，需在灰度期实测（查询 `ai_usage_source='parsed'` 且各项 token 计数均为 0 的行数，对比改前的 `estimated` 行数）估算缺口量；若量级超出预期，1B 可单独回滚（1A 保留，分类与信号不受影响）。
- **既有测试断言会变**：presence 判定会改写 `bfe_model_protocol/utils/usage_parse_test.go:64`、`bfe_model_protocol/gemini/usage_test.go:49`、`mod_body_process/content_quota_usage_test.go`、`mod_ai_token_auth/mod_ai_token_auth_test.go:2907-2918` 等断言，需同步更新并在 PR 中说明语义变化。
- **proto 依赖**：`ai_zero_cost_reason = 765` 需 `bfe-access-pb` 仓库先行（proto + 重新生成 pb.go + `docs/protobuf.md` + `CHANGELOG`，流程与 #1410 的 764 / v0.3.15 一致），`bfe/go.mod` 需相应提升 require；字段为 `optional string`，向后兼容，旧消费方无感。
- **`ai_cost_value=0` 显式写出**改变既有语义：原本"字段缺席"表达"0 或未知"，改后表达"非 2xx / 未计费"。使用 `ai_cost_value IS NULL` 判定的对账脚本需改为 `ai_cost_value = 0` / `IS NULL` 组合，需与 owner 确认切换时点（可在同一 PR 的 CHANGELOG 中提示）。
- **Warn 量与 IO**：`BillingZeroCost` 的 Warn 在高并发零计费场景可能放大；若噪音过大可对该 Warn 采样，**计数器必须不采样**（对账依赖精确计数）。
- **对外语义**：无新增配置项、无模块顺序调整、无转发行为变化；除 1B 的边界形态外无计费金额变化，仅新增计数器维度与日志字段（1B 的边界金额变化需在 CHANGELOG 的 `Changed` 中明示）。
- **残余盲区（本 issue 范围外）**：`#1408` 的"usage 已识别却停扣费（窗口期配额计划状态）"若成因不在价表，仍可能只有 warn 而无计数器——需该 issue 自身闭环后评估是否补"识别后停扣"维度。
- **`BillingZeroCost` 的假阳性**：上游合法返回 0 用量且未被视为 `parsed` 的形态（如 image 模式返回 `ImageCount=0`）会落入 `no_usage_tokens` 原因码，需在运营初期核对基数并以之为准校准告警阈值。
- **步骤 5 的告警基线变更（已确认接受）**：方案 B 把 `UsageUnrecognized` 收窄为"仅上游未识别"，其数值会下降，而中止形态改由新计数器 `AbortBillingSkipped` 承载——既有的 `UsageUnrecognized` 阈值/看板需在发布前同步调整，并在 `CHANGELOG` 的 `Changed` 与 `rmb_quota.md` §7.5 中公告；`ai_usage_source` 新增取值 `aborted`（764 无需 proto 变更、无需 pb 发版），旧消费方若按白名单匹配取值会落为 unknown，按字符串直存则无感，发布前需与该消费方确认。两项均无金额影响。

## 八、实施顺序与文档同步清单

建议顺序（每步独立可验证、可回滚）：

| 序 | 步骤 | 金额影响 | 前置 |
| --- | --- | --- | --- |
| 1 | 步骤 1A（presence 分类 + 注释修正） | 无 | — |
| 2 | 步骤 2（三元一致性解除禁用） | 无 | 1A（source 可用作判据） |
| 3 | 步骤 3（`BillingZeroCost` + 原因码 + 765 字段） | 无 | `bfe-access-pb` 先行；1A |
| 4 | 步骤 4（batch 原因码 / `DeductFailed` / 0 成本显式化） | 无 | 3 |
| 5 | 步骤 5（中止行级标记 `ai_usage_source=aborted` + 计数器 `AbortBillingSkipped`） | 无 | 3；**已确认（A + B，2026-10-13）** |
| 6 | 步骤 1B（计费门槛切 presence） | **下降**（仅边界形态） | 1A 已合入并验证；**口径已确认（2026-10-13）** |

文档同步：

- `docs/zh_cn/sys_design/rmb_quota.md` §7.5：补 issue #1414 说明（presence 判定、`BillingZeroCost`、`ai_zero_cost_reason`、`ai_usage_source` 四态）与变更记录条目；**同时改写 #1410 段**——中止形态改由 `AbortBillingSkipped` 承载，`UsageUnrecognized` 收窄为"仅上游未识别"；
- `docs/zh_cn/configuration/bfe.conf.md`（及 en_us 对应页）：`Server.EstimateToken` 的兜底条件改为"仅在响应完成**且上游未提供 usage** 时可计费"，并同步 `docs/zh_cn/sys_design/rmb_quota.md` 中的估算条件描述；
- `docs/zh_cn/sys_design/ai_access_log_fields.md`：新增 765 `ai_zero_cost_reason` 字段说明，补 764 `ai_usage_source` 的第 4 个取值 `aborted`，并修订 `ai_cost_value` 的"0 vs 缺席"语义；
- `CHANGELOG.md`：`Fixed`/`Changed` 条目（含 1B 边界计费口径变化、`UsageUnrecognized` 语义收窄、对账脚本语义变化的提示）；
- `bfe-access-pb`：proto 765 + 生成物 + `docs/protobuf.md` + `CHANGELOG`（764 的取值 `aborted` 无需 proto 变更，其字段注释可随本次一并在 pb 侧补充）；
- `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/`：新增 TC-25 文档与场景数登记（含中止形态的 A1/A2 两臂断言）。

## 九、实施记录（2026-10-13）

按 Skill `bfe-rd-workflow` 的 Phase 2→7 执行（Phase 2 配置文档、Phase 3 系统设计文档、Phase 4 代码与单测、Phase 5 集成测试设计文档、Phase 6 集成测试代码、Phase 7 回归）。

### 9.1 变更文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `UsageSourceAborted` 常量（第 4 态）；`ZeroCostReason*` 10 个原因常量；`zeroCostReason` 字段 + `SetZeroCostReason/ZeroCostReason`；常量与字段注释同步 |
| `bfe_model_protocol/utils/usage_parse.go` | `UsageFields` 新增 `UsagePresent` / `PromptPresent`；OpenAI 家族 / Anthropic / Gemini 三族按对象与字段存在性置位；跨协议组合 OR |
| `bfe_modules/mod_body_process/llm_util.go`、`body_process.go` | `QuotaUsage` 新增 `UsagePresent` / `PromptPresent`；`isguess := !UsagePresent`；SSE 与 whole-body 两条路径同步 |
| `bfe_modules/mod_body_process/content_quota_usage.go` | 全 0 的 final usage 写通（`UsedQuota > 0 \|\| IsFinalUsage`）；继承 prompt 的条件改为 `!PromptPresent`；估算累加增加 `!IsFinalUsageSeen()` 门槛 |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `UpdateCtxByUsage` 改为返回 presence 并按 presence 写通；`tokenReadResponseHandler` 用 presence 标记 final usage / 门控估算；finish handler：估算与镜像加 `!IsFinalUsageSeen()`、中止分支（`AbortBillingSkipped` + `UsageSourceAborted` + 计数/归因）、缓存命中标记、未识别守卫埋原因码、**零计费收口点**（`BillingZeroCost` + 原因码 + 条件告警）、`DeductFailed`；`calcCostUnits` 早退点埋原因码 + 非模型调用早退 + `price_zero` 判定；新增 `isBatchManagementMode` / `noBillableTokens` / `zeroCostNeedsWarn` 辅助 |
| `bfe_modules/mod_ai_token_auth/batch_quota.go` | 结算收口点（`BillingZeroCost` + 原因码 + 告警）、model table 缺失告警、price miss 补 `MarkPriceMiss`、`DeductFailed` |
| `bfe_modules/mod_access_pb3/request_log.go` | `usageTripleInconsistent` 增加 source 参数并解除 `UsedQuota <= 0` 早退；`ai_zero_cost_reason` 写出；`ai_cost_value` 对观测行显式写 0 |
| 单测（7 个文件） | 新增 12 例、更新既有断言 8 处（详见 §五"实际交付的测试载体"） |
| 集成测试（5 个场景） | SC03 新增 TC-25/TC-26；SC05 新增 TC-21；SC12 新增 TC-04（含 monitor 端口捕获）；SC28 TC-11 追加断言（含 `batchLogEntry` 扩展）；新增共享 `tests/integration/common/monitor.go` 计数器读数工具 |
| 文档 | `docs/zh_cn/configuration/bfe.conf.md`、`docs/en_us/configuration/bfe.conf.md`、`docs/zh_cn/sys_design/rmb_quota.md`（§7.5 + 变更记录第 15 条）、`docs/zh_cn/sys_design/ai_access_log_fields.md`（补 763/764/765 三行 + §5.2/§5.7）、`CHANGELOG.md`（Added/Changed/Fixed）、`测试设计文档` 4 个场景说明 + 总体说明 |
| `go.mod` / `go.sum` | `require github.com/bfenetworks/bfe-access-pb v0.3.16`（开发期曾用临时 `replace => ../bfe-access-pb`，v0.3.16 进入 upstream 与模块代理后已移除并按 `go get ...@v0.3.16` + `go mod tidy` 落地，`go.sum` 仅替换该模块条目） |
| `bfe-access-pb`（独立仓库） | proto 765 `ai_zero_cost_reason` + 764 注释补 `aborted` 取值 + 重新生成 pb.go + `docs/protobuf.md` + `CHANGELOG v0.3.16`；已 commit `03daf2c` + annotated tag `v0.3.16`（tag 对象 `fa0880d`），先推 origin（fork），**v0.3.16 现已在 upstream `bfenetworks/bfe-access-pb` 与模块代理可见** |

### 9.2 验证结果（全部实跑）

| 范围 | 结果 |
| --- | --- |
| `go build ./...` / `go vet ./...` / `gofmt` | 通过、无告警、改动文件格式干净 |
| 单测：`bfe_basic` / `bfe_model_protocol/...` / `bfe_modules/...` / `bfe_server/...` | 全部通过 |
| 集成：SC03（26 例，含新增 TC-25/TC-26） | 通过（≈32–46s） |
| 集成：SC05（21，含 TC-21）、SC12（4，含 TC-04）、SC28（14，含 TC-11 追加断言） | 全部通过 |
| 集成回归：SC01/SC02/SC04/SC06/SC07/SC08/SC09/SC10/SC11/SC14/SC15/SC16/SC17/SC18/SC19/SC20/SC21/SC22/SC23/SC24/SC25/SC26/SC27/SC29/SC30/SC31（26 个场景） | 全部通过 |
| 集成：SC13（TLS 热加载路径重定向） | **失败 2 例，经核验为既有失败、与本改动无关**：在干净 `HEAD 1704ef10` 工作树（`git worktree add`，无本次任何改动）上复现完全相同的两条失败——`sc13_tls_conf_reload_path_test.go:173 decode reload rsp failed: invalid character 'U' in string escape code` 与 `:354 reload with path should fail: example_ca.crt missing in version dir`，属 Windows 环境下该场景自身的路径/转义问题（reload 响应体中的 Windows 路径未被 JSON 转义）；不在 issue #1414 范围内，未处理 |

### 9.3 实施中发现并回写的偏差（已在正文各节标注）

1. **`PromptPresent` 字段级 presence 为本方案新增**（步骤 1A 补充）：否则零输出的 Anthropic `message_delta` 会抹掉 `message_start` 的 prompt。
2. **三元一致性检查的落地规则**为"`unrecognized`/`aborted` 行豁免，其余照比"（步骤 2）——保留 #1409 对无来源行（透传流量）的既有检测。
3. **收口点排除两类形态**（步骤 3）：AI 缓存命中（只写 `cache_hit` 标记、不计数），以及"从未成为可计费模型调用"的请求（`mode` 为 `file`/`batch` 的管理端点、未解析出 `targetModel`）——后者由集成测试实测发现：批量生命周期的 upload/create/get 三个控制请求会各自计入 `BillingZeroCost`（`reason=config_missing`），抬高告警基数；修正后计数器与行级原因码两侧一致（都不产生）。
4. **`batch_release` 只由结算兜底分支产生**（步骤 4 偏差记录），取消/失败终态释放不产生信号。
5. **"`source=parsed` 且全 0 → 不计数"未落地**：该形态按 `no_usage_tokens` 计入（可发现优于静默）。
6. `DeductFailed` 与 batch 结算缺单测载体（§五 表中已记为待补）。

### 9.4 未完成 / 待办

- ~~**`bfe-access-pb` 的 v0.3.16 尚未进入 upstream**~~ **已完成（2026-10-13）**：`v0.3.16` 已在 `bfenetworks/bfe-access-pb`（annotated tag `fa0880d` → commit `03daf2c`）与模块代理可见；`bfe/go.mod` 的临时 `replace` 已移除，改为 `require ... v0.3.16`，并复验通过（`go build ./...`、`go vet ./...`、单测、SC03/SC05/SC28 集成全绿）。
- **Git 提交未执行**：`bfe` 侧变更（23 个文件 + 本目录）尚未 commit；按 Skill 要求需人工确认提交内容后授权，且 `git push` 默认推 `origin`。
- **运营侧动作（非代码）**：`UsageUnrecognized` 的既有告警阈值/看板需对准新的 `AbortBillingSkipped`；使用 `ai_cost_value IS NULL` 判"无成本"的对账脚本需改为 `= 0` / `IS NULL` 组合；与访问日志消费方确认对 `ai_usage_source` 新增取值 `aborted` 的处理。
- **待补覆盖**：SC12 A2 形态的行级标记（需在 `mod_ai_token_auth_test.go` 补 A2 单元用例或 SC05 补中止臂）；`DeductFailed` 的故障注入单测。
- **另案处理**（步骤 6）：缓存维度拆分漏计/重复计费、余额封顶欠额、`/count_tokens` 口径确认、以及"中止时是否按已解析的初始 usage 计费"。
- **既有失败（非本 issue 引入，已核验）**：SC13（TLS 热加载路径重定向）在 Windows 上 2 例失败，干净 HEAD 工作树同样复现，详见 §9.2；如需修复应单独开卡。

## 附：被否决 / 降级的候选

- **让 `UsedQuota` 退化为纯展示值并另设独立哨兵字段**：由步骤 1A 以 presence 字段达成同一目标，且不必改动 `TokenUsage` 结构（计费函数族、共享对象全部读写方），代价更低、风险更小，故降级为实现 1A 而非重构。
- **在 `mod_access_pb3` 侧以启发式判定（`total=0 且 in/out>0`）报矛盾**：依赖外部先验（鉴权期按请求体播种 `PromptTokens` 的规则），且与 issue 主张的"一等公民字段"相悖，故只作为步骤 2 的补充计数，不作为主判据。
- **步骤 5 方案 C（中止分支追加 Warn）**：客户端中断率会线性放大 warn 量，属观测噪音；#1410 已为该分支确立 counter-only 模式（`mod_ai_token_auth.go:369-373`），行级可查询由方案 A 的 `ai_usage_source=aborted` 承担，故不采纳（2026-10-13）。
- **步骤 5 方案 D（中止分支维持现状）**：会使 issue 声明的"辅助观测点"不成立，且 A2 子形态（`total>0` 却零成本）在步骤 4 之前完全不可发现，故不采纳（2026-10-13）。
- **对每个新形态单独补一个计数器**：可复现 #1410 的路径依赖问题（新形态仍需人工发现），故改为步骤 3 的单一收口 + 原因码枚举。