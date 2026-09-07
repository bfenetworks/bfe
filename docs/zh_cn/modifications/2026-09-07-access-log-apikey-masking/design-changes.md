# BFE 访问日志 API Key 脱敏闸口设计变更（修复访问日志明文记录原始 Key）

## 1. 背景

缺陷报告：[bfenetworks/bfe#1357](https://github.com/bfenetworks/bfe/issues/1357)（E2E 用例 SC3201-TC011，终态 FAILED_PRODUCT）。

线上 BFE v1.8.7 输出的 pb_access3.log（`mod_access_pb3` B2Log RequestLog）中，consumer API Key 原文（`#AI_product-<随机串>` 格式）仍出现在 3 类字段：

| 字段 | 含义 | 泄漏内容 |
|------|------|----------|
| 49 `authorization` | 通用认证头字段 | Authorization 头原值，每条记录必有 |
| 801 `ai_route_rule_hits[].rule_owner` | 路由命中规则 owner | apikey 型规则的 owner 就是原始 Key（owner_type=apikey） |
| 841 `ai_auth_hit_quota_plans[]` | 命中的配额 Plan ID | Plan Id 本身由原始 Key 构成 |

伴随特征：

1. 鉴权失败的记录（INVALID_API_KEY）同样落盘提交的 Key 原文——暴破探测的 Key 也会被收集；
2. 跨 run 多把 Key 同窗可见；
3. 字段 701 `ai_apikey_id` 本身符合 2026-08-19 改造后的语义（= key_id），泄漏全部发生在改造未覆盖的其他字段。

**根因**：2026-08-19 变更 `f26f418`（见 `modifications/2026-08-19-update-ai-access-log-fields`）只把字段 701 的语义从原始 Key 改为内部 key_id，但原始 Key 在系统中同时扮演三个角色，后两个角色使它必然从结构化字段流入日志：

- **认证头通道**：通用字段 49 `authorization` 的设计就是记录认证头，AI 字段脱敏改造从未覆盖通用字段，pb3 组装层也没有统一的凭据脱敏闸口；
- **路由 join key 通道**：ai-route 导出格式中，apikey 型路由规则的 owner 就是原始 Key——客户端只出示原始 Key，路由匹配以它为 join key 是必然的；命中证据（801）随后原样带出；
- **配额标识通道**：quota plan 的 Id 由网关导出侧用原始 Key 构成，`mod_ai_token_auth/token_rule_table.go:186` 将 `plan.Id` 原样 append 进 841。

线上 v1.8.7 已包含 `f26f418` 仍然泄漏，排除版本滞后：这是脱敏改造的范围缺失（只改了 701 单点，未处理"原始 Key 作为标识符"的根因），属真实产品缺陷而非部署问题。

> 该行为同时违反 `docs/zh_cn/sys_design/ai_access_log_fields.md` §1.2 目标 2 与 §6 安全与合规第 1 条（"访问日志中不再记录原始 API Key 值"、"ClientApiKey…不会写入访问日志"）。

---

## 2. 目标

1. **止血（本变更，BFE 侧最小改动）**：在 `mod_access_pb3` 组装 RequestLog 输出前增加统一凭据脱敏闸口，保证任何字段都不再出现本请求的原始 API Key：
   - 字段 49 `authorization` 直接置空——访问日志中认证头原文没有保留价值；
   - 字段 801 `rule_owner`、841 `plan.Id` 等结构化字段中的原始 Key 字节级替换为对应 `key_id`（认证后 `ClientKeyId` 已知）；
   - 鉴权失败场景（无 `ClientKeyId`）下，含原始 Key 的字段一律置空/脱敏，禁止落盘暴破 Key。
2. **回归防护**：BFE 集成测试 SC05 增加全字段 raw-Key 字节扫描断言（现有仅校验 701=key_id）；线上由 E2E SC3201-TC011 守门。
3. **存量处理**：修复上线后轮转清理 pb_access3.log 历史 BackupCount 文件，避免存量 Key 继续暴露。
4. **可选加固（非必需，后续跨仓变更，见 §9）**：评估是否消除"原始 Key 作为标识符"的设计——ai-route apikey 规则 owner 与 quota plan Id 改用 key_id。注意：经核实，脱敏闸口对访问日志已是**完备**的输出侧控制（单条日志只携带本请求自己的 Key，闸口已知该 Key），该变更并非修复本缺陷所必需，仅作为降低对闸口依赖的可选优化，成本与触发条件见 §9。

---

## 3. 变更总览

| 泄漏字段 | 编号 | 现状 | 变更后 | 需修改的文件 |
|----------|------|------|--------|--------------|
| `authorization` | 49 | 记录 Authorization 头原值 | 置空（不输出） | `request_log.go` |
| `ai_route_rule_hits[].rule_owner` | 801 | 原始 Key（apikey 型规则 owner） | 替换为 `ClientKeyId`；无 key_id 时置空 | `request_log.go`（新增脱敏闸口统一处理） |
| `ai_auth_hit_quota_plans[]` | 841 | Plan Id 由原始 Key 构成 | 对 Plan Id 做原始 Key → `ClientKeyId` 字节级替换；未认证时置空 | `request_log.go`（新增脱敏闸口统一处理） |
| （防护） | — | 无全字段扫描 | 集成测试 SC05 增加全字段 raw-Key 字节扫描断言 | `tests/integration/...`（SC05 用例） |

> 说明：801/841 的数据源（`AiRouteResult.Owner`、`AiAuthInfo.HitQuotaPlans`）保持不变，本变更只在日志组装层做输出侧脱敏，不动路由匹配与配额校验逻辑（路由匹配仍在内存以 raw Key 进行）。

---

## 4. 详细设计

### 4.1 脱敏闸口的位置

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`（闸口实现位于同包新增 `credential_mask.go`）

在 `requestLogGen`（`request_log.go:41`）完成所有字段组装、返回 `*bfe_access_pb3.BfeLog` **之前**，调用统一的脱敏函数：

```go
func (m *ModuleAccessPb3) requestLogGen(req *bfe_basic.Request, res *bfe_http.Response) *bfe_access_pb3.BfeLog {
    ...
    reqAiInfoGen(requestLog, req, res)

    // credential masking gate: the last line of defense before log output
    maskSensitiveCredentials(requestLog, req)

    return bfeLog
}
```

设计要点：

- 闸口放在**最后一步**，对全部字段（含通用字段与 AI 字段）统一生效，避免后续新增字段绕过脱敏；
- 闸口只依赖 `req *bfe_basic.Request`，从中取 `AiBasicInfo.ClientApiKey`（原始 Key）与 `AiBasicInfo.ClientKeyId`（内部标识），**不需要跨模块访问 `mod_ai_token_auth` 的 token 表**——单条日志只携带本请求自己出示的那把 Key，801/841/49 中出现的原始 Key 必然等于 `ClientApiKey`；
- 非 AI 请求（`AiBasicInfo == nil`）不做替换，但字段 49 的置空策略对所有请求一致生效（见 §4.2）。

### 4.2 字段 49 `authorization` 置空

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log.go`（`reqReqHeaderInfoGen`，`request_log.go:220-225`）

删除/注释 Authorization 头的落盘逻辑，`reqLog.Authorization` 不再赋值：

```go
// Authorization
// 安全要求：认证头原文不落盘（见 bfenetworks/bfe#1357），原始 API Key 等凭据不得写入访问日志。
// values, found = req.HttpRequest.Header["Authorization"]
// if found {
//     data := strings.Join(values, ",")
//     reqLog.Authorization = proto.String(data)
// }
```

> 决策依据：issue #1357 修复建议明确"字段 49 authorization 建议直接置空——访问日志中认证头原文没有保留价值"。下游如需区分鉴权失败原因，已有字段 712 `ai_auth_reject_reason`。

### 4.3 字段 801 / 841 的替换规则

最终实现没有按字段逐个特判，而是在 `credential_mask.go` 中基于 protobuf reflection 做了**全字段通用递归扫描**（含嵌套 message 与 repeated 字段，如 `AIRouteRuleHit`、`ClusterKeyName`），801/841 只是被统一规则覆盖的两个具体字段：

```go
func maskSensitiveCredentials(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
    aiInfo := req.GetAiBasicInfo()
    if aiInfo == nil || aiInfo.ClientApiKey == "" {
        return
    }
    maskRawKeyInMessage(reqLog.ProtoReflect(), aiInfo.ClientApiKey, aiInfo.ClientKeyId)
}

// maskRawKeyValue：已认证时 rawKey -> key_id；未认证时整个值置空
func maskRawKeyValue(value string, rawKey string, keyId string) string {
    if keyId == "" {
        return ""
    }
    return strings.ReplaceAll(value, rawKey, keyId)
}
```

要点：

1. **已认证**（`ClientKeyId != ""`）：任何字段中 `strings.Contains(v, rawKey)` 的字符串值一律 `ReplaceAll` 替换为 `key_id`。801 的 owner 与 rawKey 精确相等，替换结果即 `key_id`；841 的 Plan Id 由原始 Key **构成**，替换后保留 Plan 的其他构成部分（如 `plan-<key_id>-monthly`）；
2. **未认证/鉴权失败**（`ClientKeyId == ""`）：相关字段置为空字符串，绝不允许提交的 Key 原文（含暴破 Key）落盘；
3. **通用扫描即兜底**：新增字段无需修改脱敏代码即被覆盖，作为防回归的纵深防线；
4. 对非 string 标量、枚举、map 字段不处理（`RequestLog` 无 string map；凭据只会以字符串形态出现）。

### 4.4 与认证模块的协作约定

- `AiBasicInfo.ClientApiKey`：客户端提交的原始 Key，已在内存中（用于上游转发），脱敏闸口只读取、不修改；
- `AiBasicInfo.ClientKeyId`：`mod_ai_token_auth` 认证成功后写入（见 `token_rule_table.go` 中 `token.KeyId` 的传递路径）；
- 若认证模块未运行或请求未走 AI 网关（`AiBasicInfo == nil`），闸口跳过替换，但 49 置空不受影响。

---

## 5. 涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `bfe/bfe_modules/mod_access_pb3/request_log.go` | `reqReqHeaderInfoGen` 移除 Authorization 落盘；`requestLogGen` 末尾接入脱敏闸口 |
| `bfe/bfe_modules/mod_access_pb3/credential_mask.go`（新增） | `maskSensitiveCredentials` 及兜底扫描实现 |
| `bfe/bfe_modules/mod_access_pb3/request_log_test.go` | 更新 Authorization 断言（改为断言为空）；新增 801/841 脱敏断言与未认证置空断言 |
| `bfe/tests/integration/`（SC05 用例） | 增加全字段 raw-Key 字节扫描断言 |
| `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md` | §6.1 安全说明同步：49/801/841 的脱敏语义 |

> 明确**不修改**：`mod_ai_token_auth/token_rule_table.go`（841 数据源不变，输出侧脱敏）、`bfe_basic/request_ai_basic.go`（字段语义不变）。可选加固方案（§9，非必需）落地时再动导出侧。

---

## 6. 测试计划

### 6.1 单元测试

**文件：** `bfe/bfe_modules/mod_access_pb3/request_log_test.go`

1. 已认证请求：`ClientApiKey="#AI_product-abc"`、`ClientKeyId="key-123"`，构造 `AiRouteRuleHits`（owner=raw key）与 `AiAuthHitQuotaPlans`（含 raw key 的 plan id）：
   - 断言 `Authorization == nil`；
   - 断言 `AiRouteRuleHits[0].RuleOwner == "key-123"`；
   - 断言 `AiAuthHitQuotaPlans` 中不再包含 raw key 字节串，且包含 `"key-123"`；
   - 断言 `AiApikeyId == "key-123"`（保持 701 语义不回退）。
2. 未认证请求（`ClientKeyId == ""`）：断言 801/841 中被置空，日志任何字段均不含 `ClientApiKey` 字节串。
3. 非 AI 请求：断言 49 置空，其余字段不受影响。

### 6.2 编译验证

```bash
cd bfe
go build ./...
go test ./bfe_modules/mod_access_pb3/...
```

### 6.3 集成验证

1. BFE 集成测试 SC05 增加**全字段 raw-Key 字节扫描断言**：对解码后的 `RequestLog` 做序列化字节级扫描，断言不含测试用 raw Key；现有仅校验 701=key_id 的断言保留。
2. 端到端：E2E SC3201-TC011（run_id 20260907-081746.041888124-sc3201tc011）解除 FAILED_PRODUCT 挂起、requeue 复验，解码 `pb_access3.log` 确认 49/801/841 均无原文。

---

## 7. 兼容性说明

1. **字段 49 `authorization` 语义变化**：从记录认证头原文改为不输出。下游日志消费方若依赖该字段需提前通知；AI 场景下鉴权失败原因已有 712 `ai_auth_reject_reason` 承载。
2. **字段 801 `rule_owner` 语义变化**（apikey 型规则）：从原始 Key 改为 key_id。proto 编号与类型不变，仅取值变化。
3. **字段 841 `ai_auth_hit_quota_plans`**：取值中原始 Key 部分被 key_id 替换，其余构成不变。依赖 plan Id 精确值做关联的下游需同步适配（若 §9 可选加固落地、plan Id 本身改为 key_id 构成，届时再次对齐）。
4. **非 AI 请求**：仅字段 49 行为变化，其余字段不受影响。

---

## 8. 风险与回滚

### 8.1 主要风险

| 风险 | 说明 | 规避措施 |
|------|------|----------|
| 下游依赖字段 49 / 801 原值 | 日志消费方可能用 authorization 或 rule_owner 做分析/关联 | 提前通知按 key_id 关联；SC3201-TC011 复验时同步确认下游 |
| 脱敏闸口遗漏字段 | 后续新增字段可能再次携带原始 Key | 兜底全字段字节扫描（§4.3）+ SC05 回归断言 |
| 未认证场景误留 Key | 鉴权失败路径的 801/841/49 处理不一致 | 单元测试覆盖未认证分支；SC3201-TC011 含 INVALID_API_KEY 场景 |
| key_id 为空导致日志关联困难 | 老配置 token 无 key_id | 认证模块已要求导出 token 必含 key_id（见 2026-08-19 变更 §8.1） |

### 8.2 回滚方案

1. 恢复 `reqReqHeaderInfoGen` 中 Authorization 落盘逻辑；
2. 移除 `requestLogGen` 末尾的 `maskSensitiveCredentials` 调用（或整体删除 `credential_mask.go`）；
3. 恢复 `request_log_test.go` 旧断言；
4. 重新编译部署。

> 注意：回滚会重新引入 bfenetworks/bfe#1357 的凭据泄漏缺陷，仅作为应急手段，不建议长期停留。

---

## 9. 后续扩展（可选加固，非必需，不在本变更范围）

> 定位说明：本节为**可选**的深度加固，**不是**修复 bfenetworks/bfe#1357 的必要条件。脱敏闸口（§4）对访问日志已是完备的输出侧控制——单条日志只携带本请求自己出示的那把 Key，闸口已知该 Key（`ClientApiKey`），可覆盖全部现有及未来字段（兜底全字段扫描）；经核实 `routeResult.Owner` 在 BFE 侧仅流向访问日志（`request_log.go:480`），`plan.Id` 仅流入 `HitQuotaPlans`，不存在闸口覆盖不到的旁路。

1. **导出侧消除"原始 Key 作为标识符"（可选）**：ai-route apikey 规则 owner 与 quota plan Id 改用 key_id。路由匹配仍在内存以 raw Key 进行，仅导出/日志证据字段换 id。带来的收益仅是"owner/plan Id 本身不再敏感"，从而降低对脱敏闸口的依赖。
2. **成本与风险**：涉及 ai-gateway-api / conf-agent 导出格式与 BFE 消费两端，需同步发版；且 plan Id 同时是配额计量（Redis 余额检查）的标识，改构成意味着存量配额数据的迁移或双读兼容，风险不小。客户端只出示原始 Key，"原始 Key 作为内存内 join key"本身不可避免，只要它不越过系统边界（日志面已被闸口守住），就不构成泄露。
3. **建议的触发条件**：当出现以下情况时再启动该变更——
   - 导出格式被 BFE 以外的独立组件消费并自行落日志（如 conf-agent / api-server 审计日志直接记录 owner/plan Id），闸口无法覆盖这些面；
   - 安全合规明确要求系统内标识符也不得包含凭据原文。
4. **若该加固落地**，本变更的 801/841 替换逻辑可简化为直通（owner/plan Id 本身已是 key_id），脱敏闸口仍保留作为最后防线。
5. **存量日志清理（必须，独立于上述可选项）**：修复上线后轮转清理 pb_access3.log 历史 BackupCount 文件，避免存量 Key 继续暴露。

---

*文档生成日期：2026-09-07*
