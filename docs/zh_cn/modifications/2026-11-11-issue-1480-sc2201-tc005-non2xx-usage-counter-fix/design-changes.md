# BFE Issue #1480 复验修复说明：SC2201-TC005 非 2xx 失败行 usage 上报的计数器可达性与设计一致性

- 来源：[issue #1480 评论 SC2201-TC005](https://github.com/bfenetworks/bfe/issues/1480#issuecomment-6761452019)（QA 复验，2026-11-10，run `20261110-135100.055269669-sc2201tc005`，FAILED_PRODUCT）
- 缺陷：启用 RMB plan 的 Key 请求返回 **400 且 body 携带 usage** 的受控后端时，期望"失败行 usage 被保留上报"可由计数器证明；实际 `NON2XX_REAL_USAGE_KEPT` delta=0 且 #1480 设计名计数器 `Non2xxUsageReported` 不存在（实现名为 `NON2XX_ESTIMATE_DROPPED`/`NON2XX_REAL_USAGE_KEPT`），而 Report 400 行却有完整 token——**两个观测无法被任何现存计数器同时解释**，#1480 的防漏计哨兵失效。
- 代码库：`bfe/`（行号基于当前 HEAD `9006619a` 逐条核实；初稿基于 `5611f8f1`，此后 #1501/PR #1504（`698ff57a`→`5040ff1d`→`9006619a`）重构了 `tokenReadResponseHandler`：`MarkResponseCompleted()` 移入 `GetBodyAccessor` 成功分支、新增截断体 else 分支，函数行号整体前移，见 §二.步骤 1 注记）；QA 部署构建 `75ddd9ab`（vcs.time=2026-11-08T05:01:05Z），其分支走向为推断待确认（见 §三.R3 核验步骤）。
- 关联：#1398（估算镜像）、#1409（非 2xx 残留抑制）、#1414（presence 判定）、#1480（total=in+out + 失败行上报设计，2026-10-31）、#1493（非 2xx 抑制语义恢复，2026-11-08，**否决了 #1480 的保留语义反转**）、#1501（截断体不置完成标记，2026-11-11，与本修复同一函数）。
- **裁定记录（2026-11-12）**：步骤 2 候选 2 已获裁定接受——#1480 设计文档按其 §五.2/§五.6/§六/改动清单 :180 等条款逐处加注"已被 #1493 取代"（承认实现命名双计数器口径，见 `2026-10-31-issue-1480-total-tokens-in-plus-out/design-changes.md` 顶部状态更新）；E2E 断言（SC2201-TC005、SC3201-TC011）改用实现名 `NON2XX_REAL_USAGE_KEPT`，由 QA 侧在复验时更新断言并解除 FAILED_PRODUCT。

## 一、根因

### R1（核心，本地源码实锤）：非 2xx 真实 usage 的解析被 StatusOK 门控挡死，"真实 usage 保留"分支不可达

当前代码里，`finalUsageSeen` 只可能由两条路径置位，**两条都到不了非 2xx 响应**：

1. `tokenReadResponseHandler`（`mod_ai_token_auth.go:379`）：`if res.StatusCode == bfe_http.StatusOK && isCompleteNonStreamBody(res)` —— 非流式 body 的 usage 解析（`UpdateCtxByUsage`）整体被 `StatusOK` 门控，400/500 带 usage 的响应**根本不进解析**；
2. `QuotaUsageProcessor.Process`（`mod_body_process/content_quota_usage.go:27-31`）：`NewQuotaUsageProcessor` 对非 2xx 直接返回 nil —— 流式事件路径同样不解析。

于是 `dropUnbilledEstimate`（`mod_ai_token_auth.go:741`）的分支判定对"400 + usage"固定走**估算残留分支**（鉴权期播种的 `PromptTokens > 0` ⇒ 不清零前提成立 ⇒ 清零 + `Non2xxEstimateDropped.Inc(1)`，`:764-791`），而 `:752-762` 的**真实 usage 分支（`Non2xxRealUsageKept` + `UsageSource=parsed`）是死代码**——`IsFinalUsageSeen()` 对非 2xx 恒为 false。

这精确解释 QA 观测：`NON2XX_REAL_USAGE_KEPT delta=0`。失败臂不被任何计数器证明不是部署问题，是**本地源码即如此**——之前修复 #1480 时实现了"保留"分支（#1493 恢复后改为双计数器），但**漏掉了让非 2xx usage 到达该分支的解析通路**。既有单测 `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation`（`mod_ai_token_auth_test.go:3041`）以手动 `MarkFinalUsageSeen()` 播种覆盖了 finish 侧分支，恰恰掩盖了"从真实响应到不了该分支"这一缺口。

### R2（设计漂移，实锤）：#1480 设计文档的计数器命名/语义条款被 #1493 否决后未回修订

- #1480 设计（`2026-10-31-issue-1480-total-tokens-in-plus-out/design-changes.md` §五.6/§六/改动清单 :180）：`Non2xxEstimateDropped` **改名** `Non2xxUsageReported`，语义从"丢弃残留"反转成"记录非 2xx 用量（含残留保留上报，source=failed）"，`dropUnbilledEstimate` → `keepNon2xxUsage`。
- #1493（`2026-11-08-issue-1493-non2xx-usage-suppression-restore`）：以 §10 字段定义规范条款（"禁止对从未成功调用上游的请求上报鉴权期/字节长度估算 token"）**否决了 #1480 的保留语义**，恢复 #1409 清零抑制；实现保留 `Non2xxEstimateDropped`（残留抑制计数）并**新增** `Non2xxRealUsageKept`（真实 usage 观测计数）。当前 HEAD 即此形态（`mod_ai_token_auth.go:82-94` 计数器定义与注释、`:449` "restored by #1493, reversing issue #1480's" 明示）。
- 后果：#1480 设计文档未同步标注被取代 → 按设计名（`Non2xxUsageReported`）断言的 E2E（SC2201-TC005 A-ingest、SC3201-TC011）与任何已部署构建**必然**失配；QA 已排除大小写问题（PascalCase 与 SCREAMING_SNAKE 实读均不存在）。
- **处置（2026-11-12 已裁定）**：采候选 2——#1480 设计文档已逐处加注"已被 #1493 取代"，承认 `Non2xxEstimateDropped` + `Non2xxRealUsageKept` 双计数器口径；E2E 断言改用实现名 `NON2XX_REAL_USAGE_KEPT`。本条款闭环。

### R3（部署侧待确认）：Report 400 行有 token 与本地 #1493 语义矛盾

按本地源码推演"400 + usage + EstimateToken=true"：usage 不解析 → 残留分支清零全部字段 + `UsageDispositionNone` → pb3（注册在 `mod_ai_token_auth` 之后，`bfe_modules/bfe_modules.go:155/215`，看到的是清理后状态）**不写 token 字段**。与 QA 实测"Report 行完整非零三元组"矛盾。可能解释：部署构建 `75ddd9ab` 与本地 HEAD 在该路径语义不同（构建时点/v cherry-pick 差异），或失败臂响应形状与假设不同。**修复部署后该矛盾应自然消失**；若仍存在，需按 §四.步骤 4 直读部署构建确认。

## 二、修复设计

### 步骤 1（核心，必做）：放开非 2xx 真实 usage 的解析通路

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`tokenReadResponseHandler`（`:373-423`）：

```go
	if isCompleteNonStreamBody(res) {
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil && bodyAccessor != nil {
			if res.StatusCode == bfe_http.StatusOK {
				// completion is a 2xx concept; non-2xx must never satisfy the
				// billing completion gate. 保持在本分支内：截断体不得置完成
				// 标记（issue #1501）。
				ctx.aiBasicInfo.MarkResponseCompleted()
			}
			body, _ := bodyAccessor.GetBytes()
			// ... 解压副本（既有逻辑，issue #1406）...
			usagePresent = UpdateCtxByUsage(ctx, body)
			if usagePresent {
				// issue #1480/#1493: a real usage object on an error response is a
				// real observation — mark it so dropUnbilledEstimate reports it as
				// parsed instead of dropping the row as estimate residue.
				ctx.aiBasicInfo.MarkFinalUsageSeen()
			}
			if res.StatusCode == bfe_http.StatusOK && !usagePresent &&
				ctx.aiBasicInfo.IsAllowEstimateToken() {
				// estimate seeding stays 2xx-only (issue #1409/#1493 clause).
				...
			}
		} else {
			// issue #1501: the upstream body was truncated（GetBodyAccessor
			// 失败或 accessor 为 nil）——保持既有 else 分支原样：不置完成
			// 标记、不解析、不估算。
		}
	}
```

**注记（相对初稿草图的必要修正）**：初稿草图把 `MarkResponseCompleted()` 放在 `StatusOK` 门下方、解析之前，那是 `5611f8f1` 时代的结构。#1501/PR #1504 已把它移入 `GetBodyAccessor` 成功分支（截断体不得置完成、不得估算）。落实本步骤时**必须保留这一嵌套结构**：外层门只放开为 `isCompleteNonStreamBody(res)`，完成标记与估算播种仍各自由 `StatusOK` 内层门控。若按初稿草图直接把完成标记提到 StatusOK 门，会回退 #1501 的截断体修复。

效果链：400+usage ⇒ 解析成功 ⇒ `finalUsageSeen=true` ⇒ `dropUnbilledEstimate` 走 `:752-762` 真实 usage 分支（`UsageDisposition=Observed`、`UsageSource=parsed`、`Non2xxRealUsageKept.Inc(1)`、不清零、不计费）⇒ pb3 写出 parsed token 行。**QA 验收判据 1/2 由此可达**。400 无 usage ⇒ `usagePresent=false` ⇒ 残留分支照旧（清零 + `Non2xxEstimateDropped`），#1493 语义不回归。

**边界（明确不做）**：SSE 流式非 2xx 帧内 usage 的解析——`QuotaUsageProcessor` 对非 2xx 仍返回 nil。流式 400/500 的 body 是 JSON 错误帧而非 usage 事件，属罕见形态；若未来出现，按同一模式在事件路径补（另行立项）。

### 步骤 2（已裁定，2026-11-12 采候选 2 并落地）：修订 #1480 设计文档的计数器条款，承认 #1493 双计数器口径

裁定理由（候选 2，接受）：

1. #1493 的恢复基于 §10 规范条款的明确否决，且比 #1480 更新（11-08 > 10-31），条款层级更高；
2. 若按 #1480 改名 `Non2xxUsageReported`，名字语义（"usage 已上报"）会**掩盖**残留被抑制的事实，与 #1493"残留不得上报"直接矛盾——两个计数器（`Non2xxEstimateDropped` 抑制行数 + `Non2xxRealUsageKept` 观测行数）语义正交、可分别告警，正是 #1410 以来"形状可分辨"的监控惯例；
3. 改名会再次打断已按实现名接入的观测（QA 现场两个实现名计数器已有导出数据）。

落地（已完成）：`2026-10-31-issue-1480-.../design-changes.md` 顶部新增状态更新横幅，并在 §二.步骤 2/步骤 3、§三.报表口径、§四.单测与集成测试表、§五.1/§五.2/§五.5/§五.6、§六.灰度建议、§七.1 改动清单及 §七.3 等被取代条款处逐处加注"⚠️ 已被 #1493 取代"；"非 2xx 记录用量"明确为**仅指真实 usage 观测**（source=parsed），估算残留不上报。E2E 断言（SC2201-TC005、SC3201-TC011）改用实现名 `NON2XX_REAL_USAGE_KEPT`，由 QA 侧复验时更新并解除 FAILED_PRODUCT。

### 步骤 3（必做）：单元测试

`bfe_modules/mod_ai_token_auth/` 新增/修订：

| 用例 | fixture | 期望 |
| --- | --- | --- |
| 非 2xx 带真实 usage | 400 + `usage{total:10,prompt:3,completion:7}` 完整 body（`isCompleteNonStreamBody` 形态）走完 read-response + finish | `finalUsageSeen=true`；`UsageDisposition=Observed`、`UsageSource=parsed`、`Non2xxRealUsageKept` +1；token 字段保留；**不计费**（余额不变）；`Non2xxEstimateDropped` 不增 |
| 非 2xx 无 usage（回归） | 400 无 usage | 残留清零、`UsageDisposition=None`、`Non2xxEstimateDropped` +1（既有用例改名后保留） |
| 2xx 语义不回归 | 200 带 usage / 200 无 usage | parsed / 估算镜像行为逐字节不变（既有全量用例） |
| 完成标记不泄漏 | 400 带 usage | `IsResponseCompleted()` 为 false（计费完成门控不被非 2xx 满足） |
| 截断体不回归（#1501） | 200 + `GetBodyAccessor` 失败/nil accessor（含压缩与非压缩） | 不置完成标记、不估算、不解析；既有 #1501 用例全量通过 |

落地（2026-11-12 已实施）：新增 `TestTokenReadResponseHandlerNon2xxRealUsage`（read-response 段：400+usage ⇒ `finalUsageSeen=true`、`IsResponseCompleted()=false`、三元组 3/7/10 逐字段断言，且鉴权期残留 `PromptTokens=25` 被解析值覆盖、2xx-only 估算播种不生效）与 `TestTokenRequestFinishHandler_Non2xxRealUsageFromResponseBody`（全链：Observed/parsed、`Non2xxRealUsageKept`+1、`Non2xxEstimateDropped`=0、RMB plan 余额不变）；非 2xx 无 usage 与 2xx 回归由既有全量用例覆盖。

集成测试同步（`bfe/tests`）：SC05 `TestTC19_Non2xxNoUsageFields` 臂 D（上游 409 + 错误体携带 usage）原反向钉住"错误体 usage 绝不进日志"（旧 200 门槛下成立），本修复后该形态成为 SC2201-TC005 的仓内复刻——臂 D 断言翻转为：日志三元组 7/11/18 逐字段等于上游 usage 对象、`ai_usage_source=parsed`、`ai_cost_value` 缺席、发送前后 Redis 余额相等（不计费）；臂 A/B（无真实 usage 的非 2xx）缺席断言不变。测试设计文档 `TC-19-非2xx响应不输出token字段.md` 的臂 D 预期、计费侧与实施备注已同步。

sys_design 同步（`bfe/docs/zh_cn/sys_design`）：`ai_access_log_fields.md` 字段 764 行与 v0.3.16 变更记录的"非 2xx 行缺席"两处补 SC2201-TC005 例外（错误体携带真实 usage 的非 2xx 行按 `parsed` 观测输出）；`rmb_quota.md` 7.4 的 `tokenReadResponseHandler` 代码摘录更新为当前结构（外层门控放开、`usagePresent` presence 判定、#1501 截断体 else 分支）并新增说明 3（本修复）、说明 4（#1414 presence 判定，旧 `UsedQuota>0` 摘录作废），7.5 计费矩阵"非 2xx"行与 #1493 说明补"例外已随本修复真正落地"、流式收集一节补"非流式路径不受 200 门槛限制"的交叉引用。其余 sys_design 文档（`ai_error_codes` / `claude_protocol_support` / `model_protocol_adapter` 等）经核查未描述非 2xx usage 解析口径，无需修改。

注：既有 `TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation`（`mod_ai_token_auth_test.go:3041`）已以手动播种覆盖 finish 侧真实 usage 分支，无需重复；本步骤新增的"非 2xx 带真实 usage"用例必须从 `tokenReadResponseHandler` 真实驱动（构造 `bfe_http.Response` 走 GetBodyAccessor 通路），才能堵住"解析通路缺失"这一真正缺口——这正是本次缺陷的成因。

### 步骤 4（部署核验）：消除 R3 矛盾

修复构建部署 QA 环境后重跑 SC2201-TC005 最小复现，直读三项：① 失败臂 `NON2XX_REAL_USAGE_KEPT` delta=1 且 `NON2XX_ESTIMATE_DROPPED` 不随该臂增长；② Report 400 行 token 来源为 parsed（与受控后端 `/hits`、响应体 usage 直读一致）；③ `AI_USAGE_INCONSISTENT` delta=0。若 Report 仍有非 parsed 来源 token，直读 `75ddd9ab` 构建的 `tokenReadResponseHandler`/`dropUnbilledEstimate` 与该构建实际分支，单独立项。

## 三、影响面与风险

- 改动文件：`mod_ai_token_auth.go`（一个 StatusOK 门控的精化：解析放开、完成标记与估算播种保持 2xx-only、#1501 截断体 else 分支原样保留）、`mod_ai_token_auth_test.go`（新增/修订用例）、SC05 集成测试 `sc05_access_log_ai_fields_test.go` 臂 D + 测试设计文档 TC-19（断言翻转）、`sys_design/ai_access_log_fields.md` 与 `sys_design/rmb_quota.md`（口径同步，见步骤 3 落地记录）、#1480 设计文档（已被 #1493 取代条款逐处加注，2026-11-12 随裁定落地）。
- 行为变化：仅"非 2xx 且 body 带真实 usage 对象"的完整 body 响应——从"残留清零不计数"变为"parsed 观测 + 计数 + 不计费"。该类行此前**无任何计数器**，不会产生重复计数；`Non2xxEstimateDropped` 基数中混有的此类行将迁移到 `Non2xxRealUsageKept`（告警阈值按两计数器之和近似过渡）。
- 计费零影响：非 2xx 在 finish 路径 `:443-445` 于计费前早退，`MarkFinalUsageSeen` 不改变任何扣费结果（验收判据：2xx 计费行逐字节不变，既有全量单测回归即证）。
- 与 #1493 的关系：本修复**不反转** #1493 的抑制语义，只是把"真实 usage"从残留分支里救出来——正是 #1493 注释（`:738-740` "Real usage parsed from the response is never dropped"）已声明但从未兑现的意图。
- 与 #1501 的关系：同一函数。放开解析门控后，`GetBodyAccessor` 失败/nil 的截断路径仍走既有 else 分支，不解析非 2xx 截断体；#1501 既有用例（含 PR #1504 二轮评审新增的断言）全量回归即证。

## 四、验收判据映射（对应 QA 评论）

1. 修复部署后失败臂使计数器 delta≥1：步骤 1 使 `NON2XX_REAL_USAGE_KEPT` 可达（裁定采候选 2，即实现名，已落地）；`AI_USAGE_INCONSISTENT` delta=0 由 total=in+out 的 parsed 三元组天然满足。
2. 失败行 token 来源语义：采候选 2 已裁定 ⇒ 仅 parsed 真实 usage 上报（source=parsed），残留行三字段缺席且 `Non2xxEstimateDropped` 随其增长；#1480 原文口径（source=failed 保留语义）已被 #1493 否决，不再复活。
3. 不回归：2xx 计费与 token 不变（既有单测全量）；4xx 鉴权拒绝行无后端命中（`AiUsageSuppressed` 语义不变）；`AiUsageInconsistent` 对漏网旧形态仍触发（检测器不改）；截断体不置完成标记（#1501 用例全量）。
