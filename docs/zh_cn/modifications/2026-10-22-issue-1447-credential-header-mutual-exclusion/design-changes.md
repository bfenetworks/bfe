# BFE Issue #1447 修复方案：客户端残留凭证头的互斥注入

- Issue: https://github.com/bfenetworks/bfe/issues/1447
- 生产事件：2026-10-22 kimi 集群 6 次上游 401 → 2 把 Key 全部判死（3600s 惩罚）→ 集群熔断、流量静默降级到 `bailian/deepseek-v4.1-flash`
- 代码库：`bfe/` **v1.8.8 分支**（tip `refs/heads/v1.8.8` = `179e7341`，本文所有行号逐条核实于该提交）
- 关联：#1439（惩罚过滤口径）、#1440（Key 耗尽合成 429）、#1441（Key 耗尽错误契约）、#1442（单可用 Key 集群）
- 裁定状态：§九 的 5 个裁定点**已全部确认**（2026-10-22，全部采纳本文推荐）；本文档为定稿方案，尚未实施（§十）
- 性质：**产品缺陷（代码缺陷）**，不是 Key 失效、额度问题或配置问题。Key 本身有效（issue 侧直连：同一把 Key 仅带 Bearer → 200）。

## 一、结论先行

**缺陷一句话**：`InjectAuth` 只*覆盖*自己那一个凭证头，不*删除*客户端带来的另外两个凭证头；上游按自己的优先级挑头（kimi 认 `x-api-key`），于是客户端那个无效 token 成了实际生效的上游凭证 → 401 → Key 被判死。

**修复一句话**：在网关**接管上游凭证**的那一刻（`selectedKey.Key != ""` 时），出站请求必须**有且只有一个**凭证头 —— 即本次注入的那个；其余 `Authorization` / `x-api-key` / `x-goog-api-key` 一律从出站请求里删除，且清理**不得改动入站请求的 header**（客户端凭证在鉴权/计费/脱敏/亲和性上仍要完整可用）。

**为什么必须物理删除、而不是"让网关覆盖的那个头被上游优先采用"**：上游对三个凭证头的优先级是**provider 私有且方向相反**的（issue 直连实测：kimi `x-api-key` 优先且可回落；bailian `Authorization` 优先且不回落）。网关无法按 provider 逐一声明"我最优先"——唯一能同时对两个上游成立的契约就是**线上只有一个凭证头**。这也回答了 issue「Oracle」里标注 **待确认** 的条款措辞（"是否必须物理删除残留头"）：**必须**。

## 二、缺陷机理（源码链，逐条实锤于 `179e7341`）

| # | 环节 | 位置 | 事实 |
|---|---|---|---|
| 1 | 风格识别只看"有没有 Authorization" | `bfe_model_protocol/detect.go:31-37`（`:40-42` x-api-key，`:45-47` x-goog-api-key） | 客户端同时带两个头 → 判定 `ProtocolOpenAI`，`AuthStyle=openai` |
| 2 | 识别结果写入请求上下文 | `bfe_basic/request_ai_basic.go:255-265`（`:261` 写 `ai.AuthStyle`） | 并发地 `GetApiKey` 取出的是 Authorization 里的值（**客户端网关凭证**） |
| 3 | 按风格取适配器 | `bfe_server/reverseproxy.go:1621` | `modelprotocol.Get("openai")` → openai 适配器 |
| 4 | 只覆盖、不删除 | `bfe_server/reverseproxy.go:1735-1739` 调 `adapter.InjectAuth`；`bfe_model_protocol/openai/auth.go:25-31` 仅 `Header.Set("Authorization", ...)` | 客户端的 `x-api-key` **原样留在 outreq 上**（全仓无 `Header.Del("x-api-key")`/`Del("x-goog-api-key")`） |
| 5 | 401 被当成"我们选中的 Key 鉴权失败" | `bfe_server/reverseproxy.go:1917-1926` | `state.deadSet[idx]` + `setKeyPenalty(..., 3600, ...)`，日志 `ai key [name=%s] auth failed (401), dead` |
| 6 | 惩罚严格剔除 → 两把 Key 全灭即熔断 | `bfe_server/reverseproxy.go:2262-2287`（`filterPenaltyKeys`） | 候选为空 → 合成 429（`:1860-1869`）→ `ServeHTTPForAI` 降级到 bailian → 客户端看到 200 |

放大条件（issue 现场）：kimi 集群只有 2 把 Key，且 `SessionAffinityPenaltyEnable=true` → 一条客户端请求 401 一次即可打死一把 Key 1 小时，两条请求打死整个集群（kimi 侧）。

## 三、本机复现（本次新增证据，2026-10-22，v1.8.8 / `179e7341`）

issue 中「WorkBuddy 实际发送了 `x-api-key`」此前只能是**强推断**（网关不落请求头）。本次用 SC06 集成环境（真实 `bfe` 进程 + mock 后端）直接观测，把该推断升级为**网关侧直读事实**：

**证据 1 — 客户端双头 → 上游实际收到的头**（客户端：`Authorization: Bearer ak_user_a` + `x-api-key: ak_user_a`，集群 `cluster_both_protocols`，keys `sk-both-openai-key`）

```
EVIDENCE status=200 body={"choices":[{"message":{"content":"ok"}}],"usage":{...}}
EVIDENCE upstream Authorization=["Bearer sk-both-openai-key"]      ← 网关注入的上游凭证
EVIDENCE upstream x-api-key=["ak_user_a"]                          ← 客户端残留，未被删除（缺陷本体）
EVIDENCE upstream x-goog-api-key=[""]
```

⇒ issue 的 A-01（"出站有效凭证 == 本次选中 Key"，`model_protocol_adapter.md#4`）在网关上**直接可观测地不成立**：出站请求带了两个凭证头。

**证据 2 — 让上游表现得像 kimi（`x-api-key` 非空即 401），故障在测试环境 7 秒内复现**

```
[INFO] aiClusterInvoke: select ai key [name=both-openai-key weight=50] for cluster[cluster_both_protocols]
[INFO] aiClusterInvoke: ai key [name=both-openai-key] auth failed (401), dead
EVIDENCE kimi-like status=401 body={"error":{"message":"The API Key appears to be invalid or may have expired","type":"invalid_authentication_error"}}
```

⇒ 与生产 `bfe.log` 的指纹（`ai key [name=…] auth failed (401), dead`）逐字一致，且**不需要真实上游、不需要 WorkBuddy 客户端**即可回归验证。

复现方式：SC06 包内临时用例（mock 后端 `ResponseFunc` 按请求头返回 401；`MockBackend` 已记录 `Authorization` / `x-api-key` / `x-goog-api-key`，见 `tests/integration/common/mock_backend.go:120-135、209-228`）。**该临时用例已删除**，其正式版本属于本文 §六 回归测试（SC06 TC-06 扩展 + 新增 TC-09）。

## 四、事实基线（源码定点，决定改法与边界）

### 4.1 三个凭证头是"客户端 → BFE"的网关凭证，不是上游凭证

- 产生/消费链：`http_conn.go:558-572`（`GetApiKey` → `aiMeta.ClientApiKey`，**转发前早退捕获**）→ `mod_ai_token_auth/token_rule_table.go:99`（鉴权/配额）→ `mod_access_pb3/credential_mask.go:43-46`（日志脱敏用 `ClientApiKey`）。
- 上游凭证另有来源：`cluster.AIConf.Keys`（Key 池），由 `InjectAuth` 写入。两者**语义互斥**，同一条出站请求不应同时携带。

### 4.2 注入路径与"透传模式"的边界

- `bfe_server/reverseproxy.go:1605-1610` `doSingleAIForward(..., selectedKey cluster_conf.AIKey, ...)`：`selectedKey` 是本次选中的上游 Key。
- `:1615-1617` 补探测风格；`:1621` 取适配器；`:1624-1632` 协议白名单校验；`:1644` 按风格重写上游路径。
- `:1635-1637` `outreq := new(bfe_http.Request); *outreq = *req` —— **`Header` 是 map，浅拷贝后 `outreq.Header` 与入站 `req.Header` 是同一个 map**。
- `:1727-1740` `if cluster.AIConf != nil { ... if selectedKey.Key != "" { adapter.InjectAuth(outreq, selectedKey.Key) } }`；`:1744-1748` 追加 `ExtraHeaders()`（anthropic-version）。
- **透传（BYOK）模式**：`:1791-1794`（`cluster.AIConf == nil || len(Keys) == 0` → 以空 `AIKey{}` 调用，`selectedKey.Key == ""` → 不注入）⇒ 客户端自带凭证被有意转发给上游。文档口径：`multi_api_key.md:73`（`Keys` 为空 = no key injection）。
- ⇒ **清理必须挂在"网关确实接管了凭证"这一条件上**（`selectedKey.Key != ""`），否则会破坏透传模式。

### 4.3 「不就地改入站 header」的既有惯例

- `bfe_server/reverseproxy.go:115-145` `hopByHopHeaderRemove`：注释明确"this is modifying the same underlying map from req (shallow copied above) so we only copy it if necessary"，即**需要删除时先 `bfe_http.CopyHeader` 复制一份**（`:138-142`）。清理凭证头必须沿用同一写法（`Header.Clone()`，`bfe_http/header.go:90-98`）。
- 为什么重要（不改入站会被破坏的点）：
  1. `bfe_server/reverseproxy.go:2196-2208` `clientKeySessionID`：`ClientKeyId` 为空时回落 `bfe_basic.GetApiKey(basicReq)`（**读入站头**）；**降级重试**会重新调用 `aiClusterInvoke`（`resetRequestForRetry` `:2013-2034` 只重置 `OutRequest`，入站头是唯一凭证来源）。若清理就地作用于入站 map，降级后 sessionID 会退化成"上一集群上游 Key 的哈希"。
  2. 入站凭证是鉴权/计费/日志脱敏的原始事实来源，转发阶段不应改写它（现状 `InjectAuth` 由于共享 map，实际上**已经**把入站 `Authorization` 改成了上游 Key —— 这是既有副作用，本次顺带消除）。
- 每次 attempt（含同集群 Key 轮转 `:1833-1877`、跨集群降级）都从头 `*outreq = *req` 重建 outreq ⇒ 清理动作天然幂等，必须**每次尝试都执行**。

### 4.4 Key 惩罚语义（本卡不得改动）

`401/402/403 → deadSet + 3600s 惩罚`（`:1917-1926`）、`429 → 60s 惩罚 + 轮转`（`:1907-1916`，TTL 常量 `aiKeyPenaltyTTL429` `:1959`）是 #1439/#1440/#1441 收敛后的**设计意图**（`multi_api_key.md`：某个 Key 因限流/鉴权失败失效时切到其他 Key）。本卡只消除"**由客户端残留头导致的伪 401**"，不改变真 401 的处理。

## 五、修复方案

### 5.0 四个设计原则（新不变量）

- **I-1（核心）**：出站请求携带网关注入的凭证时，**有且只有一个**凭证头 —— 即本次注入的那个。
- **I-2**：入站请求的 header map 在转发阶段**只读**；需要删除时先分离（copy-on-write）。
- **I-3**：不改变风格判定（`AuthStyle`）、不改变路径改写、不改变协议白名单/usage 链/计费 —— 本卡只做"清理 + 注入"，不做"改风格"。
- **I-4**：无 Keys 集群的透传语义不变（`selectedKey.Key == ""` 时不清理、不注入）。

### 5.1 步骤 1（必改，核心）：适配器声明自己的凭证头

`bfe_model_protocol/protocol.go`（接口新增一个方法，紧邻 `InjectAuth` 声明）：

```go
	// InjectAuth writes the upstream credential into the outgoing request.
	// An empty key is a no-op.
	InjectAuth(outreq *bfe_http.Request, key string) error

	// CredentialHeader returns the header InjectAuth writes the upstream
	// credential into (e.g. "Authorization"). Together with the other
	// adapters' credential headers it forms the set of client credential
	// headers that must not survive into an outbound request that carries a
	// gateway-injected credential (issue #1447).
	CredentialHeader() string
```

三个适配器（`auth.go` 内新增，与 `InjectAuth` 同文件，保持"协议知识就近"）：

| 文件 | `CredentialHeader()` 返回 |
|---|---|
| `bfe_model_protocol/openai/auth.go` | `"Authorization"` |
| `bfe_model_protocol/anthropic/auth.go` | `"X-Api-Key"` |
| `bfe_model_protocol/gemini/auth.go` | `"X-Goog-Api-Key"` |

`InjectAuth` 的实现与注释**不变**（空 key 仍是 no-op —— 这是透传模式的护栏）。全仓库只有这 3 个 `ProtocolAdapter` 实现（无测试替身），加方法不破坏任何调用点。

### 5.2 步骤 2（必改，核心）：新增凭证头清理能力（根包，随注册表自动扩展）

新文件 `bfe_model_protocol/credential.go`（根包可 import `bfe_http`；`utils` 叶子包保持零 BFE 依赖，不放这里）：

```go
// CredentialHeaderNames returns the credential headers declared by the
// registered adapters. They are the headers a client may use to authenticate
// to BFE (detect.go recognizes the same set) and therefore the set that must
// not reach an upstream that BFE authenticates on the client's behalf
// (issue #1447). The list is derived from the registry, so a newly registered
// protocol is covered automatically; it is sorted for stable logs/tests.
func CredentialHeaderNames() []string

// ForeignCredentialHeaders returns the recognized credential headers present
// in h other than keep (canonical forms, sorted). Presence includes
// present-but-empty values: an empty residual header is harmless upstream
// today, but "present means delete" keeps the rule unambiguous.
func ForeignCredentialHeaders(h bfe_http.Header, keep string) []string

// RemoveForeignCredentialHeaders deletes every recognized credential header
// other than keep and returns the names it removed.
func RemoveForeignCredentialHeaders(h bfe_http.Header, keep string) []string
```

实现要点：
- 比较统一走 `bfe_http.CanonicalHeaderKey`（`bfe_http/header.go:244`）；存在性判定用 `h.Values(name)`（大小写不敏感，`header.go:67`），删除用 `h.Del(name)`（`header.go:81`）；
- `CredentialHeaderNames()` 直接遍历 `registry.go` 的 `adapters`（含去重 + 排序），**不维护第二份硬编码清单**——新增协议时"忘记加进清理集"这个失误在结构上不可能发生。

### 5.3 步骤 3（必改，核心）：注入前完成清理（带 copy-on-write）

`bfe_server/reverseproxy.go` 新增包级函数（便于单测，无需起服务）：

```go
// sanitizeOutboundCredentials enforces issue #1447: an outbound request that
// carries a gateway-injected credential must carry exactly that one. The
// outbound header map aliases the inbound one (shallow copy in
// doSingleAIForward), so the map is detached before deleting anything — the
// client credential must stay intact on the inbound request (auth, billing,
// access-log masking, key-affinity session id). Returns the dropped header
// names so the caller can log them (names only, never values).
func sanitizeOutboundCredentials(outreq *bfe_http.Request, keep string) []string {
	foreign := modelprotocol.ForeignCredentialHeaders(outreq.Header, keep)
	if len(foreign) == 0 {
		return nil
	}
	outreq.Header = outreq.Header.Clone()
	modelprotocol.RemoveForeignCredentialHeaders(outreq.Header, keep)
	return foreign
}
```

调用点在 `doSingleAIForward` 的注入块内（现状 `:1735-1739`）：

```go
		if selectedKey.Key != "" {
			// issue #1447: the gateway takes ownership of the upstream
			// credential here, so the client's own credential headers must not
			// survive into the outbound request.
			if dropped := sanitizeOutboundCredentials(outreq, adapter.CredentialHeader()); len(dropped) > 0 {
				log.Logger.Info("doSingleAIForward: drop client credential headers %v for cluster[%s] (injected credential is %s)",
					dropped, cluster.Name, adapter.CredentialHeader())
			}
			if err := adapter.InjectAuth(outreq, selectedKey.Key); err != nil {
				log.Logger.Warn("doSingleAIForward: inject auth failed: %v", err)
			}
		}
```

顺序：**清理 → InjectAuth → ExtraHeaders**（`ExtraHeaders` 只补非凭证头，顺序无影响，保持现状）。

### 5.4 为什么清理点在被注入时，而不是"早于风格判定"（issue 的备选方案）

issue 建议"清理点必须早于风格判定"——**仅当目标是"把 openai 风格改成 anthropic 风格"时**才需要那样做，而那是另一个设计变更，本卡明确不做：

1. `AuthStyle` 决定的不只是认证头：还包括**上游路径改写**（`:1644` → `ai_path_rewrite.go:47`）、**协议白名单校验**（`:1624-1632`，不匹配直接 400）、**usage 解析链**（`mod_body_process`/`mod_ai_token_auth` 按 `AuthStyle` 取适配器）、**计费字段链**。改风格 = 改路由与计费语义，必须走产品评审（并与"按 cluster 强制风格"一起做）。
2. 本缺陷的**充分修法**就是删掉残留头：kimi 只看到 `Authorization: Bearer <网关 Key>` → 200；bailian 同理。风格保持 openai，一切既有语义不变。
3. 若将来引入"按 cluster/Provider 强制 `AuthStyle`"（issue 指出的 mirror 风险：只强制风格不清理 = 把故障从 kimi 搬到 bailian），它**必须与本文的清理同批落地**（见 §九 裁定 4）。

### 5.5 步骤 4（必改）：文档同步

| 文档 | 改动 |
|---|---|
| `docs/zh_cn/sys_design/model_protocol_adapter.md` | §3.2 接口表新增 `CredentialHeader()`；§4 生命周期表"认证头注入"一行补"互斥注入：仅保留本次注入的凭证头"；§6 新协议接入指南第 2 步补"实现 `CredentialHeader()`；新协议的凭证头会自动纳入出站清理集"；§8 边界表补一条"客户端残留凭证头（issue #1447）" |
| `docs/zh_cn/sys_design/multi_api_key.md` | Key 注入段落补一句：网关注入上游 Key 时，出站请求只保留该凭证头，客户端其它凭证头被删除（附 issue #1447） |
| `docs/zh_cn/sys_design/claude_protocol_support.md` | `:293` 表格"同时存在 Authorization 和 x-api-key → 优先按 OpenAI 风格处理"补"+ 出站时删除客户端 `x-api-key`" |
| `tests/integration/测试设计文档/scenario-SC06-Claude协议支持/TC-06-双头存在时Authorization优先.md` | 预期结果补：上游收到的 `x-api-key` 必须为空（残留被删除） |
| 新增 `tests/integration/测试设计文档/scenario-SC06-Claude协议支持/TC-09-客户端残留凭证头被清理.md` | 见 §六 第 7 条 |
| 本卡 | 实施后在本文件补 §十 实施记录 |

### 5.6 步骤 5（不在本卡范围，见裁定 3）：可观测性

本卡的 Info 日志（`drop client credential headers [...]`，只打头名）已足够事后归因。**裁定 3 结论：本卡不加 `ProxyState` 计数**，先用日志观察一段时间；若后续要量化"有多少客户端带残留凭证头"以决定是否在客户端侧治理，再补一个 `*metrics.Counter` 字段（`bfe_server/proxy_state.go` 新增字段即被 `web-monitor` 自动注册，`server_status.go:130` 的 `ProxyMetrics.Init` 按字段反射初始化）并在 `multi_api_key.md` 指标表补行。

## 六、回归测试

### 单元测试

1. **适配器矩阵（新增，`bfe_model_protocol/*/auth_test.go`）**：对三种风格各构造带全部三个凭证头的请求，断言 `InjectAuth` 后仅目标头存在、且值正确；同时断言 `CredentialHeader()` 返回值；
2. **透传护栏（新增，必测）**：`InjectAuth(req, "")` 后**三个凭证头全部原样保留**（无 Keys 集群的 BYOK 透传不被破坏）；
3. **清理能力（新增 `bfe_model_protocol/credential_test.go`）**：
   - `CredentialHeaderNames()` == 注册表内所有适配器的 `CredentialHeader()` 并集（防新增协议漏加）；含大小写规范化断言；
   - `ForeignCredentialHeaders`：`keep` 被排除、大小写不敏感匹配、**存在的空值头也算 foreign**、无凭证头返回空；
   - `RemoveForeignCredentialHeaders` 返回被删名单且只删应删的；
4. **copy-on-write（新增 `bfe_server/reverseproxy_ai_test.go`）**：`TestSanitizeOutboundCredentials`
   - 入站 `Authorization`+`x-api-key`、`outreq` 与之共享 map（模拟 `*outreq = *req`）→ 调用后：`outreq` 无 `x-api-key`、**入站请求仍有 `x-api-key`**、两者 map 不再同一；
   - 无外来凭证头时不复制（`outreq.Header` 仍是原 map），返回空；
   - 返回值 == 被删头名；
5. **不回归**：`bfe_server` / `bfe_basic` / `mod_ai_token_auth` / `mod_body_process` 既有全量单测（尤其 `TestSetApiKey`、`TestInjectAuth` 系列、协议不匹配与 `ExtraHeaders` 用例）。

### 集成测试（`bfe/tests/integration`，单组件模式，真实 BFE + mock 后端）

6. **SC06 TC-06 扩展（必改）**：双头请求 → 断言 `backend.XApiKeyHeaders()[0] == ""`、`Authorization` 为集群 Key 之一；同时更新 `sc06_claude_protocol_test.go:305-307` 的旧注释（"may retain client value if present" —— 修复后不再保留）；TC-01 已有 `x-api-key` 必须为空的断言，自动成为护栏；
7. **SC06 新增 TC-09（必加）**：kimi-like mock（`MockBackend.ResponseFunc`：`r.Header.Get("x-api-key") != "" → 401`）：
   - 客户端双头 → **200**；上游只收到一个凭证头；`bfe.log` 无 `auth failed (401), dead`、无 `all usable keys penalized ... circuit break`；
   - （对照臂）断言 mock 确实处于"会被残留头打死"的模式：直接对 mock 端口发一条带 `x-api-key` 的请求得 401（防止用例变成假阳性）；
8. **对称格（SC06，与 TC-09 同批）**：bailian-like mock（`Authorization` 非空且非注入值即 401）—— openai 风格 + 客户端双头 → 200，且上游只收到 `Authorization`；覆盖 issue「机制对称性」表里当前可达的两种上游偏好；
9. **SC16（gemini）**：双头/三头客户端请求 → 上游仅收到 `x-goog-api-key`（`Authorization`/`x-api-key` 均为空），扩展现有 TC-05；
10. **不回归**：SC06 / SC16 全量；SC02（多 Key 惩罚与耗尽出口）、SC10（Key 亲和性，`clientKeySessionID` 路径）、SC27（上游错误归一）、SC01、SC05；`go vet ./...`。

**验证方式**：`cd bfe && go test ./bfe_model_protocol/... ./bfe_server/... && cd tests/integration && go test -count=1 ./implementation/scenario-SC06-... ./implementation/scenario-SC16-...`（本机 2026-10-22 已确认该集成环境可跑：首次运行触发 `go build` 并把二进制缓存到 `tests/integration/.integration-test-bin/bfe-windows-amd64-179e7341…exe`，之后 SC06 单用例约 1~7 秒）。

## 七、修复后目标行为（逐条对照 issue 的 A-01~A-05）

| 断言 | 现状（issue 现场 + §三 复现） | 修复后 |
|---|---|---|
| A-01 出站有效凭证 | 出站带 `Authorization`(注入) + `x-api-key`(残留) 两个头，上游取残留 → 401 | 出站**只有一个**凭证头，值 == 本次选中 `AIKey.Key` |
| A-02 多凭证头客户端的鉴权结果 | 与只带 `Authorization` 时**不一致**（100% 失败） | 与只带 `Authorization` 时**一致**（上游只看到注入凭证） |
| A-03 鉴权失败后的 Key 处理 | 伪 401 → dead + 3600s 惩罚 → 2 Key 全灭熔断 | 不再产生伪 401；真 401 的 dead/惩罚语义**保持不变**（#1439/#1440/#1441 契约不动） |
| A-04 客户端可见结果 | 200，但已被静默降级到 `deepseek-v4.1-flash` | 200，且是**规则的原始目标集群/模型**（无降级） |
| A-05 同一客户端在不同上游的一致表现 | kimi 100% 401 / bailian 100% 200 | 两个上游表现一致（都只收到网关注入的那一个凭证头） |

出站请求形态（openai 风格、kimi 集群为例）：

```
Authorization: Bearer <选中 AIKey.Key>     ← 唯一凭证头
（x-api-key / x-goog-api-key 均不存在，无论客户端是否发送）
```

## 八、边界与残余风险

- **不做"按 cluster 强制 `AuthStyle`"（裁定 4）**：见 §5.4、§九。本卡只做清理；强制风格另开卡立项，且立项时**必须与本文清理同批落地**（否则故障从 kimi 搬到 bailian，issue「机制对称性」已论证）。
- **`api-key`（Azure 风格）等"网关不识别但上游可能认"的凭证头不在本卡清理集（裁定 2）**：当前无任何适配器注入 `api-key`，`detect.go` 也不识别它，因此该 mirror 故障**当前不可达**；清理由"注册表并集"驱动 ⇒ 一旦出现注入 `api-key` 的适配器（`model_protocol_adapter.md` §8 的 per-cluster 认证变体），它自动被纳入。在此之前，客户端带来的 `api-key` 到 Azure 类上游仍是残余风险——已裁定**另开卡跟进**（需产品先确认"哪些头的透传是有意的"）。
- **真 401 仍会惩罚 Key**：本卡只消除伪 401。容量/熔断参数（2 Key、3600s）与告警见 #1442 §三。
- **透传（无 Keys）集群不受保护**：这是有意设计（`multi_api_key.md:73`），客户端凭证必须转发；此类集群的风险由"上游本来就该用客户端凭证"承担。
- **行为变化清单（变更说明用）**：
  1. 出站请求不再携带客户端的其它凭证头（**唯一可见的协议变化**，也是修复目标）；带 `x-goog-api-key`/多余头的多凭证客户端同样被清理；
  2. 入站请求的 `Authorization` **不再**被上游 Key 覆盖（现状因共享 map 会被覆盖）：`clientKeySessionID` 在"`ClientKeyId` 为空 + 发生降级"场景下的哈希来源由"上一集群上游 Key"变回"客户端 Key"——Redis 亲和性绑定键随之变化（旧绑定自然过期，TTL 见 `SessionAffinityTTL`，影响 ≤1 个 TTL）；
  3. `bfe.log` 新增一条 Info（`drop client credential headers [...]`），仅在确有残留头时打印。
- **未验证项**：本次未复跑 issue 中 kimi/bailian 真实上游的直连对照（属 issue 侧证据，本机无上游凭证）；也**不再需要**证明 WorkBuddy 具体发了哪个头 —— 修复保证"任何客户端带来的外来凭证头都被清理"，与该客户端行为无关。

## 九、裁定结论（2026-10-22，owner：全部采纳本文推荐）

| # | 裁定点 | 结论 |
|---|---|---|
| 1 | 清理点位置 | **采纳推荐**：在网关接管凭证时（`selectedKey.Key != ""`）清理，**不改风格**（§5.4） |
| 2 | 清理集口径 | **采纳推荐**：注册表并集（当前 = `Authorization` / `X-Api-Key` / `X-Goog-Api-Key`）；`api-key` 等未识别凭证头本卡不纳入，另开卡跟进（§八） |
| 3 | 监控计数 | **采纳推荐**：本卡只加 Info 日志，不加 `ProxyState` 计数（§5.6） |
| 4 | 按 cluster/Provider 强制 `AuthStyle` | **采纳推荐**：本卡不做，另开卡立项；立项时必须与本文清理同批落地 |
| 5 | 控制面（ai-gateway-api）侧提示 | **采纳推荐**：超出本仓库范围，登记为跨仓跟进项，不在本卡 |

**裁定 1 — 清理点位置（已裁定：注入时清理，不改风格）。** 被否决的备选：在 `detect.go` 之前清理（会把风格改成 anthropic，连带上游路径改写 / 协议白名单 / usage 解析链 / 计费字段链变化，属独立设计变更）；或"只按 cluster 强制风格、不清理"（等于把故障搬到 bailian）。

**裁定 2 — 清理集口径（已裁定：注册表并集，本卡 = 三个头）。** 理由：与 `detect.go` 的识别集一致，且新增协议自动纳入（结构上不可能漏）。`api-key` 等"网关不识别但上游可能认"的头独立成卡——扩它的改动量很小（`CredentialHeaderNames()` 加一项 + 文档说明），但要先由产品确认"哪些头的透传是有意的"。

**裁定 3 — 监控计数（已裁定：本卡不加，先打日志观察）。** 指标成本低（`ProxyState` 加字段即自动注册），但会给指标表/看板新增项；先看日志量级再决定。

**裁定 4 — 按 cluster/Provider 强制 `AuthStyle`（已裁定：另开卡，本卡不做）。** issue 已论证该路径的 mirror 风险（只强制风格不清理 ⇒ bailian 立刻复现对称 401）与其当前不可达前提；立项时必须与本文清理同批落地。

**裁定 5 — 控制面提示（已裁定：不在本卡）。** "客户端残留凭证头的提示/诊断"（如 Key 使用诊断信息）属 ai-gateway-api 侧，登记为跨仓跟进项。

## 十、实施记录（2026-10-22，v1.8.8 分支）

方案按 §五 步骤 1~4 落地，未做步骤 5（裁定 3）。

### 改动文件

| 文件 | 改动 |
|---|---|
| `bfe_model_protocol/protocol.go` | `ProtocolAdapter` 新增 `CredentialHeader() string`（声明 `InjectAuth` 写入的凭证头，canonical 形式） |
| `bfe_model_protocol/openai/auth.go` | 新增 `CredentialHeader()` → `"Authorization"` |
| `bfe_model_protocol/anthropic/auth.go` | 新增 `CredentialHeader()` → `"X-Api-Key"` |
| `bfe_model_protocol/gemini/auth.go` | 新增 `CredentialHeader()` → `"X-Goog-Api-Key"` |
| `bfe_model_protocol/credential.go`（新增） | `CredentialHeaderNames()`（注册表并集：canonical、去重、排序）、`ForeignCredentialHeaders()`、`RemoveForeignCredentialHeaders()` |
| `bfe_server/reverseproxy.go` | 新增 `sanitizeOutboundCredentials()`（`Header.Clone()` 分离 + 删除 + 返回被删头名）；`doSingleAIForward` 在 `selectedKey.Key != ""` 门内、`InjectAuth` 之前调用，命中时打一条 Info（只含头名，不含值） |
| `bfe_model_protocol/credential_test.go`（新增） | 注册表并集一致性；`ForeignCredentialHeaders`（keep 豁免 / 大小写 / 空值头 / 无头 / nil map）；`RemoveForeignCredentialHeaders`（含幂等）；三适配器互斥矩阵（sanitize + inject 后只剩一个凭证头且值为注入 key）；空 key 透传护栏 |
| `bfe_model_protocol/*/auth_test.go` | 各新增 `TestCredentialHeader` |
| `bfe_server/reverseproxy_ai_test.go` | 新增 `TestSanitizeOutboundCredentials`（出站被清理、入站不动、无外来头时不复制 map、anthropic/gemini keep 分支） |
| `tests/integration/implementation/scenario-SC06-.../sc06_claude_protocol_test.go` | TC-06 补 `x-api-key` 必须为空的断言、修正旧注释（"may retain client value if present" 已不成立）；新增 `TestTC09_ClientCredentialHeadersSanitized`（kimi 型上游 / Authorization 优先上游 / anthropic 风格残留 gemini 头 三臂，含"该 mock 确实会被残留头打死"的对照臂） |
| `tests/integration/implementation/scenario-SC16-.../sc16_gemini_protocol_test.go` | TC-05 补 `x-goog-api-key`、`x-api-key` 必须为空的断言 |
| `tests/integration/测试设计文档/...` | SC06 TC-06 预期结果补残留头被删除；新增 `TC-09-客户端残留凭证头被清理.md`；SC16 TC-05 预期结果补清理断言；`测试场景总体说明.md`（SC06 用例数 8→9、TC 列表加 TC-09） |
| `docs/zh_cn/sys_design/*` | 见 §5.5：`model_protocol_adapter.md`、`multi_api_key.md`、`claude_protocol_support.md`，另补 `ai_access_log_fields.md` 的转发步骤清单 |

### 验证结果（本次实跑）

| 范围 | 命令 | 结果 |
|---|---|---|
| 构建 / vet | `go build ./...`、`go vet ./bfe_model_protocol/... ./bfe_server/...` | 通过 |
| 模块单测 | `go test ./bfe_model_protocol/... ./bfe_server/... ./bfe_modules/mod_ai_token_auth/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_body_process/... ./bfe_modules/mod_ai_rate_limit/...` | 全部通过 |
| 缺陷基线（修复前） | 临时用例（§三）：双头客户端 → 上游同时收到注入的 `Authorization` 与残留 `x-api-key`；kimi 型 mock → 401 + `ai key [...] auth failed (401), dead` | 复现成功 |
| 集成 SC06（含 TC-09 三臂、TC-06 扩展） | `go test -count=1 ./implementation/scenario-SC06-claude-protocol-support/` | 通过 |
| 集成 SC16（TC-05 扩展断言） | 同上 | 通过 |
| 集成 SC02（多 Key 惩罚 / 耗尽 / 降级） | 同上 | 通过 |
| **反向验证（证明用例有效）** | 临时把 `sanitizeOutboundCredentials` 改为 `return nil`（等价修复前行为）后重跑 TC-09：**三臂全部失败**——臂 A 复现 401 与 `auth failed (401), dead`，臂 B 报上游收到残留 `x-api-key`，臂 C 报上游收到残留 `x-goog-api-key`；恢复修复后通过 | 有效 |
| 既有回归 SC01 / SC05 / SC10 / SC17 | `go test -count=1 ./implementation/scenario-SC01-... ./implementation/scenario-SC05-... ./implementation/scenario-SC10-... ./implementation/scenario-SC17-...` | 全部通过 |
| 额外回归 SC11 / SC12 / SC18（计费与 AI 转发路径） | 同上 | 全部通过 |
| SC27（上游错误归一） | — | **不适用**：该场景 `implementation/` 下只有 `testdata/`，尚无 Go 用例可跑（方案 §六 第 10 条列了它，实际无法执行） |

### 与方案文档的偏差

1. 文档同步多改一个文件：`docs/zh_cn/sys_design/ai_access_log_fields.md` 的 `doSingleAIForward()` 步骤清单补"删除客户端残留凭证头"一行（方案 §5.5 原列 3 个 sys_design 文件）；该清单逐条描述转发步骤，不加会与实际流程不符。
2. **互斥清理放在转发层而非 `InjectAuth` 内**：`InjectAuth` 仍只写自己那一个头。原因是适配器子包不能 import 根包注册表（会成环），而清理集必须由注册表并集推导（新技术协议自动纳入）；`mod_ai_token_auth.SetApiKey`（legacy 包装，生产无调用方）行为不变。因此 §5.1 中 "Issue 建议的"备选（把删除塞进适配器）被替换为"适配器声明头名 + 转发层清理"。
3. 未新增监控计数（裁定 3）；未做"按 cluster 强制 `AuthStyle`"（裁定 4）与 `api-key` 清理（裁定 2）。
4. 透传集群（无 `Keys`）不清理、不注入（`selectedKey.Key == ""` 门），由单测 `TestInjectAuthEmptyKeyKeepsClientCredentials` 覆盖。

### 可观测变化（变更说明用）

1. 出站请求不再携带客户端的其它凭证头（修复目标）；带 `x-goog-api-key` 等多凭证头的客户端同样被清理；
2. 入站请求的 `Authorization` 不再被上游 Key 覆盖（现状因 outreq 与入站共享 header map 会被覆盖）：`clientKeySessionID` 在"`ClientKeyId` 为空 + 发生降级"场景下改回按客户端 Key 派生哈希，Redis 亲和性绑定键随之变化（旧绑定按 TTL 自然过期）；
3. `bfe.log` 新增一条 Info：`doSingleAIForward: drop client credential headers [...] for cluster[...], injected credential is ...`（仅在确有残留头时打印，只含头名）。

### 遗留事项

- 按 cluster/Provider 强制 `AuthStyle`：另开卡立项，必须与本次清理同批（裁定 4）；
- `api-key` 等"网关不识别但上游可能认"的凭证头清理：另开卡，需产品先确认透传白名单（裁定 2）；
- 监控计数：观察日志量级后再评估（裁定 3）；
- 控制面（ai-gateway-api）侧"客户端残留凭证头"提示：跨仓跟进项（裁定 5）；
- v1.8.9-dev 前向移植：`CredentialHeader()`、`credential.go`、`sanitizeOutboundCredentials` 与本次测试需一并移植。