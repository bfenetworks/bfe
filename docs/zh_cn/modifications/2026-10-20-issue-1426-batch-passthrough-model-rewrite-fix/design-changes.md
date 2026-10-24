# 批量透传模式跳过请求体 model 改写修复（/v1/files multipart 体被替换）

日期：2026-10-20

状态：**代码与测试已实现并验证（2026-10-20，见 §七实施记录）**。本文档是 BFE 数据面对 [bfenetworks/bfe#1426](https://github.com/bfenetworks/bfe/issues/1426) 修复的完整说明（根因核实／改动点／测试／边界）。

- Issue：[bfenetworks/bfe#1426](https://github.com/bfenetworks/bfe/issues/1426)
- 代码库：`bfe/`（数据面）
- 关联：`2026-10-07-ai-batch-support`（批量一期实现，本文档修正其透传语义缺口）、`2026-10-19-issue-254-ai-batch-task-state-and-settle-fix`（同链路批量状态/结算修复）
- 行号基于当前工作区（2026-10-20 逐条核实，与 issue 中 HEAD `d444b096` 的行号略有偏移，已按现状更新）

---

## 一、缺陷摘要

AI 路由 target model 非空时，经 BFE 转发 `POST /v1/files`（`Content-Type: multipart/form-data`），数据面的 model 改写逻辑把**整个 multipart 请求体替换为 `{"model":"<target-model>"}`**，只保留原 Content-Type、并把 Content-Length 置为 `-1`。provider 收到 multipart 头 + JSON 体的畸形请求，确定性返回 `400 multipart: NextPart: EOF`，上传与后续 batch 创建链路全部不可用。

同一链路的 `POST /v1/batches`（合法 JSON 体）也会被 `sjson` 注入额外的 `model` 字段——属同一越界改写面：即使 provider 宽容接受，也形成"客户端与服务端看到的请求体不一致"的隐性数据完整性问题。

产品条款（Oracle）：provider 原生 OpenAI Batch API（`/v1/files` + `/v1/batches`）经网关**透传**（`ai-gateway-api` 设计 `batches.md#§1`）——网关可在旁路做限流/预留/记账，但不得改写请求体语义。

## 二、根因（当前工作区逐条核实）

调用链（按顺序）：

1. `bfe_server/http_conn.go:523-528`（`extractClientModel`）：`condition.ReqBodyJsonFetch(request, "model", nil)` 从请求体取 JSON `model`；multipart 体不是 JSON，取不到；回退 `ExtractModelFromPath`，`/v1/files` 路径不含 model → `model=""`。（access log `ai_requested_model` 空，实锤）
2. `bfe_server/http_conn.go:566-570`：写入 `aiMeta.ClientModel=""`、`aiMeta.TargetModel=""`；`DetectModeFromPath` 将 `/v1/files` 判为 `ModeFile`。（access log `mode=file`，实锤）
3. `bfe_server/reverseproxy.go:1308`：`bodyModel = aiMeta.ClientModel` → `""`，作为"请求体当前 model"的缓存。
4. `bfe_server/reverseproxy.go:1656`（`computeTargetModel`，定义于 `:1533-1556`）：AI route target model（`attempt.Model`）非空时直接覆盖 → `targetModel="controlled-model-a"`，并在 `:1659` 记入 `aiMeta.TargetModel`。（access log `ai_target_model`，实锤）
5. **`bfe_server/reverseproxy.go:1686-1703`（缺陷点）**：`targetModel != bodyModel` 成立，调用 `condition.ReqBodyJsonSet(basicReq, "model", targetModel)`；成功后把 `outreq` 与 `basicReq.HttpRequest` 的 `ContentLength` 置 `-1` 并删除 `Content-Length` 头。**该判定没有检查请求是否为 batch/file 透传模式，也没有检查 Content-Type。**
6. `bfe_basic/condition/primitive.go:1271-1295`（`ReqBodyJsonSet`）：不校验 Content-Type，直接 `sjson.SetBytes(body, "model", value)`。multipart 字节不是合法 JSON，但 sjson 不报错，返回只含 `{"model":"controlled-model-a"}` 的新体；`bodyAccessor.SetBytes` 覆盖原体。
7. 结果：provider 收到 `Content-Type: multipart/form-data` + 单行 JSON 体 → `400 multipart: NextPart: EOF`（issue 直连重放同型 body 复现同型错误；合法 multipart 同夹具返回 200，排除后端问题）。

注意：`reverseproxy.go` 已有透传保护先例——`isBatchPassthroughMode(aiMeta)`（`:2029-2037`，`ModeFile || ModeBatch`）已用于禁用 cluster fallback（`:1292`）、key-level retry（`:1805`）、batch hint 推导（`:1834`）与 404 绑定清理（`:1944`），注释明确"批量流量的大文件体必须原样单次转发"。唯独 model 改写块没有接入同一判定，属遗漏而非设计分歧。`maybeInjectStreamUsage`（`:1563-1598`）也有 mode 守卫（仅 chat/completion，`:1573`），同样不受本缺陷影响。

## 三、修复步骤

设计原则：**透传模式一律不改写请求体**——原始字节与 Content-Length 原样送达 provider；`aiMeta.TargetModel` 的计算与记录（`:1656-1659`）保持不变，访问日志、key allow/block 校验、限流等旁路逻辑不受影响。

### 步骤 1（必改）：改写块接入透传模式保护

文件：`bfe_server/reverseproxy.go`（`doSingleAIForward`，`:1686-1703`）。

将

```go
newBodyModel = bodyModel
if targetModel != bodyModel {
    if err := condition.ReqBodyJsonSet(basicReq, "model", targetModel); err != nil {
```

改为

```go
newBodyModel = bodyModel
// issue #1426: batch/file traffic is provider-native passthrough. Its body
// (multipart upload, batch create JSON) must reach the backend byte-for-byte;
// never inject or overwrite the model field, mirroring isBatchPassthroughMode
// guards on fallback/retry above.
if targetModel != bodyModel && !isBatchPassthroughMode(aiMeta) {
    if err := condition.ReqBodyJsonSet(basicReq, "model", targetModel); err != nil {
```

跳过时 `newBodyModel` 保持 `bodyModel`，fallback 后续 attempt 也不会重复触发；`:1656-1659` 的 `computeTargetModel`/`aiMeta.TargetModel` 记录**保留在守卫之外**，access log 的 `ai_target_model` 字段与现状一致。

### 步骤 2（必改）：非 JSON Content-Type 兜底拒绝改写

同一位置再加一层防御：改写前检查请求 Content-Type，显式非 JSON（如 `multipart/form-data`、`text/`、`application/octet-stream`）时跳过改写。这是独立于 mode 判定的兜底——即便未来新增透传类 mode 而漏改 `isBatchPassthroughMode`，multipart 体也不会再被替换。

实现建议：在 `reverseproxy.go` 新增小helper（如 `isNonJsonContentType(h bfe_http.Header) bool`），以 `mime.ParseMediaType` 解析 `Content-Type`：解析失败或媒体类型不是 `application/json`、`*/*+json` 系列时视为非 JSON；头缺失时**不**拦截（部分客户端发 JSON 体但不带头，现有 chat 改写兼容此情况，保持现状）。

守卫合成：

```go
if targetModel != bodyModel && !isBatchPassthroughMode(aiMeta) && !isNonJsonContentType(outreq.Header) {
```

不改 `condition.ReqBodyJsonSet` 本体：该 primitive 同时被 mod_rewrite 等模块的条件动作复用，在那里"按字节替换"是合法语义，改动其行为属于另一合约问题，超出本修复范围。

### 步骤 3（同步）：文档注释

- `isBatchPassthroughMode` 注释（`:2029-2035`）补充"use"：`doSingleAIForward` 的 model 改写跳过。
- `2026-10-07-ai-batch-support/design-changes.md` 实施记录追加一条：透传语义修正指向本文档，避免两份文档表述冲突。

## 四、回归测试

单测（`testing` + `testify`，落在 `bfe_server/reverseproxy_ai_test.go`）：

1. **mode=file 不改写**：构造 `AiBasicInfo{Mode: ModeFile}` + `OutRequest`（multipart 体、带 `Content-Type: multipart/form-data; boundary=...`、明确 `Content-Length`），调用改写判定路径（将判定抽为纯函数 `shouldRewriteBodyModel(aiMeta, header, targetModel, bodyModel) bool` 或直接测 `doSingleAIForward` 的改写段——优先抽纯函数，与 `2026-10-19-issue-254` 的可测性改造同法），断言：body 字节不变、`Content-Length` 保留、`newBodyModel == bodyModel`。
2. **mode=batch 不注入**：`ModeBatch` + 合法 JSON 体（无 `model` 字段），断言改写后 body 仍无 `model` 字段（sjson 注入被跳过）。
3. **multipart + mode=chat 也不改写**（Content-Type 兜底）：即使 mode 不是 file/batch，multipart Content-Type 也拒绝改写。
4. **chat 覆盖回归**：`ModeChat` + JSON 体 + target model 非空，断言 `model` 被写入（既有语义不受影响）；无 Content-Type 头的 JSON 体也仍被改写。
5. **`isNonJsonContentType` 边界**：`application/json`、`application/json; charset=utf-8`、`*/*+json` → 可改写；`multipart/form-data`、`text/plain`、非法值 → 拒绝；头缺失 → 可改写。

集成/E2E：

6. 修复部署后复跑 `integration-test` 仓库 `SC1401-TC026`（issue 的失败用例），`upload-create` 必须 `PASSED`；provider observation 的 `request_body_sha256` 与客户端 multipart 原始体一致。
7. 手工 A/B 复核（issue 已给出方法）：同一 key 给 `/v1/files` 配非空 target model 后发 multipart 上传，应返回 `200/201` 并可回读 file id；取消 route rule 后行为不变。
8. 回归 `/v1/chat/completions` target model 覆盖仍生效（access log `ai_target_model` 与请求体 `model` 字段一致）。

验证命令：

```bash
cd bfe
go test ./bfe_server/ ./bfe_basic/
make test   # 全仓单测 + vet
```

## 五、修复后目标行为

- `POST /v1/files`（mode=file，target model 非空）：multipart 原始字节、boundary、part 头与 Content-Length 原样到达 provider；响应与 file id 正常返回客户端；access log 仍为 `mode=file`、`ai_requested_model` 空、`ai_target_model=<target>`（旁路记录不变）。
- `POST /v1/batches`（mode=batch）：创建 JSON 原样透传，不再被注入 `model` 字段；限流/预留/记账旁路不受影响。
- `/v1/chat/completions` 等 chat 模式：target model 覆盖、fallback 重算、`stream_options.include_usage` 注入（`:1710`）全部保持原样。

## 六、边界与残余风险

- **fallback/key-retry 与改写跳过的交互**：透传模式本就禁用 fallback 与 key retry（`:1292`、`:1805`），跳过改写不会改变重试路径；chat 模式 fallback 重算 target model 后仍正常改写。
- **`aiMeta.TargetModel` 与实际请求体不再一致**（透传模式）：这是设计意图——TargetModel 是路由/计费/日志的路由决策记录，不代表请求体内容；access log 已有 `mode` 字段可区分，消费方不应假设 TargetModel 一定写入了请求体。
- **`isNonJsonContentType` 误拒风险**：以"显式非 JSON 才拒绝、头缺失不拒"为准，避免误伤不带 Content-Type 的 JSON 客户端；`*/*+json` 后缀形态纳入可改写集合。
- **provider 依赖 body model 的少数场景**：若某 provider 的 `/v1/batches` 实现要求 body 必须带 `model`，透传语义下由客户端自行携带（客户端本就应带）；网关不替客户端伪造字段。
- **不改对外语义**：无新增配置项、无模块顺序调整、无计费/限流/路径改写行为变化；仅修复透传模式下本不应发生的请求体改写。
- **残余观察项**：issue 提到的 `/v1/batches` 被注入 `model` 后 provider 是否拒绝，原环境未走到该步骤；本修复从根上消除注入，无需再验证 provider 宽容度。

## 七、实施记录（2026-10-20）

按 `bfe-rd-workflow` Phase 1–7 落地，代码与测试与 §三/§四 方案一致（Phase 2 无配置变更，跳过）。

### 7.1 改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | 新增 `shouldRewriteBodyModel()`（透传模式跳过 + 非 JSON Content-Type 兜底，判定抽为纯函数便于单测）与 `isNonJsonContentType()`（`mime.ParseMediaType`，`*/*+json` 与 `application/json` 放行，头缺失/非法不误拒）；`doSingleAIForward` 改写块改经该判定；`isBatchPassthroughMode` 注释补充 model 改写跳过语义 |
| `bfe_server/reverseproxy_ai_test.go` | 新增 `TestShouldRewriteBodyModel` 真值表 13 例（chat 改写回归、file/batch 透传跳过、Content-Type 兜底、`*+json`/头缺失/非法边界） |
| `docs/zh_cn/sys_design/ai_batch.md` | 新增 §8"请求/响应体透传完整性（数据面不改写）"与变更记录 |
| `tests/integration/测试设计文档/scenario-SC28-批量任务支持/TC-16-透传模式请求体不改写.md`（新建） | TC-16 设计文档；`场景说明.md` 与 `测试场景总体说明.md` 登记 |
| `tests/integration/implementation/scenario-SC28-ai-batch-support/sc28_ai_batch_test.go` | mock provider 增加 `/v1/chat/completions` 回显端点与逐请求 Content-Type/Content-Length 记录；`withRouteTargetModel` 启动选项（构建后 conf 内改 ai_route.data，不污染共享 testdata）；`batchLogEntry` 补 target/requested model；新增 `TestTC16_PassthroughBodyNotRewritten` |

### 7.2 验证结果

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、SC28） | 通过 |
| 单测 `bfe_server`（含 `TestShouldRewriteBodyModel`）/ `bfe_basic` / `mod_ai_batch` / `mod_ai_token_auth` / `mod_ai_route` | 全部通过 |
| SC28 `TestTC16_PassthroughBodyNotRewritten` | 通过（修复后）；**反向验证**：临时回退两处守卫后同一用例确定性 FAIL（provider 收到改写体、multipart 解析失败），证明用例能检出本缺陷 |
| 集成回归 SC28 全量（TC-01…TC-16） | 通过（22.6s）；另回归 SC02 多 API-Key（25.9s）、SC07 Redis 限流（6.5s）通过 |

### 7.3 未做与待办

- Git 提交：**未执行**，按流程需人工确认提交内容后另授权。
- 修复部署后需在 `integration-test` 仓库复跑 `SC1401-TC026`（issue 的原始失败用例）做最终确认，并复核 issue 中的手工 A/B。
