# TC-07 `EPPTimeout.Connect` 变更重建并继承状态（D4 / §4.3）

## 目的

验证连接型参数 `EPPTimeout.Connect` 变化触发 runtime 重建（修复 D4 的生效路径），且因**地址表不变**而通过
`inheritStateFrom` 继承 failover 状态（`active`），避免抖动并保证在途/后续请求不中断。

对应实现：`TestTC07_ConnectTimeoutRebuildKeepsState`。

## 前置条件

- 两个 mock EPP；`EPPCheck`：`CheckInterval=200ms`、`FailThreshold=1`、`Cooldown=30s`、`SuccessThreshold=1`；
  `EPPTimeout={Connect:200ms, Call:1s}`。

## 执行步骤

1. 主 EPP `SetServing(false)` 触发 failover → 等待 `epp_active_addr_index == 1`。
2. reload：`EPPTimeout.Connect` 由 200ms 改为 1s（地址不变、签名变化 → 重建）。
3. 断言 `epp_active_addr_index` 仍为 1（状态被继承，而非重建后归零）。
4. 再次请求 → 200，由 `epp[1]`（备）处理，`StreamCount` 增加。

## 预期结果

- 连接型参数变化触发重建；地址不变时 `active`/`health` 被继承；服务不中断。

## 说明

- 单元用例 `TestEPPRuntimeHotUpdate_ConnectTimeout` 通过检查新连接的 `MinConnectTimeout` 直接验证 D4 生效；
  本用例从进程外部验证同一次签名重建对**外部状态继承**的承诺。