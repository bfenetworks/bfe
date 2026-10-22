# BFE Issue #1447 修复方案（v1.8.9-dev 前向移植）：客户端残留凭证头的互斥注入

- Issue: https://github.com/bfenetworks/bfe/issues/1447
- 性质：v1.8.8 分支修复（commit `ebbebd39`，方案文档随发布提交 `1d49779f` 一并入库）向 v1.8.9-dev 的前向移植。缺陷机理、五个裁定点（清理点位置 / 清理集口径 / 监控计数 / 按 cluster 强制 `AuthStyle` / 控制面提示）、本机复现证据、回归设计见 v1.8.8 分支的同名文档（`docs/zh_cn/modifications/2026-10-22-issue-1447-credential-header-mutual-exclusion/design-changes.md` @ `ebbebd39`）；本文只记录 **dev 分支的移植差异与验证**。
- 缺陷（摘要）：`InjectAuth` 只*覆盖*自己那一个凭证头、不*删除*客户端带来的其它凭证头；上游按 **provider 私有且方向相反**的优先级挑头（实测 kimi 认 `x-api-key`、bailian 认 `Authorization`），残留头让上游用**客户端的**凭证鉴权 → 401 → 被误判为该 Key 鉴权失败并写入 3600s 惩罚键（生产现场：一个客户端两条请求即打死 kimi 两把 Key、集群熔断、流量静默降级到 `deepseek-v4.1-flash`）。
- 修复（摘要）：网关接管上游凭证时（`selectedKey.Key != ""`），出站请求**只保留本次注入的那一个凭证头**，其余凭证头在 `Header.Clone()` 后的副本上删除（入站 header 不被改写）；清理集 = 注册表内各适配器 `CredentialHeader()` 的并集。

## 一、dev 分支移植差异（相对 v1.8.8 版）

### 1. 注入点上下文：保留 dev 的 `aiMeta.UpstreamKey` 记录

dev 在同一 `if selectedKey.Key != ""` 块内先写 `aiMeta.UpstreamKey = selectedKey.Key`（上游错误体脱敏的 pattern 来源，见 `bfe_server/reverseproxy_ai_error.go:261` `redactSecretMaterial`），随后才 `InjectAuth`。移植时**保留该赋值**，把清理块插在它与 `InjectAuth` 之间（`bfe_server/reverseproxy.go:1773-1792`）。

两者互不影响：脱敏用的是 key **值**，清理只动 header **名**；`UpstreamKey` 取值不变，因此 `AIConf.NormalizeUpstreamError` 的脱敏与归一化行为不变（该归一化器对带 `X-Bfe-Gw-Error` 标记的网关自生成响应本就跳过，本卡不产生任何错误响应，无交互）。

### 2. 相对行号不同，语义与别名关系一致

v1.8.8 文档中的行号以 `179e7341` 为准；dev 的 `doSingleAIForward` 因 batch 透传、上游错误归一等改动整体下移（`InjectAuth` 在 dev 为 `:1789`，v1.8.8 为 `:1736`；`sanitizeOutboundCredentials` 在 dev 为 `:1631`）。`*outreq = *req` 造成的出站/入站 header map 别名关系在 dev 完全相同，`Header.Clone()` 的 copy-on-write 语义不变。

### 3. 注册表与接口扩展（已核对）

dev 的 `ProtocolAdapter` 实现与 v1.8.8 相同（仅 openai / anthropic / gemini 三个，`bfe_model_protocol/registry.go:31-33`），新增 `CredentialHeader()` 后清理集仍为 `{Authorization, X-Api-Key, X-Goog-Api-Key}`；`credential_test.go` 的固定期望列表在 dev 成立（若后续新增协议，该用例会提示同步期望值，这也是它的作用）。

### 4. dev 特有的其它出站凭证写入点（已核对，不属本卡）

dev 的 `mod_ai_cache` 有两处自行写入 `Authorization` 的**侧向**调用：

- `bfe_modules/mod_ai_cache/provider/embedding/openai.go:110`（embedding 服务）；
- `bfe_modules/mod_ai_cache/provider/vector/chroma.go:250`（向量库）。

二者均用 `http.NewRequestWithContext` **自建请求**、只带 `Content-Type` 与自己的 `apiKey`，不复制客户端 header，因此不存在"客户端残留凭证头被上游采用"的问题，**不改**。

### 5. 冲突与解决（cherry-pick `ebbebd39`）

| 文件 | 冲突 | 解决 |
|---|---|---|
| `bfe_server/reverseproxy.go` | dev 在该块新增了 `aiMeta.UpstreamKey` 赋值 | 保留 dev 赋值，把清理块合入其后的 `if selectedKey.Key != ""`（两个 if 条件相同，按 dev 现有形态保留，不做合并重构） |
| `tests/integration/测试设计文档/测试场景总体说明.md` | dev 的 SC05 行已扩到 22 个用例（v1.8.8 为 19） | 采用 dev 的 SC05 行；SC06 行采用 #1447 版本（8→9 个用例并补说明） |

其余 20 个文件自动合并：`bfe_model_protocol/{protocol.go,credential.go,credential_test.go}`、各适配器 `auth.go`/`auth_test.go`、`bfe_server/reverseproxy_ai_test.go`、4 个 sys_design 文档、SC06/SC16 集成测试与测试设计文档、本方案文档。

## 二、改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/protocol.go` | `ProtocolAdapter` 新增 `CredentialHeader() string`（声明 `InjectAuth` 写入的凭证头，canonical 形式） |
| `bfe_model_protocol/openai/auth.go` | 新增 `CredentialHeader()` → `"Authorization"` |
| `bfe_model_protocol/anthropic/auth.go` | 新增 `CredentialHeader()` → `"X-Api-Key"` |
| `bfe_model_protocol/gemini/auth.go` | 新增 `CredentialHeader()` → `"X-Goog-Api-Key"` |
| `bfe_model_protocol/credential.go`（新增） | `CredentialHeaderNames()`（注册表并集：canonical、去重、排序）、`ForeignCredentialHeaders()`、`RemoveForeignCredentialHeaders()` |
| `bfe_server/reverseproxy.go` | 新增 `sanitizeOutboundCredentials()`（`:1631`）；`doSingleAIForward` 在 `selectedKey.Key != ""` 块内、`InjectAuth` 之前调用（`:1785`），命中时打一条只含头名的 Info |
| `bfe_model_protocol/credential_test.go`（新增） | 注册表并集一致性；`ForeignCredentialHeaders`（keep 豁免 / 大小写 / 空值头 / 无头 / nil map）；`RemoveForeignCredentialHeaders`（含幂等）；三适配器互斥矩阵；空 key 透传护栏 |
| `bfe_model_protocol/*/auth_test.go` | 各新增 `TestCredentialHeader` |
| `bfe_server/reverseproxy_ai_test.go` | 新增 `TestSanitizeOutboundCredentials`（出站被清理、入站不动、无外来头时不复制 map、anthropic/gemini keep 分支） |
| `tests/integration/implementation/.../scenario-SC06-...` | TC-06 补 `x-api-key` 必须为空的断言并修正旧注释；新增 `TestTC09_ClientCredentialHeadersSanitized`（kimi 型上游 / Authorization 优先上游 / anthropic 风格残留 gemini 头 三臂，含对照臂） |
| `tests/integration/implementation/.../scenario-SC16-...` | TC-05 补 `x-goog-api-key`、`x-api-key` 必须为空的断言 |
| `tests/integration/测试设计文档/*` | SC06 TC-06 预期结果、新增 `TC-09-客户端残留凭证头被清理.md`、SC16 TC-05 预期结果、`测试场景总体说明.md`（SC06 8→9） |
| `docs/zh_cn/sys_design/*` | `model_protocol_adapter.md`（§3.2 接口 / §4 生命周期与适配点表 / §6 新协议指南 / §7 关联文档 / §8 边界）、`multi_api_key.md`（§3.2 新增第 4 条）、`claude_protocol_support.md`（§7 失败处理表 + §10 风险表）、`ai_access_log_fields.md`（转发步骤清单） |

## 三、验证结果（v1.8.9-dev，本次实跑）

| 范围 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go vet ./bfe_model_protocol/... ./bfe_server/...` | 通过 |
| 单测：`bfe_model_protocol`（含新增 credential 用例）/ `bfe_server`（含 `TestSanitizeOutboundCredentials`）/ `mod_ai_token_auth` / `mod_ai_route` / `mod_body_process` / `mod_ai_rate_limit` | 全部通过 |
| 集成（串行 `-p 1`）：SC06（TC-06 扩展 + TC-09 三臂）4.9s、SC16（TC-05 扩展）5.5s、SC02 22.1s、SC27（dev 特有上游错误归一实现）14.1s、SC01 5.9s、SC05 22.3s、SC10 5.1s、SC17 3.5s | 全部通过 |
| 并发执行的说明 | 首次以 8 个包并发（默认 `-p`）执行时 SC16 失败（该包耗时由 5.5s 涨到 24.6s，BFE 进程启动与 mock 后端在资源竞争下超时）；单包重跑 10/10 通过、8 包串行复跑全部通过，判定为并发环境抖动，非代码回归 |

## 四、可观测变化（与 v1.8.8 相同）

1. 出站请求不再携带客户端的其它凭证头（修复目标）；
2. 入站 `Authorization` 不再被上游 Key 覆盖：`clientKeySessionID` 在"`ClientKeyId` 为空 + 发生降级"场景下改回按客户端 Key 派生哈希，Redis 亲和性绑定键随之变化（旧绑定按 TTL 自然过期）；
3. `bfe.log` 新增一条 Info：`doSingleAIForward: drop client credential headers [...] for cluster[...], injected credential is ...`（仅在确有残留头时打印，只含头名）。

## 五、遗留事项（同 v1.8.8 裁定）

- `api-key`（Azure 风格）等"网关不识别但上游可能认"的凭证头清理：另开卡，需产品先确认透传白名单（裁定 2）；
- 按 cluster/Provider 强制 `AuthStyle`：另开卡立项，且必须与本次清理同批落地（裁定 4）；
- 监控计数：先观察日志量级，再决定是否加 `ProxyState` 计数器（裁定 3）；
- 控制面（ai-gateway-api）侧"客户端残留凭证头"提示：跨仓库跟进项（裁定 5）；
- 两分支码值/行为已一致，本卡不涉及错误码变更，dev 的 `X-Bfe-Gw-Error` 标记与上游错误归一化器无需调整。