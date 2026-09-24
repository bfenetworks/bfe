# AI 请求 mode 识别兼容不带 /v1 的客户端入口（DetectModeFromPath）

## 1. 背景

[2026-09-21-openai-path-rewrite-no-v1-entry](../2026-09-21-openai-path-rewrite-no-v1-entry/design-changes.md)（issue #1379）修复了 openai 路径改写对无 `/v1` 客户端入口的兼容，其 §5.3 遗留本文档处理的关联问题：**`DetectModeFromPath` 同样只认 `/v1/...` 前缀，不带 `/v1` 的非 chat 端点请求会被识别为 `ModeChat`**，导致计费 mode 错误。

影响链路（均已在当前 HEAD 核实）：

```
http_conn.go:557  aiMeta.Mode = DetectModeFromPath(path)          ← 唯一生产点
       ↓
mod_ai_token_auth.go:497-502  ImageCount/VideoCount 提取（mode 门控）
       ↓
mod_ai_token_auth.go:549 + cluster_conf_load.go:1563 LookupModelPrice(model, mode)  ← 按 (模型, mode) 查价
```

两类错误后果：

1. **价差方向错误**：`/embeddings`、`/audio/speech` 等裸路径请求按 chat token 价计费（embedding 单价通常远低于 chat，方向是**多收**）；
2. **计费维度丢失**：`/images/generations`、`/video/generations` 裸路径请求不会提取 `ImageCount`/`VideoCount`（按次计费变按 token 估算计费）。

时间线说明：该缺口先于 #1379 存在（未配置 provider path 的透传 cluster 上，裸路径请求本就能成功并被误计费）；#1379 使**配置了 provider path 的 cluster** 上的此类请求从上游 404 变为成功，误计费可达面随之扩大，不宜长期搁置。

受影响面界定：chat 主场景（含 Trae 的 `/chat/completions`）带不带 `/v1` 都正确落 `ModeChat`，**不受影响**；仅"不带 `/v1` 的非 chat 端点请求"受影响，而主流 SDK 的 base_url 自带 `/v1`，实际触发面窄。

---

## 2. 代码核实结果

| 引用点 | 实际位置 | 核实结论 |
|---|---|---|
| mode 生产点 | `bfe_server/http_conn.go:557`（`EnableAiGateway` 分支内，每个 AI 请求执行） | ✅ 唯一生产点 |
| mode 消费点 1 | `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:497-502` | ✅ `ModeImageGeneration`/`ModeVideoGeneration` 门控 `ImageCount`/`VideoCount` 提取 |
| mode 消费点 2 | `mod_ai_token_auth.go:549` → `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:1563` `LookupModelPrice(table, model, mode)` | ✅ mode 为空时回退 `ModeChat`（:550-552），按 (cluster, model, mode) 查价，查不到则告警并返回 0（不扣费） |
| 识别缺口 | `bfe_basic/request_ai_basic.go:182-207` `DetectModeFromPath`：全部 10 个分支均以 `/v1/...` 前缀匹配，default → `ModeChat` | ✅ `/embeddings`（无 `/v1`）落 `ModeChat` |
| 观测手段 | 文本访问日志 `bfe_modules/mod_access` 无 mode 字段；**PB 访问日志 `mod_access_pb3` 记录 mode**（`request_log.go:399-401` `reqLog.AiMode = proto.String(aiInfo.Mode)`） | ✅ 集成测试可解析 `pb_access3.log` 断言 `AiMode`：`tests/integration/common/access_log_parser.go` 的 `ParseAccessLog` 已具备解析能力，SC05（access-log-ai-fields）已用同款 harness 断言 AI 字段 |
| 端点知识重复 | 改写公式私有端点表 `openAIEndpoints`（`bfe_server/ai_path_rewrite.go`）与 `DetectModeFromPath` 的端点集合各自为政 | ⚠️ 两处口径漂移风险（如本次：改写认 `/embeddings`、mode 识别不认） |

mode 常量（`bfe_basic/request_ai_basic.go:37-49`）：`chat`、`completion`、`image_generation`、`image_edit`、`embedding`、`audio_speech`、`audio_transcription`、`rerank`、`video_generation`、`responses`。

控制面边界：ai-gateway-api 仅在 model-prices 接口将 mode 定义为价格配置枚举（`design-docs/api-define/OpenAPI接口定义/model-prices.md`），**不描述 path→mode 推断规则**（纯数据面行为）——本次无需跨仓库文档同步。

---

## 3. 根因分析

端点 → mode 的知识硬编码在 `DetectModeFromPath` 的分支条件里，并内置了"客户端必然带 `/v1`"的假设；而 #1379 引入的改写端点表（`openAIEndpoints`）是另一份独立维护的端点知识。同一份"哪些是 OpenAI 标准端点"的事实存在两处定义，且一处兼容裸入口、一处不兼容，缺口由此产生。

---

## 4. 修复方案

### 4.1 端点表上提 `bfe_basic` 共享（核心）

新建 `bfe_basic/openai_endpoint.go`：

```go
// openAIEndpointModes 是 OpenAI 标准端点（已剥离可选 /v1 前缀的形态）到
// 计费 mode 的映射。key 按前缀规则匹配：path == key 或
// strings.HasPrefix(path, key+"/")（覆盖 /models/{model} 形态）。
// mode 取 ModeChat 的端点（/models、/moderations、/audio/translations）
// 无专属价格档，保持默认 chat 语义不变。
var openAIEndpointModes = map[string]string{
    "/audio/speech":         ModeAudioSpeech,
    "/audio/transcriptions": ModeAudioTranscription,
    "/audio/translations":   ModeChat, // 无专属 mode，同现状默认
    "/chat/completions":     ModeChat,
    "/completions":          ModeCompletion,
    "/embeddings":           ModeEmbedding,
    "/images/edits":         ModeImageEdit,
    "/images/generations":   ModeImageGeneration,
    "/models":               ModeChat, // 无专属 mode，同现状默认
    "/moderations":          ModeChat, // 无专属 mode，同现状默认
    "/responses":            ModeResponses,
    "/rerank":               ModeRerank,
    "/video/generations":    ModeVideoGeneration,
}

// IsOpenAIEndpoint 报告 path（须已剥离可选 /v1 前缀）是否命中 OpenAI 标准
// 端点集合；bfe_server 的路径改写公式与 DetectModeFromPath 共用本定义。
func IsOpenAIEndpoint(path string) bool { ... }

// lookupOpenAIEndpointMode 返回端点对应的 mode；未命中返回 ("", false)。
func lookupOpenAIEndpointMode(path string) (string, bool) { ... }
```

`DetectModeFromPath`（`request_ai_basic.go:182`）重写为：

```go
func DetectModeFromPath(path string) string {
    if mode, ok := lookupOpenAIEndpointMode(stripV1Prefix(path)); ok {
        return mode
    }
    return ModeChat
}
```

（`stripV1Prefix` 同步上提或内联：仅剥 `/v1/` 前缀，与改写公式同规则。）

`bfe_server/ai_path_rewrite.go`：删除私有 `openAIEndpoints` 表与 `isOpenAIEndpoint`，改写公式改调 `bfe_basic.IsOpenAIEndpoint`——#1379 的改写行为由此与本识别的端点口径**强制一致**，从结构上消除漂移。

### 4.2 行为兼容性

| 路径 | 现状 mode | 修复后 mode | 是否变化 |
|---|---|---|---|
| `/v1/images/generations`（及全部带 `/v1` 的 10 类端点） | 正确 mode | 同一 mode（剥离 `/v1` 后命中同一端点） | 不变 |
| `/v1/models`、`/v1beta/...`、gemini 路径 | `chat` | `chat`（不命中端点 → 默认） | 不变 |
| `/v10/xxx`、`/`、空路径、精确 `/v1` | `chat` | `chat` | 不变 |
| `/images/generations`、`/embeddings` 等裸端点 | `chat`（错误） | 正确 mode | **唯一变化面** |

一处刻意收紧的边界：形如 `/v1/images/generationsfoo`（端点后直接拼接非 `/` 字符）的路径，旧实现按前缀匹配给 `image_generation`，新实现要求端点后必须为 `/` 或结尾 → `chat`。该形态不是任何真实端点，且新口径与改写公式的端点守卫一致（此类路径在改写侧同样不透传保护），属修正而非回归。

### 4.3 不动的部分

- 改写公式语义（#1379 行为）零变化，仅端点表来源从私有改为共享；
- `mod_ai_token_auth` 计费逻辑、`LookupModelPrice` 语义零改动；
- `http_conn.go:557` 调用点不变；
- anthropic（`/v1/messages`）与 gemini 请求恒为 chat 相关的既有行为不变（ DetectModeFromPath 对二者本就落默认）。

---

## 5. 代码变更汇总

| 改动点 | 文件 | 说明 |
|---|---|---|
| 端点表 + 共享查询 | `bfe_basic/openai_endpoint.go`（新） | `openAIEndpointModes`、`IsOpenAIEndpoint`、`lookupOpenAIEndpointMode`、`stripV1Prefix` |
| mode 识别重写 | `bfe_basic/request_ai_basic.go:182-207` `DetectModeFromPath` | 剥离可选 `/v1` 前缀 + 查共享表；10 个硬编码分支删除 |
| 改写公式复用共享表 | `bfe_server/ai_path_rewrite.go` | 删 `openAIEndpoints`/`isOpenAIEndpoint`，改调 `bfe_basic.IsOpenAIEndpoint` |
| 单测 | `bfe_basic/request_ai_basic_test.go`（或新增） | `DetectModeFromPath` 全矩阵（§6.1） |
| 回归 | `bfe_server/ai_path_rewrite_test.go` | 既有用例全量通过（行为不变） |
| 前序文档标注 | `docs/zh_cn/modifications/2026-09-21-openai-path-rewrite-no-v1-entry/design-changes.md` §5.3 | 补"已由本文档方案解决"指引 |

---

## 6. 测试

### 6.1 单元测试（核心）

`DetectModeFromPath` 全矩阵：

| 路径 | 预期 mode | 类别 |
|---|---|---|
| `/v1/chat/completions`、`/chat/completions` | `chat` | 带/不带 `/v1` 同结果 |
| `/v1/completions`、`/completions` | `completion` | 同上 |
| `/v1/embeddings`、`/embeddings` | `embedding` | 同上（本修复主场景） |
| `/v1/responses`、`/responses` | `responses` | 同上 |
| `/v1/rerank`、`/rerank` | `rerank` | 同上 |
| `/v1/images/generations`、`/images/generations` | `image_generation` | 同上（含按次计数场景） |
| `/v1/images/edits`、`/images/edits` | `image_edit` | 同上 |
| `/v1/audio/speech`、`/audio/speech` | `audio_speech` | 同上 |
| `/v1/audio/transcriptions`、`/audio/transcriptions` | `audio_transcription` | 同上 |
| `/v1/video/generations`、`/video/generations` | `video_generation` | 同上 |
| `/v1/models`、`/models/gpt-4` | `chat` | 无专属 mode → 默认 |
| `/v10/xxx`、`/v1beta/models/x:generateContent`、`/`、``、精确 `/v1` | `chat` | 边界不回退错误 |
| `/compatible-mode/v1/chat/completions`（provider 原生路径） | `chat` | 与改写透传保护口径一致 |

`bfe_server` 侧：#1379 全部用例（含 TC-07/08 集成场景对应的单测）原样通过。

### 6.2 集成测试

PB 访问日志记录 `AiMode`（`mod_access_pb3/request_log.go:399-401`），且 `tests/integration/common/access_log_parser.go` 的 `ParseAccessLog` 已能解析 `pb_access3.log`——**SC05（access-log-ai-fields）就是加载 `mod_access_pb3` + `mod_ai_token_auth` 并断言 AI 字段的现成 harness**，观测 mode 无需走 Redis 计费断言。方案：在 SC05 扩展 1 个 TC：

- 同一 cluster 依次发送 `/v1/embeddings`、`/embeddings`、`/images/generations`（后两个不带 `/v1`，Bearer 认证）；
- 断言三条请求对应的 PB access log 记录 `AiMode` 依次为 `embedding`、`embedding`、`image_generation`（裸 `/images/generations` 同时覆盖按次计数场景的 mode 门控前提）；
- 对照意义：修复前第二条记录为 `chat`（误计费根因），第三条为 `chat`（`ImageCount` 不提取）。

该 TC 随代码一起提交（SC05 场景说明与用例清单同步更新）；若实现期发现 SC05 harness 复用成本过高，允许降级为"单测 + 后续补 TC"，但需在提交说明中标注。

补充（可选，更深一层）：Redis 计费断言（裸 `/embeddings` 按 embedding 价扣减、对照组 `/v1/embeddings` 同价，SC03/SC11/SC12 形态）可独立交付，不在本方案验收门槛内。

### 6.3 验证

`go build ./...`、`go test ./bfe_basic/... ./bfe_server/...`、`go vet`、`gofmt -l` 全部通过。

---

## 7. 风险与回滚

- **行为变化面**仅"不带 `/v1` 的非 chat 端点请求"的 mode 识别与计费：从误计费（多收/按次丢失）修正为正确计费，可能出现个别账单下降——属修正而非回归。
- 价格表缺少对应 mode 条目时 `LookupModelPrice` 返回 nil → 告警不扣费，与现状相同，无新增失败模式；修复后部分原先误按 chat 计费的请求会变为"查不到 embedding 价则不扣费"，属控制面价格配置完备性问题（应在 model-prices 补齐对应 mode 价格）。
- 无配置格式变化、无发布顺序约束、与控制面零耦合。
- 回滚 = revert 单个提交；端点表共享后回滚需与 #1379 提交一并评估（两提交相邻，回滚顺序从后往前）。
