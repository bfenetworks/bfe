# TC-07 健康 Key 不被惩罚过滤误跳过

## 目的

回归 `2026-11-04-ai-key-penalty-filter-index-mismatch` 修复：惩罚过滤 + 失败标记必须使用同一套 `AIConf.Keys` 下标，不能因为候选集被过滤重排而把健康 Key 判为 dead。

修复前形态：会话先绑到 `key-a`（下标 0），`key-a` 返回 401 后被标 dead 并写惩罚键，重试时候选集只剩 `key-b`（下标 0），`deadSet[0]` 命中 `key-a` 的标记 → `key-b` 被判 dead → 同一请求直接返回合成 429，`key-b` 从未被尝试。

## 前置条件

- `cluster_penalty_ttl` 配 2 把可用 Key：`key-a` / `key-b`（Weight 50/50）；
- `SessionAffinity=true`、`SessionAffinityPenaltyEnable=true`、`SessionAffinityPenaltyTTLAuth=300`；
- 在 Redis 预置绑定 `bfe:ai:key_affinity:cluster_penalty_ttl:penalty_ttl_key_id = key-a`，把首次选 Key 固定到 `key-a`（下标 0），复现修复前的触发条件；
- mock 后端按注入的凭证区分：`sk-key-a` → 401，其余 → 200。

## 请求构造

同 TC-01。

## 后端响应

| Provider Key | 状态码 | 响应体 |
|--------------|--------|--------|
| `sk-key-a` | 401 | `{"error":"invalid api key"}` |
| `sk-key-b` | 200 | `{"ok":true}` |

## 执行步骤

1. 启动 BFE，预置绑定到 `key-a`；
2. 发送 1 条请求；
3. 检查响应码、mock 后端命中次数与两次注入的 `Authorization`、惩罚键集合及其 TTL。

## 预期结果

- 响应 `200`（由 `key-b` 兜住；修复前是合成 429）；
- mock 后端命中 **2** 次，顺序为 `key-a` → `key-b`；
- Redis 中只有 `...:key-a` 一个惩罚键，TTL = **300s**；
- 无「all usable keys penalized」熔断日志。

## 清理

同 TC-01。