# BFE Issue #1401 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1401
- 缺陷（facet-2）：Kimi（kimi-for-coding）在客户端不带 `stream_options.include_usage` 时，把最终 usage **嵌在流式 finish chunk 的 `choices[0]` 内**（`choices[0].usage`，顶层无 `usage`）；`ParseOpenAIUsageFields` 只 gjson 顶层 `usage.*`，全文无 `choices[0].usage` 回退 → 真实 usage（实测 prompt=77207 / completion=168 / total=77375 / cached=73472）解析为全 0 → `isguess=true` → 不计 final usage。`EstimateToken=true` 下日志/计费落**估算伪造值**；`EstimateToken=false` 下日志落 `(0, -1, 0)` 脏数据（负 output tokens）且计费副本被清零按 0 扣费。抓包窗口 10 条连接 9 条为该形态（10/10 响应 200、SSE 正常、`[DONE]`、干净 FIN）
- 与 #1398 的关系：#1398 修了"清零块抹零 + 不注入 include\_usage"；facet-2 是 **parser 盲区**，不在 #1398 修复范围——即便清零块已隔离到计费副本，共享对象的观测值仍是 0/估算值。注入（`maybeInjectStreamUsage`）只是降低触发概率的缓解：实测 deepseek honoring 注入，**kimi-for-coding 忽略注入**（注入已生效的部署上 389/458 条 openai 流式仍 total=0 且无一行估算镜像形态），故 parser 回退是唯一可靠的治本修复
- 代码库：`bfe/`（已对照当前 HEAD 逐条核实，行号基于当前 HEAD）

## 一、根因

### 1.1 parser 盲区（缺陷本体）

`bfe_model_protocol/utils/usage_parse.go:110-139` `ParseOpenAIUsageFields` 的提取链：

1. 主链：`usage.{total_tokens,prompt_tokens,completion_tokens,...}`（`:111`）
2. Responses API 回退：`response.usage.*` / `usage.input_tokens_details` 门控（`:119-136`，#1381）

**全文无任何 `choices[N].usage` 回退**。Kimi finish chunk 形态：

```json
{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls",
  "usage":{"prompt_tokens":77207,"completion_tokens":168,"total_tokens":77375,"cached_tokens":73472,...}}]}
```

顶层 `usage` 不存在 → 主链与 Responses 回退全部读 0 → 返回全零 `UsageFields`。注意嵌套 usage 的 cache 字段名是 **`cached_tokens`**（非 `cache_read_tokens`），回退必须一并映射，否则 cache 分项仍丢失。

### 1.2 全链路推演（当前 HEAD 逐环）

1. `mod_body_process/llm_util.go:135-145` `GetQuotaUsage`：`fields.UsedQuota==0 && ImageCount==0 && VideoCount==0` → `isguess=true`，`curtoken=EstimateContentToken(chunk)`（chunk 字节/4）。
2. `llm_util.go:166`：`isFinalUsage = !isguess && adapter.IsFinalUsageEvent(...)` → isguess=true 恒为 **false**。OpenAI 适配器白名单本已接受无顶层 type 的 chunk（`bfe_model_protocol/openai/stream.go:48-53` 含 `ev.Type == ""`），缺的只是"解析出非 guess 数值"这一前置——补上 parser 回退后该 chunk 自动被识别为 final usage，`llm_util.go` / `content_quota_usage.go` / `mod_ai_token_auth.go` 均无需改动。
3. finish（`mod_ai_token_auth.go:312`）：`IsFinalUsageSeen()=false`：
   - `EstimateToken=true`：`estimateBillable=true`（`[DONE]` → `MarkResponseCompleted`）→ 估算兜底 + 镜像（`:318-320` 及 #1398 副本语义）→ 日志/计费落 **估算伪造值**（prompt=请求体/4≈76K 纯属巧合接近真实，completion=chunk/4 与真实 168 无 causal 关系，reasoning/tool-call JSON 大块导致**多扣**）；
   - `EstimateToken=false`：estimateBillable 恒 false → 计费副本进清零块按 0 扣费；共享对象 `CompletionTokens` 保持 auth 预置哨兵 `-1`（`mod_ai_token_auth.go:576` `COMPLETION_TOKENS_UNKNOWN`），`mod_access_pb3/request_log.go:434` 原样写 `AiOutputTokens=-1` → **负 output tokens 脏数据**。

### 1.3 开关无关性（症状形态对照）

| 环节 | EstimateToken=true | EstimateToken=false |
| --- | --- | --- |
| 日志三元组 (in/out/total) | (≈76K, 累加估算, 估算和)——伪造 | (0, **-1**, 0)——脏数据 |
| 计费 | 按估算扣（completion 估算显著大于真实 → 多扣） | 真实 77375 按 **0** 扣 → 收入流失 |
| facet-2 暴露 | 被估算掩盖为不可信假数 | 全量暴露 |

修复（parser 回退）对两开关等效必要。

### 1.4 触发条件合取（为什么仅"OpenAI + 不带 stream\_options 的客户端 + Kimi"组合发病）

1. **上游非标准格式**：上游把 usage 嵌在 `choices[N].usage`（主流协议均无此形态，Kimi 为唯一直接来源；OpenAI 官方带 include\_usage 发顶层空 choices usage chunk，不带则完全无 usage——那是 #1398 facet-1 形态）；
2. **客户端不带 stream\_options**：TRAE 等客户端不发 → Kimi 走缺省嵌套格式；
3. **BFE 未补上**：parser 无回退，且注入被上游忽略。

前两条在网关外部不可控，**仅第三条在 BFE 侧可修**。

### 1.5 部署差距说明（重放裁决结论）

issue 的三方复核（pcap 重组字节 × pb3 日志 × v1.8.8.1 tag 源码重放）结论：tag 代码 + 真实字节下 11/11 条流 `estimateBillable=true`、镜像后 `AiTotalTokens` 必为非零估算——**tag 源码本身无"total 恒 0"缺陷**；166 部署二进制为混合构建（auth 侧含 #1398、含注入逻辑且 conf 两键生效，但 `mod_body_process` 完成标记链路（#1352/#1364）缺失或为旧实现）→ 镜像永不生效 → 389/458（85%）openai 流式按 0 扣费。该部分属**运营修复**（全量干净构建），见 §六；本方案代码修复针对 facet-2 parser 盲区本体。

## 二、修复步骤

### 步骤 1（必做，治本）：`ParseOpenAIUsageFields` 增加 `choices[N].usage` 回退

文件：`bfe_model_protocol/utils/usage_parse.go` `ParseOpenAIUsageFields`（`:110-139`）

在既有 Responses API 回退（`:119-136`）之后追加同级回退，保持"主链优先、回退仅补零"的既有风格：

```go
if fields.PromptTokens == 0 && fields.CompletionTokens == 0 {
    // 既有 Responses API 回退（issue #1381）不动……

    // Kimi nested fallback (issue #1401): some OpenAI-compatible
    // upstreams (kimi-for-coding) embed the final usage inside the
    // finish chunk's choice — choices[i].usage — when
    // stream_options.include_usage is absent or ignored. The top-level
    // usage chain reads all-zero there, silently demoting real usage to
    // guess/estimate (or, with EstimateToken=false, to zero billing and
    // negative output tokens in the access log).
    if nested, ok := parseNestedChoiceUsage(data); ok {
        fields = nested
    }
}

// parseNestedChoiceUsage extracts usage embedded in choices[i].usage
// (Kimi finish-chunk shape). Only finish chunks carry it, so intermediate
// content chunks never false-positive. Cache-read maps cached_tokens /
// prompt_tokens_details.cached_tokens (Kimi field names).
func parseNestedChoiceUsage(data []byte) (UsageFields, bool) {
    n := gjson.GetBytes(data, "choices.#").Int()
    for i := int64(0); i < n; i++ {
        prefix := fmt.Sprintf("choices.%d.usage", i)
        if !gjson.GetBytes(data, prefix).Exists() {
            continue
        }
        nested := parseOpenAIUsageFieldsWithPrefix(data, prefix, "prompt_tokens", "completion_tokens")
        if nested.PromptTokens == 0 && nested.CompletionTokens == 0 {
            continue
        }
        if nested.CacheReadTokens == 0 {
            nested.CacheReadTokens = gjson.GetBytes(data, prefix+".cached_tokens").Int()
        }
        if nested.CacheReadTokens == 0 {
            nested.CacheReadTokens = gjson.GetBytes(data, prefix+".prompt_tokens_details.cached_tokens").Int()
        }
        return nested, true
    }
    return UsageFields{}, false
}
```

要点：

- **门控与既有回退一致**：仅当主链（及 Responses 回退）prompt/completion 全 0 时触发；顶层 usage 存在时永远主链优先（不会改写 honoring 上游的真实值）；
- **逐 choice 扫描**而非硬编码 `choices.0`（chat completions `n>1` 时 usage 可能挂在其他 index；数组通常长度为 1，循环成本可忽略）；中间 content chunk 无 `choices[i].usage`，天然不误判；
- **`cached_tokens` 映射**：Kimi 嵌套 usage 的 cache 字段名非 `cache_read_tokens`，必须补映射，否则 cache 分项仍丢（计费单价不同，影响 RMB 成本拆分）；
- **跨协议组合器自动受益**：`ParseUsageFieldsCrossProtocol`（`:153-184`）首调 `ParseOpenAIUsageFields`；Anthropic/Gemini body 无 `choices` 键，不会误吞；#1364 跨协议回退顺序不变；
- **下游零改动**：回退命中后 `UsedQuota>0` → `isguess=false`（`llm_util.go:141`）→ `isFinalUsage=true`（`llm_util.go:166`，适配器白名单已含 `type==""`）→ `MarkFinalUsageSeen` → finish 按真实 usage 扣费、日志记真实分项——现有累加/守卫/副本链路原样复用；
- 透传语义不变：`GetQuotaUsage` 只读不改事件，Kimi finish chunk 原样回传客户端。

### 步骤 2（必改）：`EstimateToken` / `InjectStreamUsage` 程序默认值 true

文件：`bfe_config/bfe_conf/conf_basic.go` `ConfigBasic.SetDefaultConf`（`:86-114`）

```go
cfg.EstimateToken = true
cfg.InjectStreamUsage = true
```

机制：`BfeConfigLoad` 先 `SetDefaultConf` 再 `gcfg.ReadFileInto`（`bfe_config_load.go:56-59`）——默认值先行、conf 显式键仍可覆盖（显式 `false` 生效）。消除"二进制升级不带新 conf 时修复静默失效"的陷阱（issue 中 166 排障已实证该陷阱的排查成本；两键当前为 Go 零值 false 兜底）。

同步：`docs/zh_cn/configuration/bfe.conf.md` / `docs/en_us/configuration/bfe.conf.md` 默认值列与说明；`conf/bfe.conf:33/38` 已是 true 无需改。

### 步骤 3（防御性，小额）：日志边界钳制负 output tokens

文件：`bfe_modules/mod_access_pb3/request_log.go:432-435`

```go
out := usage.CompletionTokens
if out < 0 { // COMPLETION_TOKENS_UNKNOWN sentinel, e.g. EstimateToken=false and no usage seen
    out = 0
}
reqLog.AiOutputTokens = proto.Int64(out)
```

步骤 1 之后该场景仅剩"完全无 usage 的流"（更罕见），但 `-1` 脏数据会污染按 `ai_output_tokens` 聚合的报表/回补重算，钳制成本一行。内部哨兵语义（auth 预置 -1、估算累加从 -1 起步）不变，仅在日志出口sanitize。

### 步骤 4（验证项）：注入有效性复抓

注入实现已在 HEAD（`bfe_server/reverseproxy.go:1530-1570` `maybeInjectStreamUsage`，调用点 `:1677`，#1398）。步骤 1 上线后复抓"TRAE → 网关 → Kimi"同场景，断言响应流出现 `"choices":[],"usage":{...}` 顶层 chunk：

- 若出现 → Kimi honoring 注入，双保险生效；
- 若仍无（与 issue 实测一致）→ 确认 Kimi 忽略 include\_usage，**parser 回退是唯一可靠来源**，注入仅为 honoring 上游保留。

### 步骤 5（回归覆盖缺口补齐）：嵌套 usage mock 探针

SC1401-TC022/TC023 的真实 usage 探针为 Anthropic 非流式，**不覆盖 Kimi 嵌套 `choices[0].usage` 形态**。需新增 OpenAI SSE mock（finish chunk 带 `choices[0].usage`，含 `cached_tokens`），双开关各断言网关记录**真实 usage 而非估算/清零/负值**（见 §三）。

## 三、回归测试

新增/修改用例（单元测试放被测代码旁 `_test.go`，沿用 `testing` + `testify`）：

1. `bfe_model_protocol/utils/usage_parse_test.go`：
   - Kimi finish chunk（`choices[0].usage`：`prompt_tokens/completion_tokens/total_tokens/cached_tokens`）→ 全字段非零、`UsedQuota=total_tokens`、`CacheReadTokens=cached_tokens`；
   - 中间 content chunk（`choices[0].delta` 无 usage）→ 仍 guess（回退不误判）；
   - 顶层 usage 与嵌套 usage 并存 → 主链优先；
   - `choices` 多元素、usage 挂在非 0 index → 命中；
   - Responses API body（`response.usage`）与 Anthropic body 不回退（跨协议不回退回归）；
   - `prompt_tokens_details.cached_tokens` 次级 cache 映射。
2. `bfe_modules/mod_access_pb3`：负 `CompletionTokens` 钳制为 0（若实施步骤 3）。
3. 集成（`bfe/tests/integration`，扩展 SC29 或新增场景）——双开关矩阵：
   `EstimateToken{true,false} × OpenAI 流式（Kimi 嵌套 usage finish chunk）`，断言：
   - pb3 日志 `ai_total_tokens` / 分项 = mock 真实值（非估算、非 0、非 -1）；
   - 计费按真实 usage 扣减（`cache_read` 分项参与 RMB 成本拆分）；
   - 既有 SC29 TC-01~03（include\_usage 注入、显式关闭估算兜底、客户端中断）全绿。
4. 既有回归全绿：SC03（RMB 扣减）、SC05（日志字段）、SC11、SC12（中断计费）、SC27（截断置位）、SC29（#1398 快照隔离）。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确行为（目标链路）

Kimi 形态 OpenAI 流式（客户端不带 stream\_options，上游忽略注入，任一 EstimateToken 取值）：

1. 中间 chunk：`choices[i].usage` 不存在 → 回退不触发，行为同现状（估算累加仅开关开时）；
2. finish chunk：`parseNestedChoiceUsage` 解析出真实 usage（77207/168/77375/cache 73472）→ `isguess=false`、`IsFinalUsage=true` → `MarkFinalUsageSeen()` + `[DONE]` → `MarkResponseCompleted()`；
3. `tokenRequestFinishHandler`：`IsFinalUsageSeen()=true` → 守卫不触发、估算兜底不触发 → 按真实 usage 扣费（RMB 按真实分项算价）；
4. 访问日志读共享对象 → 真实分项落库，`ai_total_tokens=77375` 而非估算伪造值/0/负值；
5. 客户端收到的 SSE 流逐字节不变（parser 只读）。

## 五、实施记录（2026-10-09）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/utils/usage_parse.go` | `ParseOpenAIUsageFields` 增加 `choices[N].usage` 回退（issue #1401）：新增 `parseNestedChoiceUsage`——逐 choice 扫描、主链/Responses 回退全 0 才触发、空 usage 对象跳过；cache-read 补 `cached_tokens` / `prompt_tokens_details.cached_tokens` 映射（Kimi 字段名）；下游 `llm_util`/`content_quota_usage`/`mod_ai_token_auth` 零改动 |
| `bfe_config/bfe_conf/conf_basic.go` | `ConfigBasic.SetDefaultConf` 新增 `EstimateToken = true`、`InjectStreamUsage = true` 程序默认值（issue #1401）：默认先行于 `gcfg.ReadFileInto`（`bfe_config_load.go:56-59`），conf 显式配置仍可覆盖，消除"二进制升级不带新 conf 修复静默失效"陷阱 |
| `docs/zh_cn/configuration/bfe.conf.md`、`docs/en_us/configuration/bfe.conf.md` | `EstimateToken`/`InjectStreamUsage` 默认值列更新为"issue #1401 起为程序默认值，conf 显式配置可覆盖" |
| `bfe_modules/mod_access_pb3/request_log.go` | 日志出口钳制负 `CompletionTokens`（`COMPLETION_TOKENS_UNKNOWN` 哨兵 -1）为 0（issue #1401），消除 `EstimateToken=false` 且无 usage 时的负 output tokens 脏数据；内部哨兵语义不变 |
| 单元测试 | `usage_parse_test.go` 新增 8 用例（嵌套 finish chunk 全字段/`cached_tokens` 映射、中间 chunk 不误判、主链优先、非 0 index、details cache 次级映射、空 usage 对象、跨协议组合器命中、Anthropic 不回退）；`request_log_test.go` 新增 `TestReqAiInfoGenNegativeCompletionTokensClamped`（-1→0、正常值原样通过） |
| 集成测试 | 新增场景 SC30（`tests/integration/implementation/scenario-SC30-nested-choice-usage-billing`，2 TC）：Kimi 形态嵌套 usage mock（实证数字 77207/168/77375/cached 73472），`EstimateToken{true,false}` 双开关矩阵——注入已生效但上游忽略（精确复刻 facet-2 触发条件），断言按真实 usage 计费（77207×300+168×900=23313300 定点）与 pb3 日志真实分项（非估算/非 0/非 -1）、`cached_tokens` 落 `ai_cache_read_tokens`、嵌套 chunk 原样透传；设计文档 `tests/integration/测试设计文档/scenario-SC30-Kimi嵌套usage解析/`；`tests/integration/common/util.go` `clusterSubName` 新增 `cluster_nested_usage` 映射 |

验证结果（2026-10-09）：

- 单元测试：全量 `go test $(go list ./... | grep -v tests/integration)` 全部通过（exit 0）；`go vet ./...` 全仓通过（exit 0）；
- 集成测试：SC30 TC-01/TC-02 全部通过（实施后首次运行即触发 bfe 二进制按工作区改动重建，二进制 mtime 22:45:22 晚于源码改动，确认测试的是含修复的构建）；回归 SC03、SC05、SC11、SC12、SC27、SC29 全部通过。

## 六、运营遗留事项（代码外）

- **全量干净构建替换 166 移植二进制**：issue 重放裁决判定 166 为混合构建（缺 #1352/#1364 的 `mod_body_process` 完成标记链路，镜像永不生效，389/458 openai 流式按 0 扣费）。用 tag 完整构建，勿选择性挑 hunk；发布前运行探针验证（发一条 Kimi 流式请求，pb3 日志 total 应为非零估算值；步骤 1 上线后应为真实值）。
- **构建指纹**：发布流程在启动日志输出 git commit/build id，避免"声称基于某 tag"但行为不符的排障成本。
- **计费回补评估**：166 当前构建 85% openai 流式按 0 扣费的窗口，评估回补策略（可基于历史 pb3 日志回放重算）；注意 issue 的 facet-1 窗口（42.7% 抹零）另见 #1398 遗留事项。
- **注入有效性**：按 §二步骤 4 复抓确认 Kimi 对 include\_usage 的支持度。
- **敏感数据**：证据 pcap 含真实 Authorization Key / Acl-Token JWT，回补与对账过程需脱敏，勿外传原始 pcap。
