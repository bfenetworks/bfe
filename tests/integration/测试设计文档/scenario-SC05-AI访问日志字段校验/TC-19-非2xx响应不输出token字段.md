# TC-19 非 2xx 响应不输出 token 字段（issue #1409）

## 用例编号与名称

TC-19 非 2xx 响应不输出 token 字段（未成功调用上游的请求不得报告模型用量）

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：v1.8.9-dev 分支（tip `1e27573d`，含 issue #1409 修复；v1.8.8 分支同步实现，见 `docs/zh_cn/modifications/2026-10-11-issue-1409-non2xx-estimate-token-residue-fix/design-changes.md` §九）

## 测试目的

验证访问日志（`mod_access_pb3`）中，**未成功调用上游**的请求（非 2xx）不输出 `ai_input_tokens` / `ai_output_tokens` / `ai_total_tokens` / `ai_cost_value`，并且计费口径的行仍满足不变量 `ai_total_tokens = ai_input_tokens + ai_output_tokens`。

修复前（issue #1409）：生产 pb3 日志中 `res_status_code=404` 的行带有 `ai_input_tokens > 0`（鉴权阶段按请求体长度播种的 `EstimateToken` 估算值）与 `ai_output_tokens`（哨兵夹取值），而 `ai_total_tokens=0`、`ai_cost_value` 缺失——三元组口径断裂，污染按 token 聚合的用量统计/对账/报表。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01 的环境（`newTestEnv` + `estimate` 默认开启：`bfe.conf` 未设置 `EstimateToken`，出厂默认为 `true`）。
2. `testdata/server_data_conf/host_rule.data` 的 `ai_product` 主机列表额外包含 `noroute.example.org`；该主机**没有**对应的 `ai_route` 规则（`mod_ai_route/ai_route.data` 只覆盖 `rmb.example.org` / `notable.example.org`），因此请求会先通过 token 鉴权（`token_rule.data` 的条件为 `default_t()`，按产品 `ai_product` 命中）并被播种 prompt 估算值，随后因"无 AI 路由"返回 BFE 本地生成的 404。
3. `cluster_rmb` 的 mock 后端可在运行期改写响应状态与响应体（`MockBackend.Response` / `.Body`）。

## BFE 请求

依次发送 3 次 POST 请求（每次使用独立连接，避免 404 响应后的连接复用干扰）：

| 臂 | Host | Path | Authorization | 后端响应 | 预期状态码 |
|----|------|------|---------------|----------|------------|
| A（无 AI 路由） | `noroute.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | 未调用后端 | 404 |
| B（上游 404） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `404` + `{"error":{"message":"model not found","type":"invalid_request_error"}}` | 404 |
| C（对照：200 无 usage） | `rmb.example.org` | `/v1/chat/completions` | `Bearer ak_user_a` | `200` + `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}` | 200 |

请求体均为 `{"model":"deepseek-chat"}`（与目标模型一致，不触发 body 改写）。

## 预期结果

- 臂 A：`cluster_rmb` 命中 0 次（无上游调用）；臂 B：命中 1 次；臂 C：命中 2 次累计。
- b2log 中存在 3 条 `RequestLog`，按状态码与 `ai_route_rule_hits` 是否为空区分三臂，逐条断言：
  - 臂 A（`res_status_code=404`）：`ai_input_tokens`、`ai_output_tokens`、`ai_total_tokens`、`ai_cost_value` **字段缺席**（`nil`，与"用量为 0"区分）；`ai_apikey_id=user_a_key_id`（AI 上下文字段照常输出）；`ai_cluster_key_names` 为空；
  - 臂 B（`res_status_code=404`）：同上四个 token/cost 字段均为 `nil`；`ai_cluster_key_names` 非空（记录尝试过的 cluster/key）；
  - 臂 C（`res_status_code=200`）：`ai_input_tokens > 0`、`ai_output_tokens > 0`、`ai_total_tokens` 非空，且 `ai_total_tokens == ai_input_tokens + ai_output_tokens`（issue #1409 断言 A-02；估算口径行为不变）；
  - 公共断言：凭据脱敏（逐条 `assertNoSensitiveCredential`）。
- 计费侧：臂 A/B 不产生扣减（零用量）；臂 C 按估算口径正常扣减（本用例不断言 Redis 余额，由 SC03 覆盖）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。

## 实施备注

- 实现为 `sc05_access_log_ai_fields_test.go` 中 `TestTC19_Non2xxNoUsageFields`，新增断言助手 `assertNoUsageFields` 与逐请求独立连接的助手 `sendRequestNoKeepAlive`（臂 A 的响应为 `closeAfterReply`，连接会被关闭，复用连接池会产生伪失败）；
- 臂 A 是 issue #1409 的现场主形态（鉴权先于 AI 路由检查，404 前已播种 prompt 估算值），修复落地后 `bfe` 日志会出现 `mod_ai_token_auth: drop estimated usage of non-2xx response: status=404 ...` 与计数器 `Non2xxEstimateDropped`；
- 本用例在修复落地前应当失败（臂 A/B 的 token 字段非空），修复后通过；
- 上游 404 直通依赖 `aiFallbackStatusCodes` 不含 404（`bfe_server/reverseproxy.go`），若未来把 404 纳入 fallback 白名单，本用例臂 B 改为使用 `cluster_fallback_rmb` 反查即可。