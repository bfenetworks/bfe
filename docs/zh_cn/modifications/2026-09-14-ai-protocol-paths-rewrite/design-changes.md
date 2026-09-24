# AI 上游路径按协议改写（AIConf.ProtocolPaths）

## 1. 背景

BFE 对 AI 请求的上游路径**纯透传**：`doSingleAIForward` 中 `outreq` 是入站请求的拷贝（`bfe_server/reverseproxy.go:1534` 的 `*outreq = *req`），低层发送循环的 `setBackendAddr`（`:146-148`）只改写 `URL.Scheme`/`URL.Host`。因此**客户端今天必须按 provider 原生路径发起请求**——打百炼要自己带 `/compatible-mode/v1` 前缀，打火山要带 `/api/v3`，同一套客户端 SDK 配置无法复用到多家 provider。

对主流 provider 上游端点的调研表明：各家路径各不相同，且**同一 provider 的不同协议挂在不同前缀下**（最极端的百炼：OpenAI 兼容 `/compatible-mode/v1`、Anthropic 兼容 `/apps/anthropic`）：

| provider（产品线/套餐） | OpenAI 兼容（SDK base_url path） | Anthropic 兼容（SDK base_url path） |
|---|---|---|
| 百炼 DashScope | `/compatible-mode/v1` | `/apps/anthropic` |
| Kimi 开放平台（api.moonshot.cn） | `/v1` | `/anthropic` |
| Kimi Code 会员（api.kimi.com） | `/coding/v1` | `/coding` |
| DeepSeek | `/v1` | `/anthropic` |
| 火山方舟·按量 | `/api/v3` | `/api/compatible` |
| 火山方舟·Coding Plan | `/api/coding/v3` | `/api/coding` |

控制面（ai-gateway-api）本期在 Provider 上新增 `protocol_paths`（协议 → 上游 base path 的声明式映射），经导出链路透传至 BFE `AIConf`。本文档描述 **BFE 数据面侧**的修改方案；控制面侧的字段定义、校验规则、DDL 不在本文档范围（见 §8 边界说明）。

---

## 2. 目标

1. `AIConf` 新增 `ProtocolPaths` 字段（`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:223`），配置加载/热加载兼容，空值 = 关闭；加载期校验未知协议 key 与非法 value，拒绝加载（见 §4.1）；
2. `doSingleAIForward` 按（请求协议, 当前 cluster 的 `AIConf.ProtocolPaths`）改写 `outreq.URL.Path`，使客户端可用统一 `/v1/...` 标准入口访问前缀各异的上游；
3. fallback 切换 cluster 后按新 cluster 配置重算路径（每次转发 attempt 独立计算）；
4. 未配置 `ProtocolPaths` 时，转发行为与现状**逐字节一致**（纯透传）。

非目标：

- **不做协议翻译、不改写请求/响应体**——透传原则不变（与 `2026-09-03-model-protocol-adapter`、`2026-09-09-gemini-protocol-support` 一致），本特性只改上游路径；
- **gemini 协议不进 `ProtocolPaths`**——控制面白名单（{openai, anthropic}）直接拒绝该配置；gemini 原生路径（`/v1beta/models/{model}:generateContent`）本身就是标准路径，透传已可用，无需改写；BFE 加载期对未知协议 key 同样拒绝加载（§4.1）；
- **不改写 host/scheme，不支持正则或任意 rewrite 规则**——防止越出 provider 域名（计费域跳变、SSRF 风险）；通用 rewrite 方案已在控制面评审中否决（YAGNI，调研 provider 全部被单前缀模型覆盖）；
- 不改 `mod_ai_route` 路由、`mod_ai_token_auth` 计费、`mod_ai_rate_limit` 限流的业务语义；
- 不改协议识别与 `ModelProtocols` 匹配语义（`PROVIDER_PROTOCOL_MISMATCH` 行为不变）；
- BFE 不感知 path 的计费语义（火山 `/api/v3` vs `/api/coding` 错配风险由控制面文档标注，见 §7）。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 配置 | `AIConf` 新增 `ProtocolPaths map[string]string` | `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` |
| 转发层 | `doSingleAIForward` 改写 `outreq.URL.Path`（核心改造） | `bfe/bfe_server/reverseproxy.go` |
| 转发层 | 新增纯函数 `rewriteUpstreamPath`（公式化路径计算） | `bfe/bfe_server/`（新文件或 reverseproxy 内，实施定） |
| 协议适配层 | **零修改**（路径是端点知识，不是协议知识） | — |
| 路由/计费/限流模块 | 零修改 | — |
| 测试 | `rewriteUpstreamPath` 单元测试；`doSingleAIForward` 带 `ProtocolPaths` 的用例；fallback 路径重算用例 | `bfe/bfe_server/*_test.go` |

---

## 4. 详细设计

### 4.1 `AIConf` 扩展

`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:223`：

```go
type AIConf struct {
    // ... 现有字段不变 ...
    // ProtocolPaths 为 protocol -> 上游 base path 的映射（SDK base_url 的 path 部分）。
    // key 取值域 {openai, anthropic}（与改写公式支持的协议一致）；空/nil = 关闭（纯透传）。
    // 加载期校验：未知 key、非法 value 拒绝加载（见下方"加载行为"）。
    ProtocolPaths map[string]string
}
```

加载行为：`AIConf` 现有字段多数无 JSON tag，新字段沿用默认序列化命名（`ProtocolPaths`）；`nil`/空 map 与"字段不存在"等价，均为关闭。**BFE 加载期做校验**（落点为既有 `AIConfCheck`，`cluster_conf_load.go:1163`，cluster 加载流程已调用）：

- **key 白名单**：key 必须 ∈ {openai, anthropic}。未知 key（拼写错误、控制面 bug、手工改 conf 引入的 `gemini` 等）**拒绝加载**（启动失败 / 热加载拒绝）——与 `ValidateProtocols` 校验 `ModelProtocols` 的既有先例一致（`bfe_server/bfe_confdata_load.go:54-56`）。BFE 是配置的终极消费者，只依赖控制面校验会被手工改 conf、旧版控制面等路径绕过；配置错误应当响亮失败，而不是运行期静默透传成上游 404。
- **value 格式**：必须 `/` 开头、不以 `/` 结尾、不含 `..`/`?`/`#`、长度 ≤ 128（与控制面同规则，防手误）。
- 运行期语义保持不变：通过加载校验后，按 AuthStyle 查表，**未配置该协议的 base 即透传**（默认分支）。

### 4.2 执行点：`doSingleAIForward` 内（每次 attempt 重算）

**位置**：`bfe_server/reverseproxy.go:1533-1535` 创建 `outreq` 之后、`:1604` `clusterInvoke` 之前。选此位置的依据：

- **协议已知**：`:1513-1515` 已确保 `aiMeta.AuthStyle` 非空（未识别时经 `DetectAuthStyle` 探测）；
- **cluster 已知**：`cluster.AIConf` 在手（`:1522` 协议匹配、`:1552` `computeTargetModel` 同款用法）；
- **每次 attempt 独立**：`doSingleAIForward` 是 fallback/重试循环（调用点 `:1642`、`:1710`）的每次迭代体，在此改写天然满足"fallback 切换 cluster 后按新配置重算"；
- **原始路径不被污染**：`outreq` 是入站 `req` 的拷贝（`:1534`），改写只落在 `outreq.URL.Path`、**不改** `basicReq.HttpRequest`，因此下一次 attempt 的拷贝始终从原始客户端路径出发——与 `:1547-1551` 注释中 `computeTargetModel` "Always start from ClientModel for every cluster attempt" 的既有原则同构。这是正确性关键，必须用测试固化（见 §6）。

**核心逻辑**（独立纯函数，便于单测）：

```go
// rewriteUpstreamPath 按（请求路径, 协议, cluster AIConf）计算上游路径。
// 语义：protocol_paths[protocol] = 该协议官方 SDK base_url 的 path 部分
//   openai    base 含 /v1 尾（/compatible-mode/v1、/api/v3、/coding/v1）
//   anthropic base 不含 /v1（SDK 自拼 /v1/messages：/apps/anthropic、/coding、/anthropic）
func rewriteUpstreamPath(reqPath string, authStyle string, aiConf *cluster_conf.AIConf) string {
    if aiConf == nil {
        return reqPath
    }
    base, ok := aiConf.ProtocolPaths[authStyle]
    if !ok || !isStandardV1Prefix(reqPath) {
        return reqPath // 未配置该协议 / 非标准入口：透传
    }
    if authStyle == bfe_basic.AuthStyleAnthropic {
        return base + reqPath                  // /apps/anthropic + /v1/messages
    }
    return base + reqPath[len("/v1"):]       // /api/v3 + /chat/completions
}

// isStandardV1Prefix: reqPath == "/v1" 或以 "/v1/" 开头（/v10/xxx 不算）
```

场景验证（覆盖本期已调研的全部 provider 形态）：

| 客户端路径 | AuthStyle | ProtocolPaths | 上游路径 | 正确性 |
|---|---|---|---|---|
| `/v1/messages` | anthropic | `{anthropic: /apps/anthropic}` | `/apps/anthropic/v1/messages` | ✓ 百炼 |
| `/v1/chat/completions` | openai | `{openai: /compatible-mode/v1}` | `/compatible-mode/v1/chat/completions` | ✓ 百炼 |
| `/v1/messages` | anthropic | `{anthropic: /anthropic}` | `/anthropic/v1/messages` | ✓ DeepSeek/Kimi 平台 |
| `/v1/messages` | anthropic | `{anthropic: /coding}` | `/coding/v1/messages` | ✓ Kimi Code |
| `/v1/chat/completions` | openai | `{openai: /coding/v1}` | `/coding/v1/chat/completions` | ✓ Kimi Code |
| `/v1/chat/completions` | openai | `{openai: /api/v3}` | `/api/v3/chat/completions` | ✓ 火山按量 |
| `/v1/chat/completions` | openai | `{openai: /v1}` | `/v1/chat/completions`（恒等） | ✓ |
| `/v1/messages` | anthropic | `{anthropic: /api/compatible}` | `/api/compatible/v1/messages` | ✓ 火山按量 |
| 任意 | 任意 | 空/未配置 | 原样透传 | ✓ 向后兼容 |

**边界**：`isStandardV1Prefix` 只认 `/v1` 精确值与 `/v1/` 前缀——`/v10/xxx`、gemini 风格 `/v1beta/...` 不进入改写，直接透传。

### 4.3 AuthStyle 与 `ProtocolPaths` 键的一致性

`aiMeta.AuthStyle` 与 `ModelProtocols` 同命名空间：`clusterSupportsAuthStyle`（`reverseproxy.go:2179-2182`）直接拿 AuthStyle 与 `ModelProtocols` 的元素比较。因此 `aiMeta.AuthStyle` 可直接作为 `ProtocolPaths` 的 key，无需映射表；`AuthStyleUnknown` 不会命中任何 key，落入透传分支（与适配器选择 "unknown falls back to openai adapter" 的行为差异见 §7 风险表，本期接受：unknown 请求本就按 openai 协议语义处理，且上游若真是 openai 兼容，base 未配置时透传即正确）。

### 4.4 不动的部分

- **协议适配层**（`bfe_model_protocol/`）：接口零修改。路径改写是"端点知识"，适配层继续只管认证注入 / usage 提取 / 错误归一；
- **低层发送循环**（`Invoke`/`setBackendAddr`/`RoundTrip`，`:398`/`:409`）：零修改，`outreq.URL.Path` 原样流入；
- **`mod_ai_route` / `mod_ai_token_auth` / `mod_body_process` / `mod_ai_rate_limit`**：业务语义不变；访问日志 `ai_protocol` 继续由 AuthStyle 填充；
- **配置加载**：`bfe_confdata_load.go` 的 `ValidateProtocols` 不涉及本字段；无新增加载校验；
- **`conf/` 示例配置**：`AIConf.ProtocolPaths` 为空即关闭，示例不需新增；如需演示可在 sample cluster 中注释给出；
- **模型改写**：`computeTargetModel`（`:1478`）与本特性正交（一个改 body 的 model 字段，一个改 URL path），顺序无依赖。

---

## 5. 关键代码索引

| 文件 | 位置 | 现状 | 变更 |
|---|---|---|---|
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIConf`（:223） | 无路径字段 | 加 `ProtocolPaths map[string]string` |
| `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | 加载期校验（`AIConfCheck`，:1163） | 仅 `ModelTable` 解析校验 | 新增 `ProtocolPaths` key 白名单（{openai, anthropic}）+ value 格式校验，未知 key 拒绝加载 |
| `bfe/bfe_server/reverseproxy.go` | `doSingleAIForward`（:1504-1606） | outreq 拷贝后直接 `clusterInvoke` | 创建 outreq 后调用 `rewriteUpstreamPath` 赋值 `outreq.URL.Path` |
| `bfe/bfe_server/reverseproxy.go`（或同包新文件） | 新函数 | — | `rewriteUpstreamPath` + `isStandardV1Prefix` |
| `bfe/bfe_server/reverseproxy.go` | `:1534`、`:1547-1551` | outreq 拷贝语义 / "start from ClientModel" 原则 | 不变，作为本特性正确性依赖引用 |

---

## 6. 测试计划

### 6.1 单元测试（新增）

- `rewriteUpstreamPath`：§4.2 场景表全量（8 个改写 + 1 个透传）；边界（`/v1` 精确、`/v1/` 前缀、`/v10/xxx` 不透传、`aiConf == nil`、空 map、未配置的协议 key、未知 AuthStyle）；
- `doSingleAIForward` 级：带 `ProtocolPaths` 的 `AIConf` 下断言 `basicReq.OutRequest.URL.Path` 为改写值、入站 `basicReq.HttpRequest.URL.Path` 未被修改；
- **fallback 重算用例**（核心回归）：cluster A（`{anthropic: /apps/anthropic}`）失败 fallback 至 cluster B（`{anthropic: /coding}`），断言第二次 attempt 的上游路径为 `/coding/v1/messages` 而非 `/apps/anthropic/v1/messages`——固化"每次 attempt 从原始路径重算"的语义。

### 6.2 回归

- `bfe_server` 现有 AI 转发单测全量通过（尤其 fallback、key 重试、`computeTargetModel` 相关）；
- `go build ./...`、`go test`（bfe_config / bfe_server / bfe_route）、`go vet`、`gofmt -l` 全部通过。

### 6.3 端到端联调（与 ai-gateway-api 联调）

- 百炼 provider：`protocol_paths={openai: /compatible-mode/v1, anthropic: /apps/anthropic}`，客户端打 `/v1/messages` → 上游 `/apps/anthropic/v1/messages` 真实调用成功；
- 火山 Coding Plan provider：`protocol_paths={anthropic: /api/coding}`，客户端打 `/v1/messages` → 上游 `/api/coding/v1/messages` 真实调用成功；
- 未配置 `protocol_paths` 的存量 provider：全量回归（透传行为不变）。

---

## 7. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 旧 BFE 加载含 `ProtocolPaths` 的配置 | 实施时验证 `cluster_conf` 反序列化对未知字段的容忍度；若不容忍，发布顺序约束变为"严格 BFE 先行"（见 §8） |
| 配置先行生效语义错位：旧 BFE 容忍未知字段时，`ProtocolPaths` 被静默忽略，配置了改写的请求会打到 `/v1/...` 而非 provider 前缀（404/401） | **发布顺序硬约束：BFE 先于 ai-gateway-api 发布**（同 gemini 先例）；联调用例在发布后验证 |
| fallback 沿用上一 attempt 的改写后路径 | 由 outreq 拷贝语义保证（不改入站请求），§6.1 专利用例固化 |
| 未知 AuthStyle 请求不命中任何 key 落入透传，与适配器 "unknown → openai adapter" 的 fallback 语义存在差异 | 本期接受：unknown 请求本就按 openai 语义处理；若上游配置了 openai base 而请求未带可识别凭据，属客户端配置问题，透传结果与现状一致 |
| path 挂计费语义（火山订阅 Key 配 `/api/v3` 错扣费） | BFE 不感知计费域；由控制面文档标注 + 二期连通性探测兜底，不在本特性范围 |

回滚策略：新字段 + 一处赋值调用为纯增量代码，回滚 = revert 提交 + 控制面清空 `protocol_paths`（或回滚控制面发布）；`ProtocolPaths` 为空时全部路径回到透传，无数据面状态残留。

---

## 8. 与控制面的边界

- **发布顺序（硬约束）**：BFE 必须先于（或同步于）ai-gateway-api 发布。若控制面先导出 `ProtocolPaths` 而 BFE 未升级：BFE 加载失败（不容忍未知字段）或配置静默不生效（容忍未知字段）——后者比 gemini 场景（加载即失败）更隐蔽，因此顺序约束必须遵守。
- **配置格式**：`AIConf.ProtocolPaths map[string]string`，key 与 `ModelProtocols` 同取值域（本期仅 `openai`/`anthropic`）；由 ai-gateway-api `Provider.protocol_paths` 经 `newAIConf`（`model/icluster_conf/cluster.go:1341`）恒透传，cluster 不单独持有路径配置（与 `ModelProtocols` 同模式）。
- **控制面配套改动**（不属于本文档范围）：ai-gateway-api 侧 Provider/`ProviderParam` 增加 `ProtocolPaths` 字段、校验规则（key 白名单 + ⊆ `model_protocols`、路径格式）、`TProvider` 增列 DDL、`newAIConf` 透传、dashboard 表单。
- 访问日志、监控指标：`ai_protocol`、cluster/key 维度指标语义不变；如需新增"路径改写生效次数"指标，另行评审。
