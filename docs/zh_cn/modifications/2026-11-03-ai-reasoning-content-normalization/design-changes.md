# DeepSeek 思考模式 `reasoning_content` 请求侧归一化

## 1. 背景

DeepSeek 思考模式模型对 chat 请求有一条单向校验：**"最后一条 user 消息之后"的 assistant 消息必须携带 `reasoning_content` 字段**，缺字段或值为 `null` 即 400：

```
HTTP/400 {"error":{"message":"The `reasoning_content` in the thinking mode must be
passed back to the API. (request_id: …)","type":"invalid_request_error","code":"invalid_request_error"}}
```

而 DeepSeek 自身约 1/3 的回合不产生 reasoning 文本（流式首帧 `""`、末帧 `null`）。契约不自洽：**客户端忠实镜像上游响应就已经违规**，由此产生两个表象不同、根因相同的问题：

| | 问题一：路由切到 DeepSeek 时爆发 | 问题二：长会话内偶发 |
|---|---|---|
| 现象 | 切流后**首请求**集中 400（同一秒级窗口内多个 api-key 同时出现） | DeepSeek 时段内间歇性 400，与切流无关 |
| 直接原因 | 当前轮 assistant 由别的后端生成，不带该字段 | DeepSeek 自己不发字段，客户端丢弃空字段后下一轮必挂 |
| 触发前提 | 上一轮由其他 provider 生成 | 上一轮 DeepSeek 没有 reasoning |
| 复发特征 | 只在路由翻转点 | 逐日复发 |

实测（同一时段 1,493 条 DeepSeek 请求、10 条 400）：失败集中在三类客户端（自研 aha bridge、aiohttp、claude-cli）；其余客户端因自动补 `""` 而零失败。失败率 ≈ 客户端工具回合占比 × 无 reasoning 回合比例，随 agent 型流量增长而恶化。

**为什么由网关兜底**：判断标准是"客户端忠实镜像上游响应是否就已违规"。两个问题的答案都是"是"——上游自己不发字段；切流时客户端无从得知下一个后端要校验什么。准入红线：只兜底"上游契约不自洽 + 客户端无法单方面合规"的问题。

**为什么落在 AI 转发阶段而不是 body 处理管道**（本次方案的关键选择）：

1. 故障属性是 **provider（上游实体）**，不是模型名：同一 provider 下所有思考模式模型共用这条校验，而客户端经 ModelMapping 传入的模型名多样（含别名），用模型清单近似 provider 必然漏配；
2. `mod_body_process` 挂在 `HandleAfterLocation`，此时**目标 cluster 尚未确定**——cluster 是在 AI 转发循环里逐个 attempt 选出的（key 轮换 + route 级 fallback），`aiMeta.Provider` 直到 `reverseproxy.go:2102` 才由 `cluster.AIConf.Provider` 赋值；条件引擎也没有 cluster 原语（`req_cluster_in`/`bfe_cluster_in` 未映射），只能退化成"按模型名猜 provider"；
3. 转发阶段已有同构先例：`AIConf.NormalizeUpstreamError`（上游错误归一）、`Server.InjectStreamUsage`（缺字段注入）、`computeTargetModel` + `condition.ReqBodyJsonSet`（请求体 model 重写）都在 `doSingleAIForward` 内完成，body 改写契约、batch/file 与非 JSON 豁免、per-attempt 缓存均有现成写法。

**目标**

1. 校验窗口内缺 `reasoning_content` 的 assistant 消息由网关补 `""`，使 DeepSeek 思考模式模型上的该类 400 归零；
2. 只对显式开启开关的 cluster 生效，未配置的流量字节级不变；
3. 任何解析/改写异常 fail-open，不新增请求失败；
4. 配置驱动、按 cluster 秒级回滚，自带指标与 Sunset 退出条件。

**验收标准**

1. 单测：`bfe_server` 新增用例全绿（第 6.1 节 14 项 + `AIConfCheck` 分支）；
2. 集成测试：SC45 的 P0 用例（TC-01…TC-09）全绿（第 6.2 节）；
3. 灰度判据：目标错误签名归零、抓包确认字段落地、`ReasoningStill400` 恒为 0（第 7 节）。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 集群配置 | `AIConf.EnsureReasoningContent []string` + `AIConfCheck` 校验 | `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` |
| 归一化实现（新文件） | `ensureReasoningContent`：开关判定、门禁、窗口定位、`""` 定点注入、anthropic 空 thinking 块、fail-open | `bfe_server/reverseproxy_ai_reasoning.go`（新增） |
| 转发主链路 | `doSingleAIForward` 内调用 + 契约反向变更计数 | `bfe_server/reverseproxy.go` |
| 请求级标记 | `AiBasicInfo.ReasoningPatched` | `bfe_basic/request_ai_basic.go` |
| 指标 | 4 个 counter（`ReasoningPatched` / `ReasoningSkip` / `ReasoningFail` / `ReasoningStill400`） | `bfe_server/proxy_state.go` |
| 文档/样例 | 字段文档与配置样例同步 | `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md`、`conf/` |
| 集成测试 | 新增 SC45 场景（设计文档 + 实现 + 两处索引） | `tests/integration/测试设计文档/scenario-SC45-推理内容归一化/`、`tests/integration/implementation/scenario-SC45-reasoning-content-normalization/`、`tests/integration/README.md`、`tests/integration/测试设计文档/测试场景总体说明.md` |

## 3. 现状时序与问题定位

`doSingleAIForward`（`bfe_server/reverseproxy.go`）单个 attempt 的顺序：

```text
doSingleAIForward(cluster, aiMeta, attempt, ...)
  ├─ 协议路径重写   applyAIProtocolPathRewrite()               :2009
  ├─ 目标模型计算   targetModel := computeTargetModel()        :2032
  ├─ HandleAfterAITargetModel 回调（mod_ai_context 等，若启用） :2043
  ├─ model 重写     shouldRewriteBodyModel + ReqBodyJsonSet    :2064
  ├─ 流式 usage 注入 maybeInjectStreamUsage()                   :2088
  ├─ AIConf 应用（Provider / Key / 凭据注入）                   :2099-2130
  └─ adapter.InjectAuth() → 转发
```

插入点：**`:2088` 之后、`AIConf` 应用块附近**，即回调与 model 重写都已完成之后。顺序是硬约束，两个原因：

1. `mod_ai_context`（若启用）在 `HandleAfterAITargetModel` 回调内整体 re-marshal 请求体，其 `Message.MarshalJSON`（`bfe_modules/mod_ai_context/messages_parse.go:101`）会丢弃值为空的 `reasoning_content`——本改动若跑在它前面会被抹掉；
2. model 重写（`ReqBodyJsonSet`，`:2064`）会改写请求体并重置 `Content-Length`，本改动必须在其后，否则同样被覆盖。

## 4. 详细改动

### 4.1 集群配置（`AIConf`）

```go
// bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go
// AIConf 新增（json 键与 Go 字段同名 PascalCase，与 ModelProtocols 一致）：
//
// EnsureReasoningContent lists the model access protocols for which the
// gateway fills an empty reasoning_content on assistant messages inside the
// upstream validation window. Empty/nil means disabled.
EnsureReasoningContent []string
```

`AIConfCheck`（`:1290`）新增校验：

| 规则 | 说明 |
|---|---|
| 协议名合法且在实现内 | 仅允许 `openai`、`anthropic`——即本期真正实现了注入分支的两种协议。`gemini` **不能等同于"暂不支持"**：Gemini 请求体是 `contents[].parts[]` 结构，没有 `messages` 数组也没有 `reasoning_content` 字段，注入无落点；其思考内容回传是另一套机制（`thought`/`thoughtSignature`），空串解决不了。因此 `AIConfCheck` 直接拒绝该值，避免"配了但什么都不做"的假开关；将来若出现同类契约，先实现分支再放开取值 |
| ⊆ 本 cluster 协议 | 每项须满足 `modelprotocol.Supports(conf.ModelProtocols, p)`；`ModelProtocols` 为空按 `["openai"]` 处理（与 `Supports` 的既有语义一致），未申报的协议报错 |
| 空/nil | 合法，表示关闭；零值即兼容旧 `cluster_table.data` / `cluster_conf.data` |

取值样例：`["openai"]`、`["anthropic"]`、`["openai","anthropic"]`。数组而非 bool/枚举，是为了新增协议时只加元素、不扩组合值。

### 4.2 归一化实现（`bfe_server/reverseproxy_ai_reasoning.go`，新增）

```go
func ensureReasoningContent(basicReq *bfe_basic.Request, outreq *bfe_http.Request,
    cfg *cluster_conf.AIConf, aiMeta *bfe_basic.AiBasicInfo)
```

门禁顺序（任一不满足即原样返回、计数后结束）：

| # | 门禁 | 依据 |
|---|---|---|
| 1 | `cfg == nil`、开关数组为空，或数组不含本次请求协议 | 开关与协议维度（数组包含判断，不走 `modelprotocol.Supports`——后者对空列表默认 openai，语义不同） |
| 2 | `aiMeta.Mode != ModeChat` | 仅 chat 处理；`ModeFile`/`ModeBatch`（`isBatchPassthroughMode` 的等价判定）计 `ReasoningSkip`，`ModeResponses` 等其他模式不计数（设计上不在范围内） |
| 3 | `isNonJsonContentType(outreq.Header)` | 复用 `:2546` 的既有豁免语义，计 `ReasoningSkip` |
| 4 | `hasRequestBodyEncoding(outreq.Header)`（`Content-Encoding` 非 identity） | 压缩字节上跑 gjson/sjson 会破坏请求体，计 `ReasoningSkip`；语义照 `bfe_modules/mod_body_process/body_process.go:615` |
| 5 | `outreq.GetBodyAccessor()` 为 nil 或失败 | 无可改写对象，计 `ReasoningSkip` |
| 6 | `body, all := bodyAccessor.GetBytes(); !all` | body 超过 `AccessibleBodySize`（默认 2MB、上限 8MB，`bfe_http/request.go:1010-1023`）时只缓冲了前 N 字节，**必须跳过**，计 `ReasoningSkip`（注：现有 `condition.ReqBodyJsonSet` 未做此检查，本改动不沿用该疏漏） |

改写算法（`openai` 分支）：

```text
1. gjson: messages = GetBytes(body, "messages")；不存在或非数组 → 原样返回
2. 定位窗口起点：从末尾向前找第一个 role == "user" 的消息下标 lastUserIdx
   （找不到、或其后没有 assistant → 无需改写，原样返回）
3. 遍历 (lastUserIdx, len(messages)) 之间的 assistant 消息：
     raw := messages.<i>.reasoning_content
     if !raw.Exists() || raw.Type == gjson.Null:
         body, _ = sjson.SetBytes(body, "messages.<i>.reasoning_content", "")
         命中数++
4. 命中数 > 0 → bodyAccessor.SetBytes(body, false)；
   同步 ContentLength = -1 + Del("Content-Length")（outreq 与 basicReq.HttpRequest 两处，
   与 model 重写段 `:2064-2081` 的处理完全相同，保证 fallback/重试时长度一致）；
   aiMeta.ReasoningPatched = true
```

`anthropic` 分支（`/v1/messages`）：对窗口内的 assistant，若 `content` 数组中不存在 `type == "thinking"` 的元素，在数组头部插入 `{"type":"thinking","thinking":"","signature":""}`。**默认不启用**——空 signature 能否被上游接受尚未验证，实测通过后再把 `anthropic` 加入配置数组。

fail-open（强约束）：上述任一步骤出现解析失败、结构异常（`messages` 元素非对象、`content` 既非 string 也非数组）、类型不匹配，一律**不改 body 直接返回**，只记日志与 counter。实现上有一处必须前置的校验：**先 `gjson.ValidBytes(body)` 判断整体是否合法 JSON**——gjson 对截断的 body 仍能部分解析，sjson 会继续往里追加字段，把畸形请求改得更长（实测：`{"messages":[…,{"role":"assistant"}` 会被补成仍缺右括号的更长 body）。截断/非法 body 一律走 `ReasoningFail` 透传。本实现不替换 decoder、不整体 re-marshal，故不存在"解码失败致 5xx"这类风险；采用 sjson 定点注入也避免了 re-marshal 的字节漂移（字段顺序、数字格式、Unicode 转义均保持原样）。

### 4.3 调用点

```go
// bfe_server/reverseproxy.go, doSingleAIForward 内：model 重写与流式 usage 注入
// 之后（:2088 之后）、AIConf 应用块之前
reasoningPatched := p.ensureReasoningContent(basicReq, outreq, cluster.AIConf, aiMeta)

// ... clusterInvoke 之后，「补齐了却仍 400」的契约反向变更计数（在 fallback 决策之前）
if reasoningPatched && res != nil && res.StatusCode == 400 {
    incStateCounter(p.proxyState.ReasoningStill400)
}
```

`ensureReasoningContent` 返回本次 attempt 是否真的改写了 body——用返回值而不是 `AiBasicInfo` 上的标记来计数 `ReasoningStill400`，保证 fallback 到下一个 attempt 时不会把"上一个 attempt 补过"误算成本次补丁失效。

### 4.4 可观测性

`bfe_server/proxy_state.go` 新增 counter（照 `ErrNormalizeHit` 等既有 AI 计数器的定义与 `incStateCounter` 用法）：

| 指标 | 含义 | 用途 |
|---|---|---|
| `ReasoningPatched` | 本 attempt 至少补了一条消息 | 效果验证；按 cluster 维度观察即可判断某个 cluster 是否真的需要该开关 |
| `ReasoningSkip` | 门禁跳过（压缩体 / 超限 / 非 JSON / batch） | 评估覆盖缺口；超限计数持续增长说明需调 `AccessibleBodySize` 或接受该盲区 |
| `ReasoningFail` | 解析失败或结构异常导致 fail-open | 健康度；突增说明上游请求结构变化或门禁条件过宽 |
| `ReasoningStill400` | **本 attempt 已补字段但仍返回 400** | 契约反向变更的唯一探测信号（见第 8 节风险 1） |

`ReasoningStill400` 的计数点在 `doSingleAIForward` 的返回路径：`aiMeta.ReasoningPatched && res != nil && res.StatusCode == 400` 时计数，**发生在 fallback 决策之前**——这样即使后续 attempt 成功兜底，也能捕捉到上游开始拒绝 `""` 的信号。

日志：命中打 debug（含 cluster、key 名、api-key）；fail-open 打 warn（含错误原因与 body 前 512 字节）。`AiBasicInfo.ReasoningPatched` 置位供日志与指标读取。

### 4.5 配置样例与文档

`cluster_conf.data`（或控制面下发的等价 JSON）：

```json
{
    "cluster_deepseek": {
        "AIConf": {
            "Provider": "deepseek",
            "ModelProtocols": ["openai", "anthropic"],
            "EnsureReasoningContent": ["openai"]
        }
    }
}
```

`conf/` 样例与 `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md` 同步新增该字段说明（AGENTS.md 要求：配置变更必须反映到 `conf/`）。

### 4.6 控制面影响

控制面（ai-gateway-api）在 cluster 的 `llm_config` 中暴露该字段并导出到 `cluster_conf.data`；BFE 侧只依赖结构本身。控制面写入时的协议值域校验与 `AIConfCheck` 同口径（合法协议名 + ⊆ provider 的 `model_protocols`），两层防线，避免"控制面放行、数据面加载失败"。

### 4.7 配置影响汇总

| 项 | 内容 |
|---|---|
| 新增字段 | `AIConf.EnsureReasoningContent []string` |
| 类型与取值 | 协议名数组；仅允许 `openai`、`anthropic`（本期已实现注入分支的协议）。`gemini` 不适用而非"待支持"：其请求体为 `contents/parts` 结构、无 `reasoning_content` 落点，配置即报错 |
| 默认值与必填 | 空（nil 或 `[]`）= 关闭；非必填 |
| 生效范围 | 仅配置了该字段的 cluster；按 attempt 生效——fallback 到未配置的 cluster 时该 attempt 不生效 |
| 校验 | `AIConfCheck`：协议名合法 + ⊆ 本 cluster 的 `ModelProtocols`（为空按 `["openai"]`）；不合规时首启失败、reload 失败并保留旧配置 |
| 热加载 | 随 `cluster_conf.data` 下发，`/reload` 后生效，无状态残留（纯请求期判定） |
| 旧配置兼容 | 缺失即关闭；旧 `cluster_table.data` / `cluster_conf.data` 无需变更；未配置 cluster 的行为与旧版本逐字节一致 |
| 文档与样例（Phase 2 落地） | `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md`、`docs/en_us/configuration/server_data_conf/cluster_conf.data.md`、`conf/` |
| 控制面 | ai-gateway-api 的 cluster `llm_config` 暴露该字段并导入导出到 `cluster_conf.data`（见 4.6） |

## 5. 行为变化与兼容性

| 场景 | 现状 | 本次改动后 | 兼容性 |
|---|---|---|---|
| 未配置该字段的 cluster（绝大多数） | 原样透传 | 门禁 1 直接返回，**零行为变化** | 旧 `cluster_table.data` / `cluster_conf.data` 无需变更 |
| 配置 `["openai"]` 的 cluster | — | 校验窗口内缺 `reasoning_content` 的 assistant 被补 `""` | 已有非空值一律不动；只增加字段，其余字节不变 |
| 客户端已自带字段（约 90% 流量） | 200 | 200，请求体不变 | 零影响 |
| kimi / bailian 等其他 provider | 200 | 不配置即不生效 | 零影响 |
| `/responses`（Codex） | 200 | **不处理**（结构不同：reasoning 是 `input` 数组独立 item，无注入落点） | 零影响 |
| `/v1/messages`（Anthropic） | 200 | 默认不启用（`anthropic` 未加入配置数组）；实测通过后可按 cluster 开启 | 零影响 |
| 压缩请求体（`Content-Encoding` 非 identity） | 透传 | 跳过改写 + `ReasoningSkip` | 不劣化 |
| body 超 `AccessibleBodySize` | 透传 | 跳过改写 + `ReasoningSkip` | 不劣化（见第 8 节风险 4） |
| batch / file 模式、非 JSON `Content-Type` | 透传 | 跳过改写 | 不劣化 |

## 6. 测试

### 6.1 单元测试

新增 `bfe_server/reverseproxy_ai_reasoning_test.go`（照 `reverseproxy_client_close_test.go` 的 `ProxyState` 构造方式）：

1. 字段为空（nil/`[]`）→ 字节级原样放行；
2. `Supports` 不含本次协议 → 原样放行；
3. 窗口内（最后一条 user 之后）的 assistant 被补；历史缺字段不动；
4. 已有非空值不动；值为 `null` 归一为 `""`；
5. 无 `messages` 的非 chat body 原样放行（字节级一致）；
6. JSON 解析失败原样放行（fail-open + `ReasoningFail`）；
7. `Content-Encoding: gzip` 跳过 + `ReasoningSkip`，字节不变；
8. `GetBytes()` 返回 `all=false` 跳过 + `ReasoningSkip`；
9. batch/file 模式、非 JSON `Content-Type` → 跳过；
10. assistant 带 `tool_calls` 且 `content` 为 `null` 时结构正确、字段插入位置正确；
11. anthropic 分支：content 数组头部插入空 thinking 块；已有 thinking 不动；未列入数组时不注入；
12. 幂等：连续两次调用，第二次不改写、字节一致（fallback 重复进入同一 body）；
13. 定点改写：断言除目标字段外其余字节完全不变（用 MB 级真实 body 回放，防 sjson 误伤）；
14. `ReasoningStill400`：由集成用例 SC45 TC-08 覆盖（该计数点在 `doSingleAIForward` 的返回路径，需真实上游响应，单测不便构造）；单测只覆盖"返回值 true/false"这一判定输入。
15. **截断 body 不被追加字段**：`gjson.ValidBytes` 前置校验拦住，返回错误 → `ReasoningFail` +1、body 逐字节不变（实现期发现：gjson 能部分解析截断 body，sjson 会继续追加，必须显式校验）。

另加基准 `BenchmarkPatchReasoningContentLargeBody`（500KB body）守护性能回归，实测数据见第 8 节风险 3。

配置侧：`cluster_conf_load_test.go` 补 `AIConfCheck` 分支（`gemini`/未知协议拒绝、未在 `ModelProtocols` 申报拒绝、重复项拒绝、空数组合法）。

### 6.2 集成测试（`bfe/tests/integration`，**新增 SC45**）

单测覆盖不到的部分必须由真实进程级用例兜住：请求体确实按字节改写到线上、cluster/协议维度门禁真的生效、多 attempt（key 轮换 + fallback）下不重复改写、指标真的被点燃。按 `tests/integration/README.md` 的定位，本场景**只启动真实 `bfe` 进程 + `httptest` mock 上游**，不引入 ai-gateway-api / conf-agent；控制面「API 写入 → 导出 `cluster_conf.data`」链路属仓库根 `integration-test/`，不在本场景内。

**目录与命名**（沿用现有约定；SC44 已占用，本场景取 **SC45**）：

| 类型 | 路径 |
|---|---|
| 测试设计文档 | `tests/integration/测试设计文档/scenario-SC45-推理内容归一化/`（`场景说明.md` + `TC-01..TC-14` 单文件；每个 TC 按仓内规范含：目的 / 前置条件 / 请求构造 / 后端响应 / 执行步骤 / 预期结果 / 清理） |
| 实现 | `tests/integration/implementation/scenario-SC45-reasoning-content-normalization/`（`sc45_reasoning_content_normalization_test.go` + `testdata/`） |

> 状态：设计文档与实现均已完成，14 个用例全绿（`go test ./tests/integration/implementation/scenario-SC45-reasoning-content-normalization/`，约 28s；TC-14 的两个 fail-fast 臂各需约 10s 等待启动超时）。

**harness 复用**：`common/process_env.go`（编译/启动/停止真实 bfe）、`common/bfe_config_builder.go`（生成临时配置）、`common/mock_backend.go`（mock 上游）、`common/monitor.go` 的 `common.ModuleMetric` / `common.WaitForModuleMetricValue`（读 `proxy_state` 计数）。断言方式为 mock 上游**记录收到的原始请求字节**（`io.ReadAll`），与客户端发送字节做定长 diff——本改动的正确性判据就是"只多一个字段、其余字节不变"，用结构化解析断言会漏掉误伤。

**用例清单**（P0 为上线门槛）：

| TC | 内容 | 优先级 |
|---|---|---|
| TC-01 | 默认关闭基线：cluster 未配该字段 → 请求体逐字节透传到上游，`ReasoningPatched=0` | P0 |
| TC-02 | 窗口内补齐：最后一条 user 之后的 assistant 缺字段 → 上游收到 `"reasoning_content":""`；窗口外历史消息不动；其余字节不变 | P0 |
| TC-03 | 已有非空值不动；值为 `null` 归一为 `""`（双臂） | P0 |
| TC-04 | 协议维度门禁：配 `["openai"]` 时 `/v1/messages`（anthropic）不被注入；配 `["openai","anthropic"]` 后进入 TC-05 | P0 |
| TC-05 | anthropic 分支：assistant `content` 数组头部插入 `{"type":"thinking","thinking":"","signature":""}`；已有 thinking 不重复插入 | P0 |
| TC-06 | 门禁：`Content-Encoding: gzip` 请求体不被改写（上游收到压缩字节与客户端一致）且 `ReasoningSkip` +1；非 JSON `Content-Type` 同理 | P0 |
| TC-07 | 多 attempt 幂等：主 cluster 已补 `""`，fallback 到另一已开启 cluster 的 attempt 不二次改写（上游字节中无重复键/损坏）；fallback 到未开启 cluster 时其请求体保持合法 | P0 |
| TC-08 | 契约反向变更信号：mock 上游对已补字段的请求返回 400 → 该 attempt `ReasoningStill400` +1；若 fallback 成功，客户端仍拿到 200 | P0 |
| TC-09 | `AccessibleBodySize` 边界：用较小的 `Server.AccessibleBodySize`（如 4KB）配大 body 触发 `all=false` → 不改写 + `ReasoningSkip`；同配置下小 body 正常补齐（两臂对照） | P0 |
| TC-10 | 与 `mod_ai_context` 共存：开启压缩并触发整体 re-marshal 时，补丁仍生效（验证"必须在回调之后"的顺序约束）；关闭压缩时为基线 | P1 |
| TC-11 | `/responses`（Codex 形态）请求不处理，逐字节透传 | P1 |
| TC-12 | batch/file 模式（`mod_ai_batch` 流式透传）不被缓冲、不被改写 | P1 |
| TC-13 | 指标面：`ReasoningPatched` / `ReasoningSkip` / `ReasoningFail` 在对应臂上的计数正确（`common.ModuleMetric(e.monitorPort, "proxy_state", ...)`，口径同 SC36 的 `SseReqServed` 断言） | P1 |
| TC-14 | 配置 fail-fast：`EnsureReasoningContent=["openai","anthropic"]` 而 `ModelProtocols=["openai"]` → 首启失败；reload 场景下加载失败且旧配置保留 | P1 |

**配套需要同步更新的索引**（漏改会导致场景清单与目录不一致）：

1. `tests/integration/README.md` 的「当前覆盖」表格新增 SC45 一行（一句话描述 + 覆盖要点）；
2. `tests/integration/测试设计文档/测试场景总体说明.md`：版本信息表的「对应 BFE 行为方案」追加本文件路径；「3. 场景清单」表格新增 SC45 行（场景编号 / 名称 / 一句话描述 / 优先级 P0 / 计划测试例数 14 / 对应设计文档指向本文件）。

运行方式（于 `bfe/` 下）：

```bash
go test ./tests/integration/implementation/scenario-SC45-reasoning-content-normalization/... -v
```

### 6.3 验证命令

`make test`（`go test -cover ./...` + `go vet ./...`）、`make check`；新文件带 Apache 2.0 license 头。集成用例首跑会自动编译并缓存 bfe 二进制（`tests/integration/.integration-test-bin/`）。

## 7. 灰度与回滚

- **灰度单位是 cluster**：开关挂在 `AIConf` 上，先给单个 DeepSeek cluster 配 `["openai"]`，验证指标达标后逐个开启其余 DeepSeek cluster；kimi/bailian 保持不配。
- **回滚（秒级）**：清空该 cluster 的 `EnsureReasoningContent` 即恢复现状，无需回退二进制；灰度期间只影响单个 cluster。
- **上线顺序**：先全量部署含本改动的二进制（字段全空，行为不变），再逐个 cluster 开配置。
- **集成门槛**：开第一个 cluster 之前，SC45 的 P0 用例（TC-01…TC-09）必须全绿——它们恰好覆盖"默认关闭不改变字节、窗口内才补、门禁生效、多 attempt 不重复改写、契约反向信号可观测"这五条上线前提。
- **验收判据**：目标错误签名（`cluster:"deepseek" && res_status_code:400 && res_body_len:185~189`）持续归零；抓包确认下一轮请求窗口内的 assistant 带 `"reasoning_content":""`；`ReasoningPatched` 平稳、`ReasoningFail`/`ReasoningSkip` 无异常突增、**`ReasoningStill400` 恒为 0**。
- **Sunset**：上游修复后下线该配置。注意上游修复有两种形态——(A) 每回合都产出 reasoning（补丁命中数自然归零）；(B) 校验改为容忍缺失（补丁继续命中但已无必要，命中数不会归零）。因此退出门槛取"目标 400 签名连续 7 天归零 **且** 上游确认已修 A 或 B 任一形态"，不以"补丁不命中"为准；确认后清空配置 → 删除实现与字段。

## 8. 风险与应对

| # | 风险 | 等级 | 应对 |
|---|---|---|---|
| 1 | 上游契约反向变更（改为要求非空或校验签名），注入 `""` 反而制造新 400 | 中 | `ReasoningStill400` 专测该路径，非零立即清空配置回滚；这是本方案唯一"补丁自身造成故障"的路径，优先级最高 |
| 2 | 定点改写误伤相邻字节（sjson 路径计算错误） | 中 | 单测 13 断言"除目标字段外字节完全不变" + 真实 body 回放 |
| 3 | MB 级 body 性能 | 中 | 开关打开的 cluster 上每个请求都有一次 body 缓冲（≤ `AccessibleBodySize`）+ 一次 JSON 校验 + 一次窗口扫描（未命中也有成本），命中时再加一次定点写入（无整体 re-marshal）。**实测**（`BenchmarkPatchReasoningContentLargeBody`，500KB body，单核笔记本 CPU）：合计 ~2.5 ms，其中 `gjson.ValidBytes` ~1.0 ms、`sjson.SetBytes` ~0.7 ms、窗口扫描 ~0.8 ms；换算 MB 级约 5 ms，**贴着验收门槛**。生产 CPU 通常更快，且典型 chat 体量远小于 1MB；若压测发现超标，优先优化 JSON 校验（例如仅在 body 超过阈值时校验，或改为"写入后校验"） |
| 4 | body 超 `AccessibleBodySize`（默认 2MB）的请求不被覆盖 | 低 | 明确记录为已知盲区，`ReasoningSkip` 可观测；若线上确有此类请求命中该 400，再评估调大 `AccessibleBodySize`（内存代价，上限 8MB） |
| 5 | 与 `mod_ai_context` 的改写顺序 | 低 | 本改动定位在 `HandleAfterAITargetModel` 回调之后；若将来调整该顺序，必须重跑单测 12（幂等） |
| 6 | 新增 DeepSeek cluster 漏配开关 | 中 | 表现为该 cluster 仍 400；验收指标按 cluster 维度聚合即可定位；控制面新 cluster 上线清单包含该字段 |
| 7 | 控制面与数据面校验口径不一致 | 低 | `AIConfCheck` 复用 `ValidateProtocols` + `Supports` 的空默认语义；导出 JSON 用 BFE 结构反序列化做契约测试 |
| 8 | 掩盖客户端缺陷：客户端丢弃了非空 `reasoning_content` 时无法与"本轮无 reasoning"区分，一律补 `""` | 中 | 请求从硬失败变为可用但静默降级（丢失该轮推理上下文，cache 前缀可能变化）。以灰度期的 cache 命中率、输出 token 分布对比兜底，异常则回滚该 cluster 开关 |
