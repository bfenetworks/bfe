# TC-01 EPP 决策转发与 pool metadata

## 目的

验证 EPP 调度的基本链路：BFE 把 cluster 名以 `llm-d.ai/inference-pool` metadata 注入 ext-proc 首消息；mock EPP 返回的决策地址（inference-sim）被 BFE 采纳为实际转发目标；请求头与请求体完整送达 EPP。

对应实现：`TestTC01_DecisionForwarding`。

## 前置条件

- inference-sim（`--model epp-test-model`，echo 模式）与单实例 mock EPP 已启动；mock EPP 决策地址 = inference-sim 地址。
- BFE 已启动，`cluster_epp_sim` 配置为 `BalanceMode=EPP`，`EPPAddr` 指向 mock EPP。

## 执行步骤

1. 向 BFE 发送 `POST /v1/chat/completions`，Host 为 `epp.example.org`，`Authorization: Bearer ak_epp`，body 含唯一标记串 `hello-epp-tc01`。
2. 断言响应状态与内容。
3. 读取 mock EPP 的记录（pool、请求头、请求体）与 `/monitor/epp_metrics`。

## 预期结果

- 响应 200，body 含 `hello-epp-tc01`（echo 自 sim，证明转发目标是 EPP 决策地址）。
- mock EPP `Pools() == ["cluster_epp_sim"]`（pool metadata 注入正确）。
- mock EPP `RequestBodies()` 与发出的 body 完全一致（请求体完整回传）。
- mock EPP `RequestHeadersList()[0]` 的 `Content-Type` 为 `application/json`。
- `epp_calls_total{cluster="cluster_epp_sim",result="ok"} >= 1`。

## 清理

- 停止 BFE、inference-sim 与 mock EPP（t.Cleanup 自动完成）。
