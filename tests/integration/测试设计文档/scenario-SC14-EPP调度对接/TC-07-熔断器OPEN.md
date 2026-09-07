# TC-07 熔断器 OPEN

## 目的

验证 cluster 级熔断：EPP 调用连续失败达到窗口错误率阈值后熔断器 OPEN（状态迁移打点），后续请求短路到本地均衡且全部成功。

对应实现：`TestTC07_CircuitBreaker`。

## 前置条件

- 单 mock EPP；`EPPCheck.Disabled=true`（排除健康检查干扰）。
- `EPPBreaker`：`WindowSize=2`、`MinVolume=1`、`ErrorRatePercent=50`、`OpenTimeout=2s`。
- `EPPTimeout`：`Connect=200ms`、`Call=1s`。
- 启动后关闭 mock EPP。

## 执行步骤

1. 连续发送 3 个请求（标记串 `hello-epp-tc07`），均断言 200。
2. 轮询 `/monitor/epp_metrics` 的 `epp_breaker_transitions_total{cluster="cluster_epp_sim",state="open"}` 直至 >= 1。

## 预期结果

- 全部请求 200（熔断前后都走本地降级，服务不中断）。
- 10s 内出现 `open` 状态迁移打点。

## 清理

- 同 TC-01。
