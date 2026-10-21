# BFE Issue #1441 修复方案：Key 耗尽路径的错误响应契约与可观测性

- Issue: https://github.com/bfenetworks/bfe/issues/1441
- 缺陷：Key 耗尽路径最终由 `bfe_basic.CreateInternalResp()` 合成的 HTTP 500 发出，响应只有 `Server: bfe` 与 0 字节 body，无 `Content-Type`、无 `{"error":{"code":...}}`、无退避信号；访问日志 `err_code`、`ai_auth_reject_reason` 均为空。客户端无法区分"上游限流（应退避）"与"网关内部错误"，按 5xx 无差别重试 → 实测请求量从基线 20–60 req/min 跃升至 **426 req/min**（约 10 倍，持续 4 分钟），放大上游压力并让惩罚窗口不断续期，形成正反馈。
- 代码库：`bfe/` **v1.8.8 分支**（tip `e6c97fd4`，本文行号逐条核实于该分支）。
- 关联：#1440（Key 耗尽返回 `(nil,nil)` 导致降级失效，**已修复**）、#1439（惩罚过滤口径，**已修复**）。**本卡只覆盖响应契约与可观测性**，不重复 #1440 的控制流缺陷。

## 一、与 #1440/#1439 修复后的现状基线（必读）

#1440 落地后，Key 耗尽出口**不再是空 body 500**：`aiClusterInvoke` 在选 Key 失败时经 `bfe_basic.CreateSpecifiedContentResp` 返回**合成 429**（`reverseproxy.go:1840-1841`），body 为 `{"error":{"type":"rate_limit_error","message":"no available upstream api key"}}`，`Content-Type: application/json`，且**故意不设 `ErrCode`**（避免误触 `ErrAiRateLimit` 的降级禁止短路）。

据此，issue 中三条件的现状为：

| 断言 | 修复前（issue 现场） | #1440 落地后当前状态 | 本卡是否仍需处理 |
|---|---|---|---|
| C-01 统一错误体（§3.1/§3.2：`error.code`/`type`/`message` 必填） | 空 body、无 `Content-Type` | **半合规**：有 `Content-Type` 与 `type`/`message`，但**缺 `error.code`**（§3.2 必填）、无 `details` | **是**（补 `code` 与 `details`） |
| C-02 日志可归因（§5） | `err_code=""`、`ai_auth_reject_reason=""` | **仍为空**（合成响应不设 `ErrCode`） | **是** |
| C-03 退避信号（`Retry-After` / `details.retry_after_seconds`） | 无任何退避提示 | **仍缺失**（`RetryAfterSeconds` 字段有定义但全仓无写入者、无消费者） | **是** |

> 结论：本卡在前序修复之上，补齐**错误码、结构化 details、退避秒数、日志归因**四件事；控制流（429 触发降级）已由 #1440 保证，本卡不得改变它。

## 二、事实基线（源码定点）

### 2.1 错误响应契约与目录（§3.1/§3.2/§4）

- 统一错误体定义：`bfe_basic/request_ai_basic.go:476-493`（`AiErrorDetail` / `AiError` / `AiErrorBody`），构造与响应：`:495-540`（`NewAiError`、`NewAiErrorWithDetails`、`CreateErrorResponse`——设置 `StatusCode`（查 `ErrorCodeToStatusCode`，`:427`）、`Server`、`Content-Type: application/json` 与 JSON body）。
- `AiErrorDetail.RetryAfterSeconds`：`:482`（JSON 字段 `retry_after_seconds`，§3.2 标注为"限流类错误预留"）。**全仓无写入者也无消费者**（`mod_ai_rate_limit` 构造 details 时未赋值，见 `mod_ai_rate_limit.go:252-260`）。
- §4 预留错误码（`request_ai_basic.go:391-424`）**没有**"网关侧无可用 Key/无可用凭证"一类码值；与容量相关的仅有 `BACKEND_UNAVAILABLE`(502, `:407`,`:454`)、`MODEL_INTERNAL_ERROR`(500)、`BACKEND_TIMEOUT`(504)。
- HTTP `Retry-After` 响应头：**v1.8.8 全仓无任何支持**。

### 2.2 日志归因链路

- `err_code`：`mod_access_pb3/request_log.go:127-131` 取 `req.ErrCode.Error()`（nil 写空串）；文本 access 另有 `mod_access/request_log.go:377`。
- `ErrCode` 类型为 `error`（`bfe_basic/request.go:87`）；哨兵定义见 `bfe_basic/error_code.go`（`ErrAiRateLimit = errors.New("AI_RATE_LIMIT")`，`:58`）。
- **`ErrCode` 的读取分两类（决定"写 ErrCode 是否安全"）**：
  - *改变控制流*：`reverseproxy.go:1336`（`== ErrAiRateLimit` → 禁止 cluster fallback）、`:1874`（同判断 → 停止 key 轮转）、`mod_ai_token_auth/mod_ai_token_auth.go:327`（`isClientAbortErr` 计费旁路，仅 `ErrClientWrite/Close/Reset`）。**只要不使用上述三类哨兵，写 ErrCode 不改变控制流。**
  - *只读归因*：`http_conn.go:613`（`ClientReqFail` 计数）、`mod_access_pb3/request_log.go:107`（log tag）、`:127-131`（`err_code`）。
- **写入的清理时机**：`reverseproxy.go:1986`（`resetRequestForRetry` 在每次 fallback 尝试前置 `ErrCode = nil`）、`:430`（roundtrip 成功后清空）。⇒ 只有"最后一次尝试"的 ErrCode 会留在最终日志里，**成功的 fallback 不会带上前一集群的失败归因**（与 #1387 的 `RejectReason` 清理语义一致）。
- `ai_auth_reject_reason`：源字段 `AiBasicInfo.AiAuthInfo.RejectReason`（`bfe_basic/request_ai_basic.go:90`），**仅由 `mod_ai_token_auth` 在鉴权/配额/模型白名单阶段写入**（`token_rule_table.go:90-96` 及 `:101/106/113/127/136/152/163/172/206`、`model_check.go:55/67/88`），日志输出于 `mod_access_pb3/request_log.go:528-529`，**唯一清空点**为 `reverseproxy.go:1321-1323`（fallback 成功时）。文档口径：`ai_error_codes.md:130` 明确其为"**认证/配额拒绝时**的错误码"。

### 2.3 其他本地合成响应路径（§3.1 要求"所有数据面错误响应"统一）

`reverseproxy.go` 中的本地合成点：

| 路径 | 构造 | 是否合规 |
|---|---|---|
| findProduct/findCluster/no-cluster/clusterInvoke 失败（`ServeHTTP` 与 `ServeHTTPForAI`：`:734`、`:773`、`:789`、`:898`、`:1191`、`:1368`） | `CreateInternalSrvErrResp` → 空 body 500 | ❌ |
| AI 无路由（`:1228-1229`） | `CreateSpecifiedContentResp(404, "text/plain", "AI route not found")` | ❌（非 JSON） |
| **Key 耗尽（`:1840-1841`）** | `CreateSpecifiedContentResp(429, json, …)` | ⚠️ 半合规（缺 `code`/`details`） |
| 协议不匹配（`:1606-1612`）、鉴权/配额（`mod_ai_token_auth`）、限流（`mod_ai_rate_limit`） | `AiError.CreateErrorResponse` | ✅ |

## 三、修复方案

设计原则：

1. **复用既有目录**：所有新响应体走 `bfe_basic.AiError.CreateErrorResponse`（不再手搓 JSON），保证 §3.1/§3.2 字段齐备；
2. **不改变控制流**：不引入 `ErrAiRateLimit`/`ErrClient*` 类哨兵；429 状态码保持（#1440 的降级触发条件不变）；
3. **归因落在 `err_code`**，不复用 `ai_auth_reject_reason`（后者语义为认证/配额拒绝，见 §2.2）；
4. **契约优先、范围可控**：本卡覆盖"Key 耗尽"现场路径（步骤 1~2）与网关侧合成 500 路径（步骤 3，裁定点 3b 已确认范围）；AI 无路由 404 与非 AI 链路另开卡收口。

### 步骤 1（必改，核心）：Key 耗尽响应改为统一错误体 + 新增错误码 + 退避秒数

**1.1 新增错误码**（`bfe_basic/request_ai_basic.go`，§4 预留码表同批更新）

```go
// Gateway-side capacity exhaustion: no usable upstream API key could be
// selected for the target cluster (all penalized / weight 0 / dead).
// Distinct from RPM/TPM (client quota) and from upstream 429 (provider).
CodeNoAvailableKey = "NO_AVAILABLE_KEY"
```

映射（`ErrorCodeToStatusCode`）：**已裁定（2026-10-21，owner：zhangmiao）——429 + `rate_limit_error`**（理由见裁定点 1）。

**1.2 耗尽分支改用 AiError 并带 `details.retry_after_seconds`**（`bfe_server/reverseproxy.go:1839-1841`）

```go
log.Logger.Warn("aiClusterInvoke: all ai keys exhausted for cluster[%s], return synthetic 429", attempt.ClusterName)
res = bfe_basic.NewAiErrorWithDetails(bfe_basic.CodeNoAvailableKey, bfe_basic.TypeRateLimitError,
	"no available upstream api key for this cluster",
	&bfe_basic.AiErrorDetail{Model: /* 目标模型，可选 */, RetryAfterSeconds: keyExhaustedRetryAfterSeconds()},
).CreateErrorResponse(basicReq)
```

要点：

- `RetryAfterSeconds` 取"惩罚窗口剩余时长"的**上界估计**：429 惩罚 TTL 为 60s（`reverseproxy.go:1875` 附近 `setKeyPenalty(..., 60, ...)`）。建议把该字面量抽为具名常量（如 `aiKeyPenaltyTTL429 = 60`）供写键与响应共用，避免两处漂移；也可用 `client.TTL(penaltyKey)` 取精确剩余值（需扩展 `redis_client` 接口，成本更高，列为可选）；
- 状态码由 `ErrorCodeToStatusCode` 决定（429 时与现状一致，**降级判定不受影响**）；
- 保留 `, return synthetic 429` 日志后缀（#1440 的排障指纹）。

**1.3（必改，已裁定）：补 HTTP `Retry-After` 头**

`Retry-After` 是标准退避信号，对不支持解析 JSON details 的第三方客户端（现场有 `claude-cli/2.1.304` 等）更有效。已裁定（2026-10-21，owner：zhangmiao）实施，取值与 `details.retry_after_seconds` 一致（惩罚窗口上界，默认 60s）。

实现方式：

- **在调用侧设置**（Key 耗尽分支，`reverseproxy.go:1840` 附近）：`res.Header.Set("Retry-After", strconv.Itoa(n))`。**不改动 `AiError.CreateErrorResponse` 的公共行为**——该构造函数被协议不匹配、鉴权/配额、限流等多处共用，全局加头会改变这些错误码的既有响应形态（超出本卡范围）；
- 或新增 `CreateErrorResponseWithRetryAfter(request, n)` 变体（语义更清晰，但需同步评估其他调用点是否也应带该头）；
- 头格式用秒数（`Retry-After: 60`）；`details.retry_after_seconds` 恒为整数秒，不需支持 HTTP-date 形式。

### 步骤 2（必改）：`ErrCode` 归因写入

在耗尽分支同时写入 `basicReq.ErrCode`（**必须是新哨兵，绝不能是 `ErrAiRateLimit`**）：

```go
// bfe_basic/error_code.go
// AI key exhaustion at the gateway (no usable upstream key). Not the
// local rate-limit sentinel: cluster fallback must still be allowed.
ErrAiNoAvailableKey = errors.New("NO_AVAILABLE_KEY") // no usable ai key
```

```go
// reverseproxy.go 耗尽分支
basicReq.ErrCode = bfe_basic.ErrAiNoAvailableKey
basicReq.ErrMsg  = "no available upstream api key"
```

影响核对（已按 §2.2 逐点验证）：

- `:1336`/`:1874` 用 `== ErrAiRateLimit` 比较 → 新哨兵不命中，**降级与轮转行为不变**；
- `mod_ai_token_auth.go:327` 的 `isClientAbortErr` 只认 `ErrClientWrite/Close/Reset` → **计费路径不变**；
- 新增副作用（预期且合理）：`http_conn.go:613` 的 `ClientReqFail` 计数 +1（该请求确实失败）、access 日志 log tag 与 `err_code` 变为 `NO_AVAILABLE_KEY`；
- `resetRequestForRetry`（`:1986`）会在后续 fallback 尝试前清空 → **成功的降级请求不会带上前一集群的归因**；若最后一次尝试即耗尽（无 fallback），`err_code` 即为本码值——正是本卡要的可观测性。

**不复用 `ai_auth_reject_reason`**（裁定点 2 已确认采用 `err_code`）：其语义为认证/配额拒绝（§5、`ai_error_codes.md:130`），且只在 fallback 成功时清空，写入会污染该字段含义。

### 步骤 3（已裁定范围）：no-cluster / clusterInvoke 失败路径纳入统一错误体

**范围（裁定点 3b，2026-10-21 已确认）**：本卡同批处理"网关侧合成 500"路径；AI 无路由 404 与非 AI 链路不在本卡范围。

1. **纳入本卡（必改）**——`bfe_server/reverseproxy.go` 中经 `CreateInternalSrvErrResp()` 合成空体 500 的网关侧失败路径：
   - findProduct 失败：`:734`（`ServeHTTP`）、`:1191`（`ServeHTTPForAI`）；
   - findCluster / findLocation 失败：`:773`；
   - no cluster：`:789`（`aiClusterInvoke` 的 `ErrBkNoCluster` 出口经 `:1368` 落成 500）；
   - clusterInvoke 返回 `err != nil || res == nil`：`:898`（`ServeHTTP`）、`:1368`（`ServeHTTPForAI`）；
   - 改法：与步骤 1 一致，改由 `bfe_basic.AiError.CreateErrorResponse` 输出结构化错误体（同时获得 `Content-Type: application/json`）。
2. **不纳入本卡**——AI 无路由 404（`:1228-1229`，`CreateSpecifiedContentResp(404, "text/plain", "AI route not found")`）：其响应体形态已被集成测试断言（SC01 TC-12 keep-alive 复用、SC05 TC-19 非 2xx 不输出 token 字段），改形态需同步改测试与产品口径，另开卡收口；
3. 非 AI 链路（`mod_auth_request`、`mod_cors`、`mod_static` 等，见 §2.3 末）不属本卡范围。

**要点与已核对的语义**：

- **归因已就绪**：上述路径中 no-cluster 已写 `basicReq.ErrCode = bfe_basic.ErrBkNoCluster`（`:1753` 附近），findProduct/findCluster 亦写 `ErrBkFindProduct` / `ErrBkFindLocation` ⇒ 访问日志 `err_code` 本就有值，本步骤只补**响应体契约**（现状缺口是空 body + 无 `Content-Type`）；
- **错误码与状态码映射**（子问题 3b-i，见 §四）：no-cluster 与 clusterInvoke 失败统一映射为既有 `BACKEND_UNAVAILABLE`（502，`request_ai_basic.go:407/454`，`internal_error`）；**状态码将由 500 变为 502**，属可观测变化，需在变更说明中提示保留方，并核对既有测试/监控断言；
- **响应构造不得掩盖既有失败语义**：这些路径返回 nil 或本地构造响应，改造时需保证 `basicReq.BfeStatusCode`、`Stat.ResponseStart` 等既有赋值（`:1367` 附近）与 `response_got` 流程不被破坏；
- **与「客户端中断」路径区分**：`err != nil || res == nil` 的大条件同时覆盖客户端中断/写失败（`ErrClientWrite` 等）。改造应只在**网关/后端侧失败**时输出结构化错误体；客户端已断开时响应无处可发（既有行为即为空体 500 落日志），保持现状即可，避免为"发不出去的响应"构造 JSON。

### 步骤 4（文档同步）

- `docs/zh_cn/sys_design/ai_error_codes.md`：§2.4 新增两行（`NO_AVAILABLE_KEY` → **429**、`rate_limit_error`、触发条件：目标集群无可用 Key——全部被惩罚 / `Weight=0` / dead；以及 no-cluster / clusterInvoke 失败 → `BACKEND_UNAVAILABLE` → 502、`internal_error`，从 §4"预留"转为正式返回）；§3.2 的 `details.retry_after_seconds` 从"预留"改为"已在 Key 耗尽路径使用"；§5 说明 `err_code` 亦承载网关侧错误码（不限于 `ai_auth_reject_reason`）；§6 排查建议表补 502 行语义（"后端/目标集群不可用：检查 cluster 配置与后端健康"）；
- 更新本卡对应 modifications 文档的实施记录。

## 四、需 owner 裁定的点

**裁定点 1（`NO_AVAILABLE_KEY` 的状态码与类型）——已裁定（2026-10-21，owner：zhangmiao）：429 + `rate_limit_error`。**

- 理由：① 语义上是"容量/凭证暂时耗尽，稍后重试可能成功"，与客户端退避语义一致；② 429 已在 `aiFallbackStatusCodes` 白名单内，`Retry-After`/`retry_after_seconds` 与之天然配套；③ 避免把"网关自身容量不足"计入 5xx 错误预算，减少 SLO 误判（issue 明确反对"把网关合成错误与上游 5xx 混为一谈"）；④ 与 `#1440` 的合成 429 状态码保持一致，降级契约零变化。
- 被否决的备选：**503 + `internal_error`**（计入网关 5xx、与 #1440"5xx 一律降级"重叠）；**复用 `BACKEND_UNAVAILABLE`(502)**（语义混淆，无法区分"后端不可用"与"凭证耗尽"）；**复用 `UPSTREAM_*` 系列**（v1.8.8 无该目录，且语义为"上游返回的限流"，与本路径"网关自身选不出 Key"不同；dev 分支移植时需注意码值对齐，见 §七）。

**裁定点 2（归因字段落点）——已裁定（2026-10-21，owner：zhangmiao）：采用 `err_code`（步骤 2）**，即写入新哨兵 `bfe_basic.ErrAiNoAvailableKey`，`mod_access_pb3` 的 `err_code` 直接承载 `NO_AVAILABLE_KEY`；**不复用 `ai_auth_reject_reason`**（其语义为认证/配额拒绝，见 §2.2、`ai_error_codes.md:130`，且只在 fallback 成功时清空，写入会污染该字段含义）。被否决的备选：新增专用访问日志字段（如 `ai_gw_error_code`）——需 `bfe-access-pb` schema 变更、成本高，若后续 `err_code` 承载的语义过多可再评估。

**裁定点 3a（HTTP `Retry-After` 头）——已裁定（2026-10-21，owner：zhangmiao）：实施**，见步骤 1.3；取头值与 `details.retry_after_seconds` 同源，且只在 Key 耗尽调用侧设置，不改变 `CreateErrorResponse` 的公共行为。

**裁定点 3b（步骤 3 的范围）——已裁定（2026-10-21，owner：zhangmiao）：接受推荐范围**，即同批处理 no-cluster / clusterInvoke 失败 / findProduct 失败的网关侧合成 500（步骤 3 第 1 条），AI 无路由 404（`:1228`，`text/plain`）与非 AI 链路不在本卡范围（另开卡收口）。

**子问题 3b-i（该范围的错误码与状态码）——推荐并默认执行：统一映射为既有 `BACKEND_UNAVAILABLE`（502，`internal_error`）**：① 零新增码值，语义贴合"后端/路由不可用"；② 与 §4 预留码表的既有规划一致。**需注意的副作用**：这些路径的状态码由 500 变为 502，属可观测变化（监控曲线、SLO 归因、既有测试断言需同步核对）；若 owner 希望保持 500 不变，则需新增如 `GATEWAY_INTERNAL_ERROR`(500) 一类码值——该选择只影响状态码与码值，不影响本步骤的响应体契约改动。

## 五、回归测试

单元测试（`bfe_server`、`bfe_basic`）：

1. 耗尽分支响应断言（扩展既有 `TestAiClusterInvokeExhaustionReturnsSynthetic429`）：`StatusCode == 429`、`Content-Type: application/json`、body 解析后 `error.code == "NO_AVAILABLE_KEY"`、`error.type == "rate_limit_error"`、`error.details.retry_after_seconds == 60`；
2. `basicReq.ErrCode == bfe_basic.ErrAiNoAvailableKey`，且 `!= bfe_basic.ErrAiRateLimit`（回归护栏，防止未来误用哨兵导致降级被抑制）；
3. `shouldTriggerFallback(res, nil, "")` 对该合成响应仍返回 true（降级契约不变）；
4. `ErrorCodeToStatusCode` 映射与 `AiErrorDetail` JSON 序列化（`retry_after_seconds` 字段名）；
5. **`Retry-After` 头（已裁定，必测）**：断言其存在且值等于 `details.retry_after_seconds`（字符串秒数形式）；同时断言其他错误码（如协议不匹配 400、限流 429）的响应**未**被加上该头（回归护栏，确保只改了 Key 耗尽调用侧）；
6. **步骤 3 覆盖路径（已裁定，必测）**：对 no-cluster（如 `cluster_conf` 中不存在的集群名）、clusterInvoke 连接失败等网关侧失败，断言响应为 `BACKEND_UNAVAILABLE`/502 + `Content-Type: application/json` + 结构化 `error.code`/`type`/`message`，且访问日志 `err_code` 仍为既有 `BK_NO_CLUSTER` / `BK_CONNECT_BACKEND` 等（归因未被新响应构造覆盖或丢失）；
7. **客户端中断路径不回归（护栏）**：`ErrClientWrite`/`ErrClientClose`/`ErrClientReset` 场景不得因本改造改变计费旁路（`mod_ai_token_auth` 的 `isClientAbortErr`）与既有空体行为。

集成测试（SC10，复用 #1439 TC-08 的环境）：

8. 扩展现有 TC-08 或新增用例：熔断降级路径下，**无 fallback 配置**时客户端收到 429、`error.code == "NO_AVAILABLE_KEY"`、`Retry-After` 存在；有 fallback 时仍成功降级（不因新增错误体/ErrCode 破坏 #1440 的降级）；
9. 访问日志断言（若该场景有 pb3 日志能力）：`err_code == "NO_AVAILABLE_KEY"`；降级成功时 `err_code` 为空（`resetRequestForRetry` 清理生效）。

回归范围：SC10 全量（含 TC-08）、SC02、SC07、SC08；**以及任何断言本地 500 空体/no-cluster 路径的既有用例（须先扫一遍：SC01 的连接复用类用例、SC05 的访问日志字段用例最容易受影响——`res_status_code` 由 500 变 502）**；`bfe_server` / `bfe_basic` / `mod_ai_route` / `mod_ai_token_auth` 单测。

## 六、修复后目标行为

以 issue 现场路径为例（kimi 唯一可用 Key 被 429 惩罚、无 fallback 配置时）：

1. 惩罚窗口内请求 → 熔断（#1439）→ 合成 429；
2. 响应：`HTTP/1.1 429` + `Content-Type: application/json` + `Retry-After: 60`，body：

```json
{"error":{"code":"NO_AVAILABLE_KEY","type":"rate_limit_error","message":"no available upstream api key for this cluster","param":null,"details":{"model":"yf-coding","retry_after_seconds":60}}}
```

3. 访问日志：`err_code=NO_AVAILABLE_KEY`、`res_body_len>0`、`res_content_type=application/json`；
4. 配置了 fallback 时：仍正常降级（429 → bailian 2xx），且**成功请求的 `err_code` 为空**（不携带前一集群归因）；
5. 客户端（含第三方 CLI）能据 429 + `Retry-After` 退避，抑制 issue 中的重试风暴放大。

步骤 3 覆盖的路径（如路由到不存在的集群、后端连接失败）：

6. 响应：`HTTP/1.1 502` + `Content-Type: application/json` + body `{"error":{"code":"BACKEND_UNAVAILABLE","type":"internal_error","message":"..."}}`（不再是空体 500）；
7. 访问日志：`res_status_code=502`、`res_content_type=application/json`、`res_body_len>0`，`err_code` 保持既有 `BK_NO_CLUSTER` / `BK_CONNECT_BACKEND` 等归因。

## 七、边界与残余风险

- **本卡不做服务端重试抑制**：issue 已明确"对同一 client_key 的短时 5xx 做服务端重试抑制"属独立议题；本卡只交付"正确的错误契约"，客户端的退避行为仍依赖其自身实现（但 429 + `Retry-After` 已给出标准信号）。
- **降级契约不可回归**：任何实现都必须保持"429 触发 cluster fallback"（#1440）与"ErrCode 不得为 `ErrAiRateLimit`"。单测第 2、3 条即为此护栏。
- **`ClientReqFail` 计数变化**：新增 ErrCode 会让该计数器把 Key 耗尽计入"请求失败"（语义上正确，但会改变既有监控曲线，需在变更说明中提示保留方）。
- **code 值需与 dev 分支对齐**：v1.8.9-dev 已有统一目录与 `X-Bfe-Gw-Error` 网关标记（#1440 移植时使用 `CodeUpstreamRateLimited`）；本卡若在 v1.8.8 新增 `NO_AVAILABLE_KEY`，**移植到 dev 时需把该码同步进 dev 的目录**（否则两分支错误码不一致）。
- **`retry_after_seconds` 的精度**：默认取惩罚 TTL 上界（60s），并非真实剩余时间；若产品要求精确值，需扩展 Redis 客户端取 `TTL`（列为可选增强）。
- **§3.1 的覆盖缺口（收窄后）**：本卡落地后，AI 链路的网关侧合成错误仅剩 **AI 无路由 404**（`text/plain`）不合规；非 AI 链路的合成错误（`mod_auth_*`、`mod_cors`、`mod_static` 等）也不在范围内。两者需另开卡收口（无路由 404 的形态变更须同步 SC01 TC-12、SC05 TC-19）。
- **状态码可观测变化（步骤 3 副作用）**：no-cluster / clusterInvoke 失败由 500 改为 502（`BACKEND_UNAVAILABLE`），会改变监控曲线、SLO 归因与既有测试断言；若 owner 要求保持 500，改用 `GATEWAY_INTERNAL_ERROR`(500) 类码值即可（见子问题 3b-i），响应体契约改动不受影响。
- **未验证项**：现场 426 req/min 的重试放大是否由具体客户端退避策略缺失主导——本卡无法证明"给了 `Retry-After` 就消除放大"，但满足契约是消除放大的必要条件。
## 八、实施记录（2026-10-21，v1.8.8 分支）

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | 新增 `CodeNoAvailableKey = "NO_AVAILABLE_KEY"`；`ErrorCodeToStatusCode` 增加映射 → 429（裁定点 1 已裁定：`rate_limit_error`） |
| `bfe_basic/error_code.go` | 新增哨兵 `ErrAiNoAvailableKey = errors.New("NO_AVAILABLE_KEY")`（注释明确：故意不是 `ErrAiRateLimit`，不得抑制 cluster fallback） |
| `bfe_server/reverseproxy.go` | ① 429 惩罚 TTL 抽为常量 `aiKeyPenaltyTTL429 = 60`，写键与 Retry-After 提示同源；② Key 耗尽分支改由 `NewAiErrorWithDetails(CodeNoAvailableKey, TypeRateLimitError, …, &AiErrorDetail{Model: attempt.Model, RetryAfterSeconds: 60})` 构造统一响应，`CreateErrorResponse` 自带 `Content-Type: application/json`；新增 `Retry-After: 60` 头（仅该调用侧设置）；写 `ErrCode = ErrAiNoAvailableKey` / `ErrMsg`（日志归因）；③ `ServeHTTPForAI` 的 findProduct 失败与 `err != nil || res == nil` 网关侧失败出口改为 `BACKEND_UNAVAILABLE`(502) + `internal_error` + 通用 message；新增 `isAIClientAbort` 谓词，客户端中断（`ErrClientWrite/Close/Reset`）保持既有空体 500 记录 |
| `bfe_server/reverseproxy_ai_test.go` | 扩展 `TestAiClusterInvokeExhaustionReturnsSynthetic429`（body 解析断言 `error.code`/`type`/`details.retry_after_seconds`、`Retry-After` 头、ErrCode 为 `ErrAiNoAvailableKey` 且 != `ErrAiRateLimit`）；新增 `TestBackendUnavailableErrorResponseContract`（502 + JSON + 无 `Retry-After` 的回归护栏）与 `TestIsAIClientAbort`（中断谓词边界） |
| `docs/zh_cn/sys_design/ai_error_codes.md` | §2.4 新增两行（`NO_AVAILABLE_KEY`→429；网关侧后端不可用→502 `BACKEND_UNAVAILABLE`）；§3.2 `retry_after_seconds` 从"预留"改为"已在 `NO_AVAILABLE_KEY` 使用并同步输出 `Retry-After`"；§5 增加说明（网关侧错误码由 `err_code` 承载，不写 `ai_auth_reject_reason`）；§6 排查表 502 行细化 |
| `tests/integration/.../scenario-SC02/…sc02_multi_api_key_test.go` + TC-09 设计文档 | 臂 B 扩展三条断言（`error.code=NO_AVAILABLE_KEY`、`Retry-After: 60`、`retry_after_seconds: 60`），两臂仍通过 |

**与本文档的偏差（实施结论，方案节相应处已标注）：**

1. **步骤 3 范围收窄为 AI 路径**：findProduct 失败（`:1191`）与 `err != nil || res == nil`（`:1363` 附近，即 `ServeHTTPForAI` 分支）已改造；**非 AI `ServeHTTP` 的同名 500 分支（`:734`/`:773`/`:789`/`:898`）未改**——AiError 的 OpenAI 兼容 JSON 契约是 AI 数据面专属（`ai_error_codes.md` 语境），套用到非 AI 流量属错误行为；如需统一非 AI 链路，应另定义契约（建议另开卡，避免本卡超范围）。
2. **响应 message 为通用文案**（如 "upstream backend unavailable for this request"），不回填 `ErrMsg`/`err.Error()`：避免内部拓扑（host:port、Redis 地址等）泄漏给外部客户端；排障信息保留在 `bfe.log` 与 `err_code`/`err_msg` 日志字段。
3. **502 路径的端到端集成测试暂缓**：响应契约已由单测覆盖（`TestBackendUnavailableErrorResponseContract`），完整链路断言（目标集群后端不可达 → 502 JSON）需要"关闭后端端口"一类脆弱设施，列为后续增强，不在本卡阻塞。

**验证结果（本次实跑）：**

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、bfe_basic、SC02 测试包） | 通过 |
| 单测：`bfe_basic`（全包）/ `bfe_server` / `mod_ai_route` / `mod_ai_token_auth` | 全部通过 |
| SC02 全量（含 TC-09 扩展，两臂） | 通过 |
| SC10 全量（#1439 熔断路径 + #1440 降级路径不受影响） | 通过 |
| SC01 / SC05 / SC07 / SC08 / SC12 回归 | 全部通过 |

**可观测变化（变更说明用）**：① Key 耗尽响应从"手搓 JSON（无 `code`）"变为统一 AiError（含 `code`/`details`）+ `Retry-After`；② `err_code` 从空变为 `NO_AVAILABLE_KEY`（`ClientReqFail` 计数相应 +1）；③ 网关侧失败（no-cluster/连接失败）从空体 500 变为 502 + JSON（`BfeStatusCode` 同步为 502）。

**遗留事项**：v1.8.9-dev 前向移植时需把 `NO_AVAILABLE_KEY` 同步进 dev 的错误码目录（dev 的 `#1440` 移植用的是 `CodeUpstreamRateLimited`，两分支码值需对齐——建议 dev 侧同样切换为 `NO_AVAILABLE_KEY` 或显式映射，移植时裁定）；非 AI `ServeHTTP` 本地错误与非 AI 模块的 §3.1 合契约化另开卡。
