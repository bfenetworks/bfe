# TC-05 unknown inference pool 降级本地均衡

## 目的

验证 EPP 明确表示"无此 cluster 的 Cell"（`Internal / unknown inference pool`）时的降级语义：主备地址均尝试后回退本地 WRR 均衡，请求仍成功。

对应实现：`TestTC05_UnknownPoolFallback`。

## 前置条件

- 两个 mock EPP，均 `SetRequestError(Internal, "unknown inference pool")`。

## 执行步骤

1. 发送请求（标记串 `hello-epp-tc05`）。
2. 断言响应成功且由本地后端（sim）服务。
3. 断言指标：`epp_calls_total{result="unknown_pool"} >= 1`、`epp_fallback_local_total >= 1`。

## 预期结果

- 响应 200，body 含 `hello-epp-tc05`（本地回退成功）。
- `unknown_pool` 与 `epp_fallback_local_total` 打点均出现。

## 清理

- 同 TC-01。
