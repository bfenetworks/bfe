# TC-20 配额拒绝请求保留 ai_apikeytags（issue #1418）

## 用例编号与名称

TC-20 配额拒绝请求保留 ai_apikeytags（配额预检拒绝的 429 日志必须保留 701/702 身份块）

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：v1.8.9-dev 分支（含 issue #1418 修复，见 `docs/zh_cn/modifications/2026-10-12-issue-1418-quota-reject-missing-apikeytags/design-changes.md`；由 [rainway-ai-gateway/ai-gateway-api#238](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/238) 克隆迁移）

## 测试目的

验证被配额预扣层拒绝的请求（HTTP 429 `QUOTA_EXHAUSTED`），其访问日志（`mod_access_pb3`）仍保留与同一 Key 成功请求一致的身份/审计块：`ai_apikey_id`（701）与 `ai_apikeytags`（702，含 `TagLevel`），外加拒绝字段 `ai_auth_reject_reason`（712）/ `ai_auth_reject_quota_plans`（713）。

修复前（issue #1418）：配额预检在 `ValidateUserTokenByReq` 内提前返回，发生在 `SetTokenAuthContext()`（702 的唯一写入点）之前，导致 429 记录「701/712/713 齐、702 缺席、Cluster 为空」，配额拒绝请求无法按 Entity 维度审计/对账。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01 的环境（`newTestEnv`），Token `ak_user_a` 携带 Entity 标签 `{TagName: "department", TagValue: "ai-team", TagLevel: 3}`，绑定 RMB QuotaPlan `plan_rmb`（`quota:plan_rmb`）。
2. 嵌入式 Redis 余额可在两个臂之间通过 `SetQuota` 改写：臂 A 前置为 0（余额耗尽），臂 B 恢复为正数。

## BFE 请求

依次发送 2 次 POST 请求：

| 臂 | Host | Path | Authorization | Redis 余额 | 预期状态码 |
|----|------|------|---------------|-----------|------------|
| A（配额耗尽拒绝） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `quota:plan_rmb = 0` | 429 `QUOTA_EXHAUSTED` |
| B（对照：同 Key 成功） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `quota:plan_rmb = 10000000000` | 200 |

请求体均为 `{"model":"deepseek-chat"}`。

## 预期结果

- 臂 A：`cluster_rmb` 命中 0 次（预检拒绝，未进入转发）；臂 B：命中 1 次。
- b2log 中存在 2 条 `RequestLog`，按 `res_status_code`（429 / 200）区分两臂，逐条断言：
  - 臂 A（`res_status_code=429`）：`ai_apikey_id = user_a_key_id`；`ai_apikeytags` 恰为 `[{tagname="department", tagvalue="ai-team", taglevel=3}]`（含 TagLevel）；`ai_auth_reject_reason = QUOTA_EXHAUSTED`；`ai_auth_reject_quota_plans = [plan_rmb]`；
  - 臂 B（`res_status_code=200`）：`ai_apikey_id = user_a_key_id`；`ai_apikeytags` 与臂 A 完全一致（同 key 同标签，作为 429 行的标签基线）；
  - 公共断言：凭据脱敏（逐条 `assertNoSensitiveCredential`）。
- 计费侧：臂 A 零扣减；臂 B 按正常 RMB 口径扣减（本用例不断言具体余额，由 SC03 覆盖）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。

## 实施备注

- 实现为 `sc05_access_log_ai_fields_test.go` 中 `TestTC20_QuotaRejectKeepsApikeyTags`；
- `assertApikeyTags` 扩展为同时比较 `taglevel`（此前只比较 tagname/tagvalue），共享 Token 的测试标签补充 `TagLevel: 3`（与 issue 现场的 level-3 Entity 标签同形），TC-01 的期望标签同步更新；
- 本用例在修复落地前应当失败（臂 A 的 `ai_apikeytags` 为空），修复后通过；
- 102 `Cluster` 在臂 A 保持为空属固有形态（认证阶段拒绝未发生转发），本用例不断言该字段。
