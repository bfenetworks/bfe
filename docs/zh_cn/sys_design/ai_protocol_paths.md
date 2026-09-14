# AI 上游路径按协议改写（AIConf.ProtocolPaths）

## 1. 背景与目标

### 1.1 背景

BFE 对 AI 请求的上游路径**纯透传**：`doSingleAIForward` 中 `outreq` 是入站请求的拷贝（`bfe_server/reverseproxy.go:1534` 的 `*outreq = *req`），低层发送循环的 `setBackendAddr`（`reverseproxy.go:146-148`）只改写 `URL.Scheme`/`URL.Host`。因此**客户端必须按 provider 原生路径发起请求**——打百炼要自己带 `/compatible-mode/v1` 前缀，打火山要带 `/api/v3`——同一套客户端 SDK 配置无法复用到多家 provider。

对主流 provider 上游端点的调研表明：各家路径各不相同，且**同一 provider 的不同协议挂在不同前缀下**（最极端的百炼：OpenAI 兼容 `/compatible-mode/v1`、Anthropic 兼容 `/apps/anthropic`）：

| provider（产品线/套餐） | OpenAI 兼容（SDK base_url path） | Anthropic 兼容（SDK base_url path） |
|---|---|---|
| 百炼 DashScope | `/compatible-mode/v1` | `/apps/anthropic` |
| Kimi 开放平台（api.moonshot.cn） | `/v1` | `/anthropic` |
| Kimi Code 会员（api.kimi.com） | `/coding/v1` | `/coding` |
| DeepSeek | `/v1` | `/anthropic` |
| 火山方舟·按量 | `/api/v3` | `/api/compatible` |
| 火山方舟·Coding Plan | `/api/coding/v3` | `/api/coding` |

同品牌不同套餐（Kimi 开放平台 vs Kimi Code、火山按量 vs Coding Plan）是不同上游实体：端点、Key、计费方式三者绑定，由控制面建为不同 provider，各自配置各自的 `protocol_paths`。

**既有机制为什么不复用**：

- **`mod_rewrite`**（`bfe_modules/mod_rewrite/`，注册于 `HandleAfterLocation`）：执行时机在 AI cluster 选择之前，规则条件是 host/路径表达式，拿不到"目标 provider + 请求协议"这两个决定改写结果的上下文；把 provider 知识编码进 condition 表达式会使配置随 provider 数量线性膨胀，且与 provider/cluster 配置形成两个事实源。
- **provider/model 前缀路由**（`provider_model_prefix_routing.md`）：裁剪的是请求体 `model` 字段（`MatchPrefix`/`StripPrefix`），不改 URL 路径，与本特性正交。
- **协议适配层**（`model_protocol_adapter.md`）：收敛的是协议知识（认证头/usage/错误归一），路径是"端点知识"而非协议知识——同一协议族内不同 provider 路径各异。适配层零修改。

### 1.2 目标

1. `AIConf` 新增 `ProtocolPaths` 字段（协议 → 上游 base path 的声明式映射），空值 = 关闭；
2. 加载期校验：未知协议 key、非法 value 拒绝加载（与 `ValidateProtocols` 同先例）；
3. `doSingleAIForward` 按（请求协议, 当前 cluster 的 `ProtocolPaths`）改写 `outreq.URL.Path`；
4. fallback 切换 cluster 后按新 cluster 配置重算路径（每次转发 attempt 独立计算）；
5. 未配置 `ProtocolPaths` 时，转发行为与现状逐字节一致（纯透传）。

非目标：

- 不做协议翻译、不改写请求/响应体（透传原则）；
- 不支持 gemini 协议的路径前缀（原因见 §8.2）；不支持正则/任意 rewrite 规则、不改写 host/scheme；
- 不改 `mod_ai_route` 路由、`mod_ai_token_auth` 计费、`mod_ai_rate_limit` 限流语义；不改协议识别与 `ModelProtocols` 匹配（`PROVIDER_PROTOCOL_MISMATCH` 行为不变）；
- BFE 不感知 path 的计费语义（火山 `/api/v3` vs `/api/coding` 错配风险，见 §8.1）。

---

## 2. 设计原则

- **配置驱动**：是否改写、改写到什么前缀完全由 cluster 配置（`AIConf.ProtocolPaths`）决定，BFE 代码中不硬编码任何 provider 品牌或路径；
- **单机制、公式化**：只有"协议 → base path"一种配置形态，转发行为由 §4.2 的固定公式决定，不引入通用 rewrite 规则引擎——调研的全部 provider 均可被该公式表达，通用机制属于无真实需求的投机设计；
- **最小侵入**：改动收敛在 `AIConf` 结构体、`AIConfCheck`、`doSingleAIForward` 三处，协议适配层与低层发送循环零修改；
- **向后兼容**：`ProtocolPaths` 为空/nil（或字段不存在）时与现状行为完全一致；
- **fail-fast**：配置错误（未知协议 key、非法路径）在加载期响亮失败（启动失败/热加载拒绝），而不是运行期静默透传成上游 404。BFE 是配置的终极消费者，只依赖控制面校验会被手工改 conf、旧版控制面等路径绕过。

---

## 3. 配置模型

### 3.1 `AIConf` 扩展

**文件**：`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:223`

```go
type AIConf struct {
    // ... 现有字段不变 ...
    // ProtocolPaths 为 protocol -> 上游 base path 的映射（SDK base_url 的 path 部分）。
    // key 取值域 {openai, anthropic}；空/nil = 关闭（纯透传）。
    // 加载期校验：未知 key、非法 value 拒绝加载（AIConfCheck）。
    ProtocolPaths map[string]string
}
```

由控制面（ai-gateway-api `Provider.protocol_paths`）经 `newAIConf` 导出，cluster 恒透传 provider 值（与 `ModelProtocols` 同模式），cluster 不单独持有路径配置。

### 3.2 加载期校验

**落点**：`AIConfCheck`（`cluster_conf_load.go:1163`，既有 `AIConf` 校验入口，已被 cluster 加载流程调用）新增对 `ProtocolPaths` 的检查：

- **key 白名单**：key 必须 ∈ {openai, anthropic}。未知 key（拼写错误、控制面 bug、手工改 conf 引入的 `gemini` 等）返回 error → 启动失败 / 热加载拒绝。与 `ValidateProtocols` 校验 `ModelProtocols` 的既有先例一致（`bfe_server/bfe_confdata_load.go:54-56`）。
- **value 格式**：必须 `/` 开头、不以 `/` 结尾、不含 `..`/`?`/`#`、长度 ≤ 128（与控制面同规则，防手误）。

### 3.3 `protocol_paths` 的语义：SDK base_url

`protocol_paths[protocol]` = **该协议官方 SDK `base_url` 的 path 部分**：

- openai：含 `/v1` 尾（OpenAI SDK 向 base_url 拼 `/chat/completions` 等）——`/v1`、`/compatible-mode/v1`、`/api/v3`、`/coding/v1`；
- anthropic：不含 `/v1`（Anthropic SDK 自行拼 `/v1/messages`）——`/anthropic`、`/apps/anthropic`、`/coding`。

这一语义与实际请求路径完全自洽：`/apps/anthropic` + SDK 拼接的 `/v1/messages` = 百炼真实路径 `/apps/anthropic/v1/messages`；`/api/v3` + `/chat/completions` = 火山真实路径。选择 SDK base_url 语义而非"任意前缀"语义，是为了让配置值可直接照抄各 provider 官方文档的 `base_url` 一栏，降低配置错误率。

---

## 4. 转发逻辑

### 4.1 执行点：`doSingleAIForward`（每次 attempt 独立计算）

**位置**：`bfe_server/reverseproxy.go:1533-1535` 创建 `outreq` 之后、`:1604` `clusterInvoke` 之前。选此位置的依据：

- **协议已知**：`:1513-1515` 已确保 `aiMeta.AuthStyle` 非空（未识别时经 `DetectAuthStyle` 探测）；
- **cluster 已知**：`cluster.AIConf` 在手（`:1522` 协议匹配、`:1552` `computeTargetModel` 同款用法）；
- **每次 attempt 独立**：`doSingleAIForward` 是 fallback/重试循环（调用点 `:1642`、`:1710`）的每次迭代体，在此改写天然满足"fallback 切换 cluster 后按新配置重算"。

### 4.2 改写公式

```go
// rewriteUpstreamPath 按（请求路径, 协议, cluster AIConf）计算上游路径。
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

三个要点：

- **"标准入口"约定**：本特性依赖"网关对外入口 = `/v1/...`"这一约定（anthropic 入口 `/v1/messages`，openai 入口 `/v1/chat/completions` 等）；客户端直连 provider 原生路径（透传模式）不受影响——非 `/v1` 开头的路径一律透传；
- **协议差异内化在公式里**：anthropic 追加完整路径（SDK 自拼 `/v1/messages`），openai 去掉 `/v1` 前缀再追加（SDK 向含 `/v1` 尾的 base 拼资源路径）；
- **`AuthStyle` 直接作 key**：`aiMeta.AuthStyle` 与 `ModelProtocols` 同命名空间（`clusterSupportsAuthStyle`，`reverseproxy.go:2179-2182` 直接比较），无需映射表；`AuthStyleUnknown` 不命中任何 key，落入透传分支。

### 4.3 场景验证

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

边界：`/v1` 精确值与 `/v1/` 前缀才进入改写；`/v10/xxx`、gemini 风格 `/v1beta/...` 一律透传。

### 4.4 原始路径不被污染（正确性关键）

改写只落在 `outreq.URL.Path`，**不改**入站 `basicReq.HttpRequest`。由于 `outreq` 每 attempt 从原始请求重新拷贝（`:1534`），fallback 切换 cluster 后的下一次 attempt 始终从原始客户端路径出发重新计算——与 `:1547-1551` 注释中 `computeTargetModel` "Always start from ClientModel for every cluster attempt" 的既有原则同构。该语义必须用测试固化（fallback 从 cluster A `/apps/anthropic` 切到 cluster B `/coding` 的场景）。

---

## 5. 与既有机制的关系

| 机制 | 职责 | 与本特性的关系 |
|---|---|---|
| `computeTargetModel` + `MatchPrefix`/`StripPrefix` | 改写请求体 `model` 字段 | 正交：一个改 body、一个改 URL path，顺序无依赖 |
| 协议适配层 `bfe_model_protocol/` | 认证注入 / usage 提取 / 错误归一 | 零修改；路径是端点知识，不是协议知识 |
| `mod_rewrite` | 通用 URL 改写（host + condition） | 不适用：执行在 AI cluster 选择前、无 provider/协议上下文 |
| `ValidateProtocols` | 加载期校验 `ModelProtocols` | 同先例：本特性的 key 白名单校验沿用相同失败语义 |

---

## 6. 兼容性与发布

- **向后兼容**：`ProtocolPaths` 为空/nil（含字段不存在的存量配置）时，全部路径透传，行为与现状逐字节一致；无配置格式破坏性变更、无数据迁移；
- **发布顺序（硬约束）：BFE 先于（或同步于）控制面发布**。若控制面先导出 `ProtocolPaths` 而 BFE 未升级：BFE 加载失败（JSON 不容忍未知字段）或配置被静默忽略（容忍未知字段）——后者使配置了改写的请求打到 `/v1/...` 而非 provider 前缀（上游 404/401），比加载即失败更隐蔽；
- **未来协议扩展**：新增支持改写的协议时，按 gemini 先例"先升 BFE（公式支持 + 白名单放开）、再放控制面"；若出现"非 `/v1` 入口、同协议不同资源异前缀"等公式表达不了的真实 provider，优先把 `ProtocolPaths` 从单前缀演进为"协议内规则列表"（增量、向后兼容），不引入 provider 级通用 rewrite。

---

## 7. 测试

- **单元测试**：`rewriteUpstreamPath` 覆盖 §4.3 场景表 + 边界（`/v1` 精确、`/v1/` 前缀、`/v10/xxx` 不透传、`aiConf == nil`、空 map、未配置的协议 key、`AuthStyleUnknown`）；`AIConfCheck` 校验全分支（未知 key、value 格式非法）；
- **转发级测试**：带 `ProtocolPaths` 的 `AIConf` 下断言 `basicReq.OutRequest.URL.Path` 为改写值、入站 `basicReq.HttpRequest.URL.Path` 未被修改；**fallback 重算专利用例**（cluster A 失败 fallback 至 cluster B，断言第二次 attempt 路径按 B 重算）；
- **回归**：`bfe_server` 现有 AI 转发单测全量（fallback、key 重试、`computeTargetModel`）；
- **端到端联调**：百炼（`/v1/messages` → `/apps/anthropic/v1/messages`）、火山 Coding Plan（`/v1/messages` → `/api/coding/v1/messages`）真实调用；存量未配置 provider 全量回归。

---

## 8. 边界与开放问题

### 8.1 计费语义

path 决定计费（火山同一模型同一协议下 `/api/v3` 按量、`/api/coding` 订阅并行存在），配置错配会错扣费（订阅 Key 配 `/api/v3` 产生额外按量费用）。**BFE 不感知计费域**，由控制面命名规范与文档标注约束；key↔端点连通性探测（创建 provider 时最小请求鉴权校验）为控制面二期能力，不在本特性范围。

### 8.2 gemini 协议不支持

非技术障碍，纯需求取舍：gemini 原生路径（`/v1beta/models/{model}:generateContent`）本身就是标准路径，透传已可用，rewrite 对 gemini 没有可改写目标；调研的 provider 也均未提供 gemini 协议端点。技术上支持只需把"标准入口前缀"与拼接变体做成 per-protocol 参数，但为零需求的假想场景推广公式模型不符合本设计"单机制、公式化"原则。BFE 侧由加载期 key 白名单兜底拒绝。

### 8.3 入口路径规范

公式依赖"网关对外入口 = `/v1/...`"约定，需在对外文档/SDK 中明确；客户端以 provider 原生路径访问（透传模式）的能力不受影响。

---

修改方案与实施记录见 `bfe/docs/zh_cn/modifications/2026-09-14-ai-protocol-paths-rewrite/design-changes.md`。
