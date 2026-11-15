# TC-04 槽满触发 cluster fallback

## 目的

验证"全部 Key 会话槽满"的合成 429 走 issue #1440 契约（`ErrCode=ErrAiNoAvailableKey`，非 `ErrAiRateLimit`），**正常触发 cluster fallback**：路由配 fallback 时槽满请求降级到 fallback 集群并成功。

## 前置条件

- primary 集群 `cluster_key_conc_primary`：单 Key（`key-a`，Weight 100）、`MaxConcurrencyPerKey=1`；
- fallback 集群 `cluster_key_conc_fb`：单 Key（`key-fb`）、不限并发；
- 路由规则 `key-conc-fallback`：`targets=[cluster_key_conc_primary]`，`fallbacks=[cluster_key_conc_fb]`；
- `ApikeyRouteTableBindings`：`ak_conc_fb` 绑该路由表；
- miniredis 为空。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | POST |
| Host | `key-conc.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_conc_a`（随后 `ak_conc_fb`） |
| Body | `{"model":"gpt-4"}` |

两集群后端均返回 200。

## 执行步骤

1. 会话 A（`ak_conc_a`）发 1 条请求 → 200，占用 primary 的 `key-a`；
2. 会话 B（`ak_conc_fb`，绑定 `key-conc-fallback` 路由表）发 1 条请求；
3. 断言 primary 后端 Hits=1（无新增）、fallback 后端 Hits=1。

## 预期结果

- 步骤 2：**200**，响应来自 fallback 集群（`AuthHeaders()` 为 `key-fb`）；
- fallback 触发链路：primary 槽满 → 合成 429（`NO_AVAILABLE_KEY`）→ `shouldTriggerFallback` 白名单命中 → 降级 `cluster_key_conc_fb`；
- primary 零新增上游交互；
- B 在 primary 的占用 ZSet 无成员（合成 429 前 acquire 全部失败，未入集）。

## 清理

测试结束时关闭 BFE、mock 后端与 miniredis。
