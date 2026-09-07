# TC-03 健康检查主备 failover

## 目的

验证 `EPPAddr` 有序主备消费与滞回 failover：主 EPP 宕机后，BFE 健康检查（gRPC health，短周期测试参数）连续失败达到 `FailThreshold` 即切到备地址，后续请求由备 EPP 调度。

对应实现：`TestTC03_HealthCheckFailover`。

## 前置条件

- 两个 mock EPP（主 `[0]`、备 `[1]`），决策地址均为 inference-sim。
- `EPPCheck`：`CheckInterval=200ms`、`FailThreshold=1`、`Cooldown=30s`、`SuccessThreshold=1`（冷却期远长于测试时长，避免 failback 干扰）。

## 执行步骤

1. 基线请求 → 断言由主 EPP 处理（`epp[0].StreamCount() == 1`）。
2. 关闭主 mock EPP（模拟进程宕机）。
3. 轮询 `/monitor/epp_metrics` 的 `epp_active_addr_index{cluster="cluster_epp_sim"}` 直至变为 1。
4. 再次请求 → 断言由备 EPP 处理（`epp[1].StreamCount() == 1`）。
5. 断言 `epp_failover_total >= 1`。

## 预期结果

- failover 在 15s 内发生（测试参数下通常 1s 内）。
- 切备后请求成功（200，echo 命中 sim）。
- `epp_failover_total{cluster="cluster_epp_sim"} >= 1`。

## 清理

- 同 TC-01。
