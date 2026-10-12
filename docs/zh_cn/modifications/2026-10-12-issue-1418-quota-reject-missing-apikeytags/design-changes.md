# Issue #1418 修复方案：配额预检拒绝（429）的访问日志缺失 ai_apikeytags

- Issue: [bfenetworks/bfe#1418](https://github.com/bfenetworks/bfe/issues/1418)（由 [rainway-ai-gateway/ai-gateway-api#238](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/238) 克隆迁移；缺陷与修复均在 BFE 数据面）
- 缺陷：绑定 level-3 Entity 的 API Key 在配额耗尽后再次请求，被配额预扣层拒绝（HTTP 429 `QUOTA_EXHAUSTED`）。该 429 的 B2Log `RequestLog` 带 701（内部 key id）、712（`QUOTA_EXHAUSTED`）、713（1 个拒绝配额计划），但 **702 `ai_apikeytags` 整个字段缺席、102 Cluster 为空**；同一 Key 的成功（200）记录 701/702 齐全。后果：配额拒绝请求在访问日志中无法按 Entity 维度审计/对账。
- 代码库：`bfe/`，**仅针对 `v1.8.9-dev` 分支**（tip `7cf31180`）；正文行号以本分支为准。
- 关联 TC：`integration-test` 仓库 SC3201-TC015（`coordinator_approved`），断言 A-06「429 `RequestLog` 保留与 200 记录一致的 `TagLevel=3` Entity 标签」当前 FAILED_PRODUCT。
- 现场证据要点：同一日志窗口内 9/9 个同时存在 200 与 429 记录的 key 均为「200 带 702、429 不带」；唯一带 702 的 429 属于限流层（§2.2，712 空、713 计数 0、Cluster 非空）——排除「标签获取失败」「产品对任何 429 都不写 702」两类解释。

## 一、口径定义（Expected 的来源）

| 条款 | 内容 |
|---|---|
| `ai_access_log_fields.md:26-27` | 701 `ai_apikey_id`（内部 key id）与 702 `ai_apikeytags`（API Key 关联的 Entity 层级标签）的生产者同为 `mod_ai_token_auth` |
| `ai_access_log_fields.md:239`（§5.2） | 「在 `ValidateUserTokenByReq()` 中找到 Token 后，**立即**把 `Token.KeyId` 写入 `AiBasicInfo.ClientKeyId`，**确保即使后续拒绝也能在日志中识别 key**」 |
| `ai_access_log_fields.md:240`（§5.2） | 「通过 `SetTokenAuthContext()` 写入 `ApikeyTags`」 |
| `ai_error_codes.md:41-49`（§2.3 配额扣减层） | `QUOTA_EXHAUSTED`（429）「由 `mod_ai_token_auth` 在请求进入时进行预扣配额校验返回」 |
| `ai_error_codes.md:149-166`（§5） | 712 `ai_auth_reject_reason` / 713 `ai_auth_reject_quota_plans` 为拒绝日志字段 |
| `rmb_quota.md:354-361`（§7.1） | 认证阶段遍历 `token.QuotaPlans` 调 `plan.HasBalance()` 做余额预检，「若余额为 0 则拒绝」 |
| `rmb_quota.md:367,380-395`（§7.2） | `SetTokenAuthContext()` 执行 `aiBasicInfo.ApikeyTags = tags`，写入发生在**认证阶段** |

日志边界（本仓库 `mod_access_pb3/request_log.go`）：701 仅当 `ClientKeyId != ""` 时写（`:400-401`）；702 仅当 `len(ApikeyTags) > 0` 时写（`:405-406`）；712/713 来自 `AiAuthInfo`（`:528-532`）。

**Expected（issue A-06）**：429 `QUOTA_EXHAUSTED` 记录须带 701（与同一 Key 的 200 记录同内部 key id）与 702（同一组含 `TagLevel=3` 的 Entity 标签），外加 712/713。即「701+702 身份/审计块在拒绝后仍保留」是已批准设计的意图（`:239` 的“即使后续拒绝”），702 不允许只存在于成功路径。

## 二、根因

按调用顺序逐环（行号均为 `v1.8.9-dev` @ `7cf31180`）：

1. **入口**：`tokenFoundProductHandler`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:552`）在 `:566` 调 `ValidateUserTokenByReq`；一旦返回 error，`:567-571` 直接 `BfeHandlerResponse` 返回错误响应（状态码经 `ErrorCodeToStatusCode`：`CodeQuotaExhausted → 429`，`bfe_basic/request_ai_basic.go:521,535`；响应体 `CreateErrorResponse` `:712-714`）。请求到此结束，**不再经过任何后续 handler**。
2. **701 为什么有**：`ValidateUserTokenByReq`（`bfe_modules/mod_ai_token_auth/token_rule_table.go:98`）在 token 命中后立即写身份（`:120-124`，注释自述意图「record key_id … as early as possible so that access log can still identify the token even if the request is later rejected」）：

   ```go
   120: // record key_id into AiBasicInfo as early as possible so that access log
   121: // can still identify the token even if the request is later rejected.
   122: if aiBasicInfo := req.GetAiBasicInfo(); aiBasicInfo != nil {
   123:     aiBasicInfo.ClientKeyId = token.KeyId
   124: }
   ```

3. **702 为什么无**：`ApikeyTags` 在**全仓库的唯一写入点**是 `SetTokenAuthContext`（`mod_ai_token_auth.go:738-760`）：

   ```go
   738: func SetTokenAuthContext(req *bfe_basic.Request, tok *Token, promptToken int64, tags []bfe_basic.ApikeyTag) {
   741:     aiBasicInfo.ClientKeyId = tok.KeyId
   ...
   751:     aiBasicInfo.ApikeyTags = tags
   ```

   而它只在**成功路径**被调用（`mod_ai_token_auth.go:577`）。已查到 token 之后的全部拒绝 return 都发生在 `:577` 之前，702 永不写入，日志边界 `len>0` 判断使其字段缺席。
4. **受影响拒绝路径（6 条，同形状缺 702）**：`KEY_DISABLED`（`token_rule_table.go:126-133`）、`KEY_EXPIRED`（`:135-142`）、`QUOTA_EXPIRED`（`:150-160`）、`INTERNAL_QUOTA_ERROR`（`:161-170`）、`QUOTA_EXHAUSTED`（`:171-180`，issue 现场）、`SUBNET_NOT_ALLOWED`（`:194-213`）。这些路径的 712/713 由 `SetAiAuthInfo`（`:90-96`）在拒绝时写入，所以 429 记录「701/712/713 齐、702 缺」——与实测形状完全一致。
5. **反向对照（机制可用的证明）**：限流层 429（`ai_error_codes.md` §2.2）在 `HandleFoundProduct` 回调链上位于 `mod_ai_token_auth` 之后，走到它时认证已成功、`SetTokenAuthContext` 已执行，故 702 保留、Cluster 非空。说明写 702 的机制存在且可用，缺失只发生在认证/配额的提前返回分支——与 TC spec 自述 `primary_risk`「配额检查在设置审计上下文前提前返回」一致。
6. **102 Cluster 为空是固有形态，不在修复范围**：`request_log.go:257` 的 `Cluster` 取自 `req.Backend.ClusterName`，该字段仅在转发阶段赋值（`bfe_server/reverseproxy.go:793`、`:1732`）。认证阶段拒绝的请求从未进入转发，Cluster 恒为空；§2.3 条款未要求该阶段填写 Cluster，拒绝请求的审计归属由 701/702 承担。

> 排除项：部署滞后解释不成立——`SetTokenAuthContext` 写 tags 与 `ClientKeyId` 早写的代码在 `v1.8.9-dev` 与 `v1.8.8` 上同构存在（`git show v1.8.8:...token_rule_table.go` 已核对），属代码缺陷而非设计条款未落地。

## 三、修复步骤

### 步骤 1（必改，核心，一处改动）：身份块早写时同时写入 ApikeyTags

`bfe_modules/mod_ai_token_auth/token_rule_table.go:120-124`，把早写身份块扩展为同时写 `ApikeyTags`：

```go
// record key_id and entity tags into AiBasicInfo as early as possible, so
// that the access log can still identify the key and its Entity attribution
// even if the request is later rejected (issue #1418).
if aiBasicInfo := req.GetAiBasicInfo(); aiBasicInfo != nil {
    aiBasicInfo.ClientKeyId = token.KeyId
    aiBasicInfo.ApikeyTags = token.Tags
}
```

要点：

- **一处改动覆盖全部 6 条已查 token 的拒绝路径**，且对未来新增的拒绝路径天然免疫（写在查找点，而非各 return 分支）；
- 成功路径 `SetTokenAuthContext` 写入的是同一组值（`ClientKeyId=tok.KeyId`、`ApikeyTags=tok.Tags`，`mod_ai_token_auth.go:741,751`），幂等，**成功日志行为零变化**；
- `NO_API_KEY`（`:102`）/`INVALID_REQUEST`（`:107`）/`INVALID_API_KEY`（`:114`）：token 未查到，无 tags 可写，保持无 701/702——未知 key 本无 Entity 归属，且符合访问日志凭据脱敏设计（`ai_access_log_fields.md` §6.1：未认证请求一律置空）；
- `token.Tags` 来自 mod-api-key 配置导出（`token.go:43`，加载时 `tokenFile.Tags` 拷贝 `:242`），载荷为 Entity type/name/level，即 702 的设计载荷，无新增敏感信息。

**替代方案及不采纳理由**：

| 替代方案 | 不采纳理由 |
|---|---|
| 在 `tokenFoundProductHandler` 错误分支（`mod_ai_token_auth.go:567-571`）补写 tags | 出错时 `ValidateUserTokenByReq` 返回 `nil, err`，handler 拿不到 token；需改返回签名或二次查询，侵入大 |
| 提前/无条件调用 `SetTokenAuthContext`（把调用移到预检之前） | 它会顺带创建 `TokenAuthContext`（`REQ_TOKEN_AUTH_CONTEXT`）并播种 `PromptTokens`/`CompletionTokens` 哨兵。为拒绝请求建立完整鉴权上下文改变请求生命周期语义（`GetTokenAuthContext` 的消费者、`tokenRequestFinishHandler` 对非 2xx 的隐含依赖），把「补一个日志字段」变成「改拒绝路径状态机」，风险收益不成立 |
| 在每条拒绝 return 前分别写 tags（6 处） | 重复且易漏，新增拒绝路径会再犯同类缺陷；查找点统一写才是与 `:239`「找到 Token 后**立即**」语义对齐的写法 |

### 步骤 2（单元测试）

`bfe_modules/mod_ai_token_auth/token_rule_table_test.go` 新增（复用既有 helper `newWhitelistTestModule` / `newWhitelistTestToken` / `newWhitelistTestRequest`，见 `:28-58`）：

1. `TestValidateUserTokenByReqRejectionKeepsIdentity`：构造带 `Tags: []bfe_basic.ApikeyTag{{TagName:"entity.type", TagValue:"entity.xxx", TagLevel:3}}` 的 token，分别走 `KEY_DISABLED` / `KEY_EXPIRED` / `SUBNET_NOT_ALLOWED`（client addr 不在白名单）拒绝路径，断言 `req.GetAiBasicInfo().ClientKeyId == "key-1"`、`ApikeyTags` 与 `token.Tags` 相等、`AiAuthInfo.RejectReason` 为相应错误码；
2. 配额类拒绝：仿 `mod_ai_token_auth_test.go:1910`（`TestQuotaPlanHasBalance`）的 Redis mock 模式，构造 `quota=0` 的非 unlimited `QuotaPlan` → 断言 `QUOTA_EXHAUSTED` 路径同样保留 `ClientKeyId`/`ApikeyTags`，且 `RejectQuotaPlans == [plan.Id]`；
3. 负面对照：`INVALID_API_KEY` → 断言 701/702 均未写入（固化「未知 key 不写身份块」的语义）。

### 步骤 3（集成测试）

`tests/integration/implementation/scenario-SC05-access-log-ai-fields/` 新增 **TC-20「配额拒绝请求保留 ai_apikeytags」**（仿 TC-01 的成功路径 tags 断言 `:532` + `:814` 的 `assertApikeyTags`）：

- **臂 A（issue 现场复现）**：带 Entity tags（`TagLevel=3`）+ finite `QuotaPlan`（`quota=0`）的 API Key 发请求 → BFE 本地 429 `QUOTA_EXHAUSTED`，受控后端 0 命中；解码 b2log 断言：701 = 内部 key id、702 含与成功记录同组的 `TagLevel=3` 标签、712 = `QUOTA_EXHAUSTED`、713 计数 1；
- **臂 B（对照臂）**：同 Key 配额恢复后发请求 → 200、后端 1 命中；断言 200 行 702 与臂 A 的 429 行 702 一致（同 key 同标签）；
- 同步新增测试设计文档 `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/TC-20-配额拒绝请求保留ai_apikeytags.md`，并在场景说明/总体说明中登记；
- **验收判据**：issue 给定的可断言判据为「429 记录出现 702 即修复成立」；修复合入后复跑 E2E SC3201-TC015，A-06 应 PASSED。

### 步骤 4（文档同步）

- `docs/zh_cn/sys_design/ai_access_log_fields.md` §5.2 首条（`:239`）改为：「在 `ValidateUserTokenByReq()` 中找到 Token 后，立即把 `Token.KeyId` 与 `Token.Tags` 写入 `AiBasicInfo.ClientKeyId` / `ApikeyTags`，确保即使后续拒绝也能在日志中识别 key 并保留 Entity 归属（issue #1418）」；
- `docs/zh_cn/sys_design/rmb_quota.md` §7.1 末尾补一句：「余额预检等拒绝发生在审计身份块写入之后，配额类拒绝日志仍携带 701/702」。

## 四、修复后目标行为（429 `QUOTA_EXHAUSTED` 走查）

1. `HandleFoundProduct`：`mod_ai_token_auth` 命中 token 规则，`ValidateUserTokenByReq` 查到 token → 立即写入 `ClientKeyId` + `ApikeyTags`（token.Tags，含 `TagLevel=3`）；
2. 配额预检 `HasBalance()` 返回余额 0 → `SetAiAuthInfo` 写 712/713 → 返回 429 `QUOTA_EXHAUSTED` 响应（`ErrorCodeToStatusCode`），请求结束；
3. 访问日志：151=429、**701=内部 key id、702=同一组 `TagLevel=3` 标签**、712=`QUOTA_EXHAUSTED`、713=[plan.Id]；102 Cluster 仍为空（固有）；
4. 同一 Key 的 200 记录与 429 记录身份块一致，可按 Entity 维度聚合计量拒绝量；
5. 计费、限流、转发行为不变（见 §五）。

## 五、边界与残余风险

- **102 Cluster 保持为空**：认证阶段拒绝的请求从未进入转发（`req.Backend.ClusterName` 仅在 `reverseproxy.go:793/:1732` 赋值），属固有形态，产品条款未要求，不在本修复范围；
- **未命中 key 的拒绝行（401 等）仍无 701/702**：token 未查到，无身份可写，符合 §6.1 脱敏设计；
- **行为零变化面**：`SetTokenAuthContext` 不动（成功路径语义不变）；新增写入仅为共享 `AiBasicInfo` 一次字段赋值，不创建 `TokenAuthContext`、不播种 token 用量、不影响计费/限流/错误响应结构/模块注册顺序；
- **范围限定 v1.8.9-dev**：本方案只针对 `v1.8.9-dev` 分支实施与验证；部署镜像当前为 `ghcr.io/rainway-ai-gateway/bfe:v1.8.8`（`ai-gateway/VERSIONS.yaml`），`v1.8.8` 分支存在同构缺陷，是否修补/回移由发布节奏另行决策，不在本方案范围；
- 风险评级：极低（单点只读式赋值，成功路径幂等覆盖）。

## 六、验证清单

```bash
cd bfe
go build ./... && go vet ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/
go test -count=1 ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/
go test -count=1 ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/
```

通过标准：

1. 新增单测（步骤 2）全部通过，既有 `token_rule_table_test.go` / `mod_ai_token_auth_test.go` 全量通过；
2. SC05 集成全量通过（含新增 TC-20 两臂）；
3. issue 判据：同一 key 的 429 记录出现 702（含 `TagLevel=3`）；
4. E2E SC3201-TC015 复跑 A-06 PASSED。

## 实施记录（2026-10-12，v1.8.9-dev 分支）

| 文件 | 改动 |
| --- | --- |
| `bfe_modules/mod_ai_token_auth/token_rule_table.go` | `ValidateUserTokenByReq` 早写身份块同时写入 `ApikeyTags = token.Tags`（`:120-126`，注释标注 issue #1418） |
| `bfe_modules/mod_ai_token_auth/token_rule_table_test.go` | 新增 `TestValidateUserTokenByReqRejectionKeepsIdentity`（key disabled / key expired / subnet not allowed / quota exhausted / invalid api key 负面对照，共 5 个子用例）；新增 `net` 导入 |
| `tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增 `TestTC20_QuotaRejectKeepsApikeyTags`（429 配额拒绝臂 + 200 对照臂）；`assertApikeyTags` 增加 `taglevel` 比较；共享 Token 标签补 `TagLevel: 3`（TC-01 期望同步更新） |
| `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/TC-20-配额拒绝请求保留ai_apikeytags.md` | 新增 TC-20 设计文档 |
| `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/场景说明.md` | TC 列表新增 TC-20；§5.1 记录共享 Token 标签（含 `TagLevel: 3`） |
| `tests/integration/测试设计文档/测试场景总体说明.md` | SC05 TC 数 19 → 20，TC 列表与关联方案登记 TC-20 |
| `docs/zh_cn/sys_design/ai_access_log_fields.md` | §5.2 首条更新为「找到 Token 后立即写入 `ClientKeyId` / `ApikeyTags`」（issue #1418） |
| `docs/zh_cn/sys_design/rmb_quota.md` | §7.1 补充「余额预检拒绝发生在审计身份块写入之后，配额类拒绝日志仍携带 701/702」 |

### 验证结果（本机实跑）

| 范围 | 结果 |
| --- | --- |
| `go build ./...`、`go vet`（受影响包）、`gofmt`（改动文件） | 通过 / 干净（`conf_mod_ai_token_auth.go` 为分支既有 gofmt 偏差，与本改动无关，未触碰） |
| `mod_ai_token_auth` / `mod_access_pb3` 全量单测 | 通过 |
| SC05 集成全量（20 TC，含新增 TC-20） | 通过 |
| SC02（多 API-Key）/ SC03（RMB 配额）/ SC11（token auth 计费）回归 | 均通过 |
| TC-20 修复前对照（stash 核心改动重跑） | 按预期失败：`ai_apikeytags length = 0, want 1`，与 issue 实测形状一致；恢复改动后通过 |

### 未验证项

- E2E SC3201-TC015（integration-test 仓库）依赖 control-plane 全栈，未在本仓库复跑；按验收判据「429 记录出现 702 即修复成立」，修复发布数据面后由 E2E 闭环（A-06 应 PASSED）。
