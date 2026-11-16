# TC-19 非 2xx 响应不输出 token 字段（issue #1409 / #1493）

## 用例编号与名称

TC-19 非 2xx 响应不输出 token 字段（未成功调用上游的请求不得报告模型用量）

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：v1.9.0-dev 分支（含 issue #1493 修复；#1493 恢复了 #1409 的本用例语义，反转了 #1480 的 `ai_usage_source=failed` 上报——对照见 `docs/zh_cn/modifications/2026-11-08-issue-1493-non2xx-usage-suppression-restore/design-changes.md`）。
- 本用例臂 A 是系统测试 **SC3201-TC011**（issue #1493）的仓内复刻：无路由模型 + 无路由绑定的 Key 请求该模型，BFE 本地 404，访问日志不得出现鉴权播种的 `ai_input_tokens = 请求体/4`。
- 本用例臂 D 是系统测试 **SC2201-TC005**（issue #1480/#1493 复验，见 `docs/zh_cn/modifications/2026-11-11-issue-1480-sc2201-tc005-non2xx-usage-counter-fix/design-changes.md`）的仓内复刻：自该修复起，非流式响应的 usage 解析不再被 200 门槛门控（`mod_ai_token_auth.tokenReadResponseHandler`），错误体携带真实 usage 对象的非 2xx 行按 `parsed` 观测上报（`Non2xxRealUsageKept` 计数、不计费）。

## 测试目的

验证访问日志（`mod_access_pb3`）中，**未成功调用上游**的请求（非 2xx）不输出 `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` / `ai_usage_source` / `ai_cost_value`，并且计费口径的行仍满足不变量 `ai_total_tokens = ai_input_tokens + ai_output_tokens`。

修复前（issue #1409 现场）：生产 pb3 日志中 `res_status_code=404` 的行带有 `ai_input_tokens > 0`（鉴权阶段按请求体长度播种的 `EstimateToken` 估算值）与 `ai_output_tokens`（哨兵夹取值），而 `ai_total_tokens=0`、`ai_cost_value` 缺失——三元组口径断裂，污染按 token 聚合的用量统计/对账/报表。

口径反复记录：#1409 确立「非 2xx ⇒ 字段缺席」→ #1480 反转为「非 2xx 带 `ai_usage_source=failed` 照实记录估算残留」（未同步修订规范文档）→ **#1493 依据 `2026-08-19-update-ai-access-log-fields/design-changes.md` §10 规范条款（字段 706/707/708 数据源是上游真实 usage；未成功调用上游不得填估算残留）裁定运行时失格，恢复 #1409 语义**。计数器随恢复改回 `Non2xxEstimateDropped`。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01 的环境（`newTestEnv` + `estimate` 默认开启：`bfe.conf` 未设置 `EstimateToken`，出厂默认为 `true`）。
2. `testdata/server_data_conf/host_rule.data` 的 `ai_product` 主机列表额外包含 `noroute.example.org`；该主机**没有**对应的 `ai_route` 规则（`mod_ai_route/ai_route.data` 只覆盖 `rmb.example.org` / `notable.example.org`），因此请求会先通过 token 鉴权（`token_rule.data` 的条件为 `default_t()`，按产品 `ai_product` 命中）并被播种 prompt 估算值，随后因"无 AI 路由"返回 BFE 本地生成的 404。
3. `cluster_rmb` 的 mock 后端可在运行期改写响应状态与响应体（`MockBackend.Response` / `.Body`）。

## BFE 请求

依次发送 4 次 POST 请求（每次使用独立连接，避免 404 响应后的连接复用干扰）：

| 臂 | Host | Path | Authorization | 后端响应 | 预期状态码 |
|----|------|------|---------------|----------|------------|
| A（无 AI 路由，SC3201-TC011 复刻） | `noroute.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | 未调用后端 | 404 |
| B（上游 404） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `404` + `{"error":{"message":"model not found","type":"invalid_request_error"}}` | 404 |
| C（对照：200 无 usage） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `200` + `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}` | 200 |
| D（上游 409，错误体携带 usage 对象） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `409` + `{"error":{"message":"conflict"},"usage":{"prompt_tokens":7,"completion_tokens":11,"total_tokens":18}}` | 409 |

请求体均为 `{"model":"deepseek-chat"}`（与目标模型一致，不触发 body 改写）。

## 预期结果

- 臂 A：`cluster_rmb` 命中 0 次（无上游调用）；臂 B：命中 1 次；臂 C：累计 2 次；臂 D：累计 3 次。
- b2log 中存在 4 条 `RequestLog`，按状态码与 `ai_route_rule_hits` 是否为空区分四臂，逐条断言：
  - 臂 A（`res_status_code=404`）：`ai_input_tokens`、`ai_output_tokens`、`ai_total_tokens`、`ai_usage_source`、`ai_cost_value` **字段缺席**（`nil`，与"用量为 0"区分）；`ai_apikey_id=user_a_key_id`（AI 身份字段照常输出）；`ai_cluster_key_names` 为空；
  - 臂 B（`res_status_code=404`）：同上五个字段均为 `nil`；`ai_cluster_key_names` 非空（记录尝试过的 cluster/key）；
  - 臂 D（`res_status_code=409`）：**SC2201-TC005 复刻（修复后语义）**——错误体中的 usage 对象被解析为真实观测，`ai_input_tokens=7`、`ai_output_tokens=11`、`ai_total_tokens=18`（与上游对象逐字段一致）、`ai_usage_source=parsed` 均非空；`ai_cost_value` 缺席且 Redis 余额不变（非 2xx 永不计费）；`ai_cluster_key_names` 非空。**状态码取 409 而非 500**：500 及 400/401/402/403/422/429 在 fallback 白名单内（`bfe_server/reverseproxy.go` 的 `aiFallbackStatusCodes` + 全部 5xx），会触发集群级 fallback 改写最终响应，无法直通（实施时实测 500 被 fallback 到对照集群返回 200）；404/409 等非白名单 4xx 才直通；
  - 臂 C（`res_status_code=200`）：`ai_input_tokens > 0`、`ai_output_tokens > 0`、`ai_total_tokens` 非空，且 `ai_total_tokens == ai_input_tokens + ai_output_tokens`（#1480 步骤 1 的 2xx total 回退保留，估算口径行为不变）；
  - 公共断言：凭据脱敏（逐条 `assertNoSensitiveCredential`）。
- 计费侧：臂 A/B/D 不产生扣减（零用量；臂 D 显式断言发送前后 Redis 余额相等）；臂 C 按估算口径正常扣减（本用例不断言 Redis 余额绝对值，由 SC03 覆盖）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。

## 实施备注

- 实现为 `sc05_access_log_ai_fields_test.go` 中 `TestTC19_Non2xxNoUsageFields`，断言助手 `assertNoUsageFields`（#1493 起同时断言 `ai_usage_source` 缺席，适用于无真实 usage 的非 2xx 行——臂 A/B）与逐请求独立连接的助手 `sendRequestNoKeepAlive`；
- 臂 A/B 修复落地后 `bfe` 日志会出现 `mod_ai_token_auth: drop estimated usage of non-2xx response: status=404 ...` 与计数器 `Non2xxEstimateDropped`（#1480 期间的 `Non2xxUsageReported` 已随 #1493 改回）；臂 D 落地后日志出现 `non-2xx response carries real usage, reported without billing` 与计数器 `Non2xxRealUsageKept`；
- 「非 2xx 但上游确实随错误响应返回真实 usage 对象 ⇒ 按 `parsed` 观测上报」的例外（#1493 断言 B-02）原仅在单测层钉住（`TestReqAiInfoGenNon2xxRealUsageReportedAsParsed`、`TestTokenRequestFinishHandler_Non2xxKeepsRealUsageObservation`）——SC2201-TC005 修复（放开 `tokenReadResponseHandler` 的 200 门槛，完成标记与估算播种仍 2xx-only）后该形态在集成环境可真实构造，由本用例臂 D 钉住（响应日志三元组 7/11/18 + source=parsed + 不计费），作为系统测试 SC2201-TC005 的仓内复刻；
- 本用例在 #1493 修复落地前（#1480 口径下）应当失败（臂 A/B 的 token 字段非空），修复后通过；
- 上游 404 直通依赖 `aiFallbackStatusCodes` 不含 404（`bfe_server/reverseproxy.go`），若未来把 404 纳入 fallback 白名单，本用例臂 B 改为使用 `cluster_fallback_rmb` 反查即可。
