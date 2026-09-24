# OpenAI provider path 兼容不带 /v1 的客户端入口（issue #1379）

## 1. 背景

[issue #1379](https://github.com/bfenetworks/bfe/issues/1379)（由 rainway-ai-gateway/ai-gateway-api#190 迁移）：Trae 以 OpenAI 兼容方式调用百炼 glm-5.2，客户端实际发送 `POST /chat/completions`（**不带 `/v1` 前缀**）。BFE 的路径改写只认 `/v1` 或 `/v1/...` 入口，配置的 openai provider path（`/compatible-mode/v1`）未生效，原样向上游转发 `/chat/completions`，百炼返回 404。配置与不配置 provider path 行为完全相同，无法区分。

该特性由 [2026-09-14-ai-protocol-paths-rewrite](../2026-09-14-ai-protocol-paths-rewrite/design-changes.md) 引入，其语义定义是：`protocol_paths[openai]` = **该协议官方 SDK base_url 的 path 部分**（如 `/compatible-mode/v1`、`/api/v3`）。OpenAI SDK 会把 base_url 与 `/chat/completions` 拼接——**客户端入口带不带 `/v1`，不影响 SDK 语义下的最终上游路径**。当前实现把"客户端是否带 `/v1`"泄漏进了上游路径，与该语义定义不符。

issue 给出的预期契约（OpenAI provider path = 上游 API 基路径）：

| 客户端路径 | provider path | 上游路径 |
|---|---|---|
| `/v1/chat/completions` | `/compatible-mode/v1` | `/compatible-mode/v1/chat/completions` |
| `/chat/completions` | `/compatible-mode/v1` | `/compatible-mode/v1/chat/completions` |
| `/v1/chat/completions` | 未配置 | `/v1/chat/completions`（原样透传） |
| `/chat/completions` | 未配置 | `/chat/completions`（原样透传） |

---

## 2. 代码核实结果

issue 引用的实现与现象均已在当前 HEAD 核实，属实：

| 引用点 | 实际位置 | 核实结论 |
|---|---|---|
| 改写仅对 `/v1` 入口生效 | `bfe_server/ai_path_rewrite.go:41-48` `rewriteUpstreamPath`：`:42` `if !ok \|\| !isStandardV1Prefix(reqPath)` 直接透传；openai 分支 `:48` `base + reqPath[len("/v1"):]` | ✅ 根因：`/chat/completions` 不满足 `isStandardV1Prefix`（`:53-55` 只认 `/v1` 精确值与 `/v1/` 前缀），改写被跳过 |
| 协议识别 | `bfe_model_protocol/detect.go:56-88` `DetectProtocol`：`/v1/messages` → anthropic、gemini 特征 → gemini，**其余默认 openai**（`:87`） | ✅ Trae 的 `POST /chat/completions`（带 Authorization）被正确识别为 openai，协议识别无缺口 |
| 改写调用点 | `bfe_server/reverseproxy.go:1542` `applyAIProtocolPathRewrite(outreq, aiMeta.AuthStyle, cluster.AIConf)` | ✅ 每次 attempt 独立重算，fallback 语义已由 `ai_path_rewrite_test.go` 固化，本修复沿用该执行点，无需改动 |
| 关联缺口：mode 识别 | `bfe_basic/request_ai_basic.go:182-207` `DetectModeFromPath` 只识别 `/v1/...` 前缀；`bfe_server/http_conn.go:557` 用其结果填计费 mode | ⚠️ 不带 `/v1` 的请求一律落 `ModeChat`（见 §5.3 已知限制） |

补充事实：

- 入站请求经 `mod_ai_route` 路由到 cluster 正常（issue 抓包证据：404 来自上游 SNI `token-plan.cn-beijing.maas.aliyuncs.com`，即请求已正确到达百炼 cluster，只差路径改写），**路由与协议识别均无需改动**。
- 配置加载/校验（`AIConf.ProtocolPaths` 的 key 白名单 {openai, anthropic}、value 格式，`cluster_conf_load.go` 的 `AIConfCheck`）与本次语义修正无关，**零改动**。

---

## 3. 根因分析

改写入口判断把两种本应等价的客户端写法当成了不同物种：

```
客户端 /chat/completions
  → DetectProtocol 识别为 openai（默认分支）✓
  → 路由到百炼 cluster ✓
  → rewriteUpstreamPath: isStandardV1Prefix("/chat/completions") == false
  → 跳过改写，原样转发 /chat/completions
  → 百炼要求 /compatible-mode/v1/... 前缀 → 404
```

问题不在协议识别、路由或配置链路，而仅在 `rewriteUpstreamPath` 的入口条件：它对 openai 协议假设了"客户端必然使用 `/v1` 标准入口"。Trae 等 OpenAI 兼容客户端（含部分自建客户端）直连 base_url 时发送的正是 `/chat/completions`。

---

## 4. 修复方案

### 4.1 目标契约

`protocol_paths[openai]` 语义修正为**上游 API 基路径**：客户端路径剥离可选的 `/v1` 前缀后，若命中 OpenAI 端点集合，则上游路径 = base + 剥离后的剩余路径；否则原样透传。

- 已配置 openai provider path：
  - `/v1/chat/completions` → `/compatible-mode/v1/chat/completions`（现状保持，回归保护）
  - `/chat/completions` → `/compatible-mode/v1/chat/completions`（**本修复新增**）
  - `/v10/xxx`、provider 原生全路径（如 `/compatible-mode/v1/chat/completions`）、任意自定义路径 → **透传**（allowlist 之外永不改写）
- 未配置 openai provider path：任何路径逐字节透传（现状保持）。
- 精确 `/v1`（无剩余路径）→ base（现状即 `base + ""`，保持）。
- anthropic、gemini 分支零改动：anthropic 公式 `base + reqPath` 以 `/v1/messages` 为锚（SDK 恒拼 `/v1/messages`，检测也以 `/v1/messages` 为锚，不存在无 `/v1` 入口）；gemini 本就不进 `ProtocolPaths`（控制面白名单拒绝）。

### 4.2 核心改动：`rewriteUpstreamPath` openai 分支（`bfe_server/ai_path_rewrite.go`）

新公式（保持纯函数，便于单测）：

```go
// stripV1Prefix 剥离可选的 "/v1" 前缀："/v1/chat/completions" -> "/chat/completions"，
// "/chat/completions" 原样返回；"/v10/xxx" 不以 "/v1/" 开头，原样返回。
// 精确 "/v1" 返回 ""（调用方按 base 处理，与现状一致）。
//
// isOpenAIEndpoint 判断剥离后的路径是否命中 OpenAI API 端点集合
//（精确匹配，或以 端点+"/" 前缀匹配以覆盖 /models/{model} 形态）。
func rewriteUpstreamPath(reqPath string, authStyle string, aiConf *cluster_conf.AIConf) string {
    if aiConf == nil {
        return reqPath
    }
    base, ok := aiConf.ProtocolPaths[authStyle]
    if !ok {
        return reqPath
    }
    if authStyle == bfe_basic.AuthStyleAnthropic {
        if !isStandardV1Prefix(reqPath) {
            return reqPath
        }
        return base + reqPath // anthropic 公式不变
    }
    // openai：base = 上游 API 基路径（SDK base_url 的 path 部分），
    // 客户端入口带不带 /v1 不影响最终上游路径。
    rest := stripV1Prefix(reqPath)
    if rest == "" {
        return base // 精确 "/v1"
    }
    if !isOpenAIEndpoint(rest) {
        return reqPath // 非 OpenAI 端点：透传，保护自定义/原生路径
    }
    return base + rest
}
```

端点集合与 `DetectModeFromPath`（`request_ai_basic.go:182-207`）已支持的 mode 端点保持一致，并补齐其未覆盖的只读端点：

```
/chat/completions、/completions、/embeddings、/responses、
/models（覆盖 /models/{model}）、/images/generations、/images/edits、
/audio/speech、/audio/transcriptions、/audio/translations、
/video/generations、/rerank、/moderations
```

设计要点：

- **allowlist 而非"全部改写"**：若对任意非 `/v1` 路径都拼 base，客户端直连 provider 原生路径（`/compatible-mode/v1/chat/completions`）或网关自定义透传路径会被二次加前缀，破坏透传语义。allowlist 保证"改写只发生在 OpenAI 标准端点上"。
- **`/messages` 不进 allowlist**：anthropic 端点恒以 `/v1/messages` 为锚且检测先行（`DetectProtocol` 对 `/v1/messages` 前缀恒判 anthropic，openai 分支实际收不到），此处仅作纵深防御。
- 端点表本期作为 `bfe_server` 包内私有实现；二期与 `DetectModeFromPath` 统一时上提共享（见 §5.3）。

### 4.3 不动的部分

- 执行点 `reverseproxy.go:1542` 与"每次 attempt 从原始路径重算"的语义不变（fallback 切换 cluster 后按新 cluster 配置重算的行为由既有测试固化）。
- 配置加载/热加载/校验（`AIConfCheck` 的 key 白名单、value 格式）零改动；`conf/` 示例配置无需变更。
- `mod_ai_route` / `mod_ai_token_auth` / `mod_body_process` / `mod_ai_rate_limit` 业务语义不变。
- anthropic、gemini 改写行为不变。

---

## 5. 代码变更汇总

| 改动点 | 文件 | 说明 |
|---|---|---|
| openai 分支改写公式 | `bfe_server/ai_path_rewrite.go` `rewriteUpstreamPath` | `/v1` 前缀剥离 + OpenAI 端点 allowlist；精确 `/v1` → base 的边界保持 |
| 新增纯函数 | `bfe_server/ai_path_rewrite.go` | `stripV1Prefix` + `isOpenAIEndpoint`（端点表） |
| 语义注释更新 | `bfe_server/ai_path_rewrite.go` 文件头 | "Only standard entry paths are rewritten" 口径更新为"基路径 + 可选 /v1 剥离 + 端点 allowlist" |
| 单元测试 | `bfe_server/ai_path_rewrite_test.go` | 补 issue 四场景 + 无 `/v1` 端点 + 透传保护用例（§6.1） |
| 控制面文档同步（跨仓库） | ai-gateway-api `design-docs/api-define/OpenAPI接口定义/providers.md` | `protocol_paths` 语义描述同步为"上游 API 基路径，兼容带/不带 `/v1` 的客户端入口"；不属于本仓库提交范围 |

### 5.3 已知限制（已由后续修改解决）

`DetectModeFromPath`（`bfe_basic/request_ai_basic.go:182`）同样只认 `/v1/...` 前缀，`http_conn.go:557` 据此填计费 mode：不带 `/v1` 的请求（如 `/embeddings`）会被按 `ModeChat` 计费，而 chat 与 embedding 的单价通常不同。改写修复落地后，Trae 类 chat 客户端场景不受影响（chat 与默认 mode 一致），但**价差大的非 chat 端点若被客户端以无 `/v1` 方式调用，路径改写正确而计费 mode 错误**。

**更新（2026-09-21）**：该问题已由 [2026-09-21-ai-mode-detect-no-v1-entry](../2026-09-21-ai-mode-detect-no-v1-entry/design-changes.md) 解决——端点表已上提为 `bfe_basic` 共享定义（`openAIEndpointModes` + `IsOpenAIEndpoint`），`DetectModeFromPath` 与改写公式共用，`isOpenAIEndpoint`/`openAIEndpoints` 私有副本已删除。

---

## 6. 测试

### 6.1 单元测试（`ai_path_rewrite_test.go` 增补）

issue 四场景（核心回归）：

| 客户端路径 | provider path | 期望上游路径 |
|---|---|---|
| `/chat/completions` | `{openai: /compatible-mode/v1}` | `/compatible-mode/v1/chat/completions`（issue 主场景） |
| `/v1/chat/completions` | `{openai: /compatible-mode/v1}` | `/compatible-mode/v1/chat/completions` |
| `/chat/completions` | 未配置 | `/chat/completions`（透传） |
| `/v1/chat/completions` | 未配置 | `/v1/chat/completions`（透传） |

边界与透传保护：

- 无 `/v1` 端点：`/completions`、`/embeddings`、`/responses`、`/models/gpt-4`（前缀匹配）+ 各 base → `base + 端点`。
- 透传保护：`/compatible-mode/v1/chat/completions`（客户端已带原生前缀）、`/custom/path`、`/messages`（anthropic 锚点）、`/v10/xxx`、`/v1beta/...` 在已配置 openai base 时**均不改写**。
- 精确 `/v1`（openai，已配置）→ base；`/v1/`（openai，已配置）→ 剩余空，按精确 `/v1` 处理 → base。
- anthropic 既有用例全量回归：`/v1/messages` → `base + /v1/messages`；`/messages` 透传。
- fallback 重算既有用例（同协议、跨 cluster 混合）保持通过。

### 6.2 验证

- `go build ./...`、`go test ./bfe_server/... ./bfe_basic/...`、`go vet`、`gofmt -l` 全部通过。
- 端到端复现 issue 场景：百炼 provider 配置 `protocol_paths={openai: /compatible-mode/v1}`，curl 模拟 Trae 发 `POST /chat/completions`（`Authorization: Bearer <网关 key>`）→ 上游收到 `/compatible-mode/v1/chat/completions`，返回 200；删除 provider path 后对照组原样透传。

---

## 7. 兼容性与回滚

- **行为变化面**仅一处：已配置 openai provider path 且客户端不带 `/v1` 打 OpenAI 标准端点的请求，从"改写不生效（上游 404）"变为"按基路径改写"。该面此前是坏的（issue 场景），无正确行为依赖。
- 带 `/v1` 入口、anthropic、gemini、未配置 provider path 的场景与现状**逐字节一致**（§6.1 回归用例固化）。
- 无配置格式变化、无发布顺序约束（`ProtocolPaths` 配置链路已于 2026-09-14 上线，本次为纯行为修正）。
- 回滚 = revert 单个提交，配置与数据面无残留状态。
