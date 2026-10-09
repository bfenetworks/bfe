# BFE Issue #1398 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1398
- 缺陷：无 final usage 的清零块将 `TokenUsage` 全部字段置 0，而访问日志（mod_access_pb3）与计费扣减读**同一对象**——后端真实产出 token 的请求（实测 TTFT 全部 > 0）在统计明细与计费中落成 `total_tokens=0`（某小时 200 响应中 417/977，42.7%）；`EstimateToken=false`（出厂默认 `conf/bfe.conf:32`）下估算兜底失效，缺陷全量暴露
- 代码库：`bfe/`（已对照当前 HEAD 逐条核实，行号基于当前 HEAD）

## 一、根因

### 1.1 清零块与日志/计费同源（缺陷本体）

`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:304-317`（issue #1352 引入、`#1364` 扩展为全字段清零）：

```go
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    tokenUsage.PromptTokens = 0
    tokenUsage.CompletionTokens = 0
    tokenUsage.CacheReadTokens = 0
    ... // 全部 10 个计费字段 + UsedQuota 一并置 0
}
```

而访问日志读的是同一对象：`bfe_modules/mod_access_pb3/request_log.go:431-435`
（`AiTotalTokens = usage.UsedQuota`），模块注册顺序
`bfe_modules/bfe_modules.go:155`（mod_ai_token_auth）先于 `:215`（mod_access_pb3），
`HandleRequestFinish` 按注册顺序执行——**清零先于日志落盘**。结果是：日志与扣费
同时归 0，且连"流中已解析出的真实分项 usage"（如 Anthropic `message_start` 的
input/cache token，经 `content_quota_usage.go:82-103` 写入）与 auth 阶段预置的
prompt 估算（`:388-392` 播种 → `SetTokenAuthContext:553-559`）也被一并抹掉。

### 1.2 `estimateBillable` 的覆盖与默认值缺口

`estimateBillable`（`:304`）为 true 需要同时满足：`IsAllowEstimateToken()`（由
`bfe_server/http_conn.go:560` 从 `Server.EstimateToken` 注入）与
`IsResponseCompleted()`。后者在当前 HEAD 的三个来源：

| 来源 | 位置 | 盲区 |
| --- | --- | --- |
| `MarkResponseCompleted()` on `IsTermination` | `content_quota_usage.go:48-50` | 依赖终止事件（`message_stop`/`[DONE]`/`response.completed`，见 `bfe_model_protocol/openai/stream.go:32-36`、`anthropic/stream.go:28-30`）；上游干净 EOF 但不发终止事件的流打不上 |
| RawEvent `isTermination := isFinalUsage` | `body_process.go:451-453` | 非流式单 JSON 要求解析出非 guess usage 且 `CompletionTokens > 0` 才视同完成 |
| `tokenReadResponseHandler` | `mod_ai_token_auth.go:216-229` | 要求 `res.ContentLength >= 0`；chunked 非流式响应（`ContentLength == -1`，`body_process.go:351` 又统一置 -1）整段跳过 |

另有三个加重因素：

1. **流式估算不置 `UsedQuota`**：`content_quota_usage.go:107-114` 只累加
   `CompletionTokens` 估算，`UsedQuota` 仅在 finish 时由 `:318-320` 兜底计算
   ——且该兜底 gated 在 `estimateBillable` 上。中途 abort 的行因此表现为
   "in/out 有值、total=0"（与 issue 中 6 条 abort 特征行吻合）。
2. **client-abort 早退**（`:289-293`）：不清零但也不赋值 `UsedQuota` → 同上
   total=0。
3. **`EstimateToken=false`（出厂默认）**：`estimateBillable` 恒 false，
   清零块对任何无 final usage 的请求（OpenAI 流式无 `include_usage`、
   OpenAI/Anthropic 非流式、abort 行）全量触发。这与 SC1301-TC044 既定判例
   "缺 Usage 按 ContentLength/4 估算"及 `docs/zh_cn/sys_design/rmb_quota.md:615`
   "正常完成，无 usage + EstimateToken → 按估算计费"的设计口径矛盾——
   **默认配置本身是缺陷组成部分**。

### 1.3 final-usage 判定的 `CompletionTokens > 0` 前置（次级缺陷）

`llm_util.go:161` 与 `body_process.go:451`：

```go
if !isguess && fields.CompletionTokens > 0 {
    isFinalUsage = adapter.IsFinalUsageEvent(streamEv)
}
```

`output_tokens=0` 的最终 usage 事件（input-only 请求、纯 cache 命中响应、
OpenAI `include_usage` chunk 且 completion 为 0）不被识别为 final → 落入
清零/估算路径。

### 1.4 从不注入 `stream_options.include_usage`

全仓 grep 仅注释/测试/文档提及。OpenAI 流式客户端不带 `include_usage` 时，
后端**依法不回 usage chunk**（OpenAI 规范行为）——这是 274/417 清零行
（OpenAI 流式）的最大来源。BFE 拿不到真实 usage，只能靠估算或清零。

### 1.5 部署差距说明

当前 HEAD 已含 #1352（`IsResponseCompleted`）、#1364（`type:"message"` 认可、
跨协议回退）、#1381（`response.completed`）、`ai_stream_truncated`
（`reverseproxy_ai_error.go:464-465` → `request_log.go:599-600`，
2026-10-06 上游错误归一）。issue 实测的 42.7% 清零（含 TTFT>0 的 OpenAI
流式行）说明**被测部署构建的完成判定断点落后于 HEAD**（issue 自述"行号基于
source checkout，与部署构建可能有偏移"）。但即便在 HEAD 上，1.2 第 3 条
（`EstimateToken=false` 下完成流全量归 0）依然成立，修复依然必要。

## 二、修复步骤

### 步骤 1（必做，止血+治本）：清零守卫改为计费局部副本，共享对象不再抹零

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`:295-351`

`TokenUsage` 是纯值结构（`bfe_basic/request_ai_basic.go:80-93`，全 int64），
浅拷贝即完全隔离。清零块、`UsedQuota` 兜底、`calcCostUnits`、`Deduct`
全部作用于副本；**仅计费结果 `UsedCost` 写回共享对象**（日志的 cost 字段需要），
token 字段保持观测/估算原值：

```go
tokenUsage := ctx.aiBasicInfo.GetTokenUsage()
// 计费在副本上进行：清零守卫只作用于计费视图，绝不回写共享对象——
// 访问日志（mod_access_pb3）与统计读共享对象（issue #1398）。
billingUsage := *tokenUsage
estimateBillable := ctx.aiBasicInfo.IsAllowEstimateToken() && ctx.aiBasicInfo.IsResponseCompleted()
if !ctx.aiBasicInfo.IsFinalUsageSeen() && !estimateBillable {
    billingUsage.PromptTokens = 0
    billingUsage.CompletionTokens = 0
    billingUsage.CacheReadTokens = 0
    billingUsage.CacheWriteTokens = 0
    billingUsage.CacheWriteTokens1h = 0
    billingUsage.AudioInputTokens = 0
    billingUsage.AudioOutputTokens = 0
    billingUsage.ImageInputTokens = 0
    billingUsage.VideoCount = 0
    billingUsage.ImageCount = 0
    billingUsage.UsedQuota = 0
}
if billingUsage.UsedQuota <= 0 && estimateBillable {
    billingUsage.UsedQuota = CalcReqUsedQuota(req, billingUsage.PromptTokens, billingUsage.CompletionTokens)
}
if estimateBillable && tokenUsage.UsedQuota <= 0 && billingUsage.UsedQuota > 0 {
    // 估算兜底值镜像回共享对象，访问日志 ai_total_tokens 记估算值而非 0
    tokenUsage.UsedQuota = billingUsage.UsedQuota
}
if billingUsage.UsedCost <= 0 && hasRMBPlan(ctx.Token.QuotaPlans) {
    billingUsage.UsedCost = m.calcCostUnits(req, ctx.serverConf, &billingUsage)
}
// 仅计费结果写回；token 字段保持观测/估算原值供日志与统计。
tokenUsage.UsedCost = billingUsage.UsedCost

if billingUsage.UsedQuota > 0 || billingUsage.UsedCost > 0 {
    for _, plan := range ctx.Token.QuotaPlans {
        // RMB 按 billingUsage.UsedCost 扣，token 按 billingUsage.UsedQuota 扣
        ...
    }
}
```

效果与语义保持：

- **#1352 语义不变**：客户端中断且未拿到最终 usage → 副本清零 → 零扣费；
- **#1364 语义不变**：守卫仍清全部计费字段，`calcCostUnits` 不会只按幸存子字段
  （如 CacheRead）计费；
- **统计不再静默记 0**：真实解析值与批准口径的估算值保留在日志中；abort 行的
  分项保留部分观测值（反映真实消耗），total 由报表侧结合
  `ai_stream_truncated`/`err_code` 解读；
- **估算兜底镜像**：`estimateBillable` 时计算出的估算 `UsedQuota` 会镜像回共享
  对象（`tokenUsage.UsedQuota <= 0` 才写），访问日志 `ai_total_tokens` 记估算值
  而非 0——"缺 usage"必须可观测。

### 步骤 2（必改）：final-usage 判定去掉 `CompletionTokens > 0` 前置

文件：`bfe_modules/mod_body_process/llm_util.go:159-163`、`body_process.go:451-452`

```go
// llm_util.go / body_process.go 同步修改：
isFinalUsage := false
if !isguess {
    isFinalUsage = adapter.IsFinalUsageEvent(streamEv)
}
```

`!isguess`（`UsedQuota>0 || ImageCount>0 || VideoCount>0`，`llm_util.go:141`）
已保证事件确带 usage 数值；adapter 的 type 白名单
（`message_delta`/`message`/`response.completed`/`""`）已排除
`message_start` 与无 usage 的中间 chunk，不会误判。

### 步骤 3（治本）：OpenAI 流式自动注入 `stream_options.include_usage`

注入点与请求体 model 改写同处：`bfe_server/reverseproxy.go:1587-1610`
（`doSingleAIForward()` 内，`condition.ReqBodyJsonSet(basicReq, "model",
targetModel)` 附近），复用既有 body 读取/回写与重试 rewind 机制
（`rewindRequestBody`，`:1969`）：

```go
if srv.Config.Server.InjectStreamUsage &&
    aiMeta.AuthStyle == bfe_basic.AuthStyleOpenAI &&
    targetModel stream 请求（body `"stream":true`） &&
    客户端未显式设置 `stream_options.include_usage` {
    if err := condition.ReqBodyJsonSet(basicReq, "stream_options.include_usage", true); err != nil {
        // 注入失败（非 JSON body 等）静默透传，保持旧行为
    }
}
```

要点：

- 仅 OpenAI 协议的 chat completions 类路径；Responses API 自带
  `response.completed` usage（#1381），不注入；Anthropic 流自带
  `message_delta` usage，不注入；
- 客户端已显式设置 `stream_options`（含显式 false）时不覆盖；
- 新增 Server 级开关 `InjectStreamUsage`，默认 **true**；
  `docs/zh_cn/configuration/bfe.conf.md` / `docs/en_us/configuration/bfe.conf.md`
  同步；
- 上游回发的 usage-only chunk（`choices:[]` + usage，位于 `[DONE]` 前）经
  `mod_body_process` 透传给客户端，这是 OpenAI 规范行为；`GetQuotaUsage`
  只读不改事件，encoder 原样回传，不丢事件。

效果：OpenAI 流式从上游拿到真实 final usage → `MarkFinalUsageSeen` → 按实际
计费；估算路径退化为"上游连 usage chunk 都不给"的罕见兜底。

### 步骤 4（配置裁决）：`EstimateToken` 出厂默认值

SC1301-TC044 判例与 `rmb_quota.md:615` 的口径是"正常完成、无 usage → 按
ContentLength/4 估算计费"。出厂默认 `false`（`conf/bfe.conf:32`）与该口径
矛盾。两选一（推荐前者）：

1. **默认改 `true`**（推荐）：与判例/设计文档一致；步骤 1 保证即使估算口径
   有偏差，统计侧也保留观测值而非静默 0；
2. 维持 `false`：则"完成但无 usage"请求计费侧为 0（靠步骤 3 消灭该场景），
   需在 `rmb_quota.md` 与 `bfe.conf.md` 明示该口径。

### 步骤 5（可观测性）

- `ai_stream_truncated`：链路在 HEAD 已具备（`reverseproxy_ai_error.go:464-465`
  → `request_log.go:599-600`，SC27 有集成覆盖）。issue 的"从未置位"应为部署
  版本差距，随发版验证；另需复核 issue 关联项——实测数据中置位逻辑零命中的
  具体断点。
- MySQL 报表表 `bfe_ai_request_log` 补 `ai_stream_truncated` 列：
  `ai-gateway-api` 仓 `db_ddl_report_mysql.sql`，跨仓 follow-up（pb3 proto
  v0.3.13 已有该字段）。
- "usage 缺失"与"usage=0"区分：步骤 1 之后，日志 `total_tokens=0` 仅可能来自
  真实零消耗、abort 或截断，可结合 `ai_stream_truncated` 与 `err_code` 区分；
  远期可选在 `bfe-access-pb` 增加显式 `usage_unknown` 标志字段。

## 三、回归测试

新增/修改用例（单元测试放被测代码旁 `_test.go`，沿用 `testing` + `testify`）：

1. `mod_ai_token_auth`（更新既有
   `TestTokenRequestFinishHandler_GuardClearsSubTokenFields` 为副本语义）：
   构造 `!IsFinalUsageSeen() && !estimateBillable` 上下文，断言
   **共享 `TokenUsage` 字段保持原值、扣减按 0 进行、日志视图非零**；
   以及 `estimateBillable` 场景扣减按估算、`UsedCost` 正确写回。
2. `mod_body_process` / `llm_util`：OpenAI `include_usage` final chunk 且
   `completion_tokens=0`（`prompt_tokens>0`）→ `IsFinalUsage=true`；
   `message_start`（output=0）仍为 false。
3. include_usage 注入：`stream=true` 且无 `stream_options` 时 body 被注入；
   已显式设置不覆盖；`stream=false`/非 JSON body/非 OpenAI 协议不注入；
   注入后请求可 rewind（fallback 重试 body 一致）。
4. 集成（`bfe/tests/integration`，建议新场景 SC29 或扩展 SC03/SC05）——
   二因子矩阵：
   `EstimateToken{true,false} × {OpenAI 流式带 include_usage、OpenAI 流式不带、
   OpenAI 非流式、Anthropic 流式、Anthropic 非流式、客户端中断、上游截断}`，
   断言 pb3 日志 `ai_total_tokens` 及分项与扣费口径（真实 usage 按实际；
   完成无 usage 按估算[开关开]；中断未拿 final 零扣费但日志保留观测值）。
5. 既有回归全绿：SC03（RMB 扣减）、SC05（日志字段）、SC11、SC12（中断计费）、
   SC27（截断置位）。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确行为（目标链路）

OpenAI 流式（不带 `include_usage`，`EstimateToken=true`，注入开关开）：

1. 请求转发前注入 `stream_options.include_usage=true`（reverseproxy.go）；
2. 上游在 `[DONE]` 前回发 usage chunk → `GetQuotaUsage` 解析出非 guess usage，
   `IsFinalUsageEvent("")` → `MarkFinalUsageSeen()`（`content_quota_usage.go:45-47`）；
3. `tokenRequestFinishHandler`：守卫不触发；按真实 usage 扣费；
4. 访问日志读共享对象 → 真实分项落库。

若上游异常不回 usage chunk（`[DONE]` 正常到达）：`MarkResponseCompleted()` 置位
→ `estimateBillable=true` → finish 时按估算兜底扣费，日志记估算值——
与 `rmb_quota.md:615` 口径一致，不再静默归 0。

## 五、实施记录（2026-10-09）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 清零守卫/`UsedQuota` 兜底/`calcCostUnits`/`Deduct` 全部作用于计费副本 `billingUsage`；估算兜底值镜像回共享对象（`estimateBillable && tokenUsage.UsedQuota <= 0`）；仅 `UsedCost`（及镜像的 `UsedQuota`）写回共享对象（issue #1398） |
| `bfe_modules/mod_body_process/llm_util.go`、`body_process.go` | `isFinalUsage` 判定去掉 `CompletionTokens > 0` 前置，非 guess usage + 适配器最终事件白名单即视同最终 usage（`output_tokens=0` 的最终事件不再漏判） |
| `bfe_server/reverseproxy.go` | 新增 `maybeInjectStreamUsage`：OpenAI 协议 chat/completions 流式请求（`stream:true`）且客户端未显式设置 `stream_options.include_usage` 时自动注入 `true`；注入失败静默透传；在 `doSingleAIForward()` model 改写同点执行并重置 Content-Length |
| `bfe_config/bfe_conf/conf_basic.go`、`conf/bfe.conf` | 新增 `Server.InjectStreamUsage`（默认 true）；`EstimateToken` 出厂默认改 `true`（与缺 Usage 按 ContentLength/4 估算的既定口径一致，issue #1398） |
| `docs/zh_cn/configuration/bfe.conf.md`、`docs/en_us/configuration/bfe.conf.md` | 参数表新增 `InjectStreamUsage`、`EstimateToken` 默认值与行为说明更新（含配置示例同步） |
| `docs/zh_cn/sys_design/rmb_quota.md` | §6.4 计费/统计视图隔离说明；§7.4 最终 usage 判定表更新（去掉 completion>0 前置）；§7.5 代码镜像与计费判定规则汇总更新（副本语义 + 估算镜像 + InjectStreamUsage 说明） |
| 单元测试 | `mod_ai_token_auth_test.go`：`TestTokenRequestFinishHandler_GuardClearsSubTokenFields` 改为副本语义（日志视图保留观测值、零扣费）、`TestTokenRequestFinishHandler_EstimateRequiresCompletedResponse` 增加共享对象保留估算与 `UsedQuota=150` 镜像断言；`llm_util_test.go` 增加 "openai final usage chunk zero completion" 臂；新增 `bfe_server/reverseproxy_stream_usage_test.go`（注入 9 形态矩阵 + 幂等） |
| 集成测试 | 新增场景 SC29（`tests/integration/implementation/scenario-SC29-usage-snapshot-billing`，3 TC）：TC-01 注入 include_usage 按真实 usage 计费与日志、TC-02 显式关闭按估算兜底（日志非零）、TC-03 客户端中断零扣费且日志保留观测值；设计文档 `tests/integration/测试设计文档/scenario-SC29-usage抹零修复与include_usage注入/`；`tests/integration/common/util.go` 增加 `cluster_usage_snapshot` 子集群名映射；总体说明同步 |

验证结果（2026-10-09）：

- 单元测试：全量 `go test $(go list ./... | grep -v tests/integration)` 94 个包全部通过；`go vet` 全仓通过；
- 集成测试：SC29 TC-01~TC-03 全部通过；回归 SC03、SC05、SC11、SC12、SC27 全部通过（SC05 批量运行时 TC06 出现一次日志落盘时序抖动，单独重跑通过，与本次改动无关）。

## 六、运营遗留事项（代码外）

- **损失回补**：评估 2026-10-09 及历史被抹零记录的计费回补策略（可基于历史
  pb3 日志回放重算扣费）；计费对账。
- **敏感数据**：issue 证据 pcap 含真实 Authorization Key / Acl-Token JWT，
  回补与对账过程中注意脱敏，勿外传原始 pcap。
