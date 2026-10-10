# TC-08 `EPPCheck.Disabled=true` + `EPPTLS.Plaintext=true` 组合热更新

## 目的

验证 issue #218 的**生产形态**：一次 `/reload/server_data_conf` 同时把 `EPPCheck.Disabled` 与 `EPPTLS.Plaintext`
置为 true（`EPPAddr` 不变）后：

1. runtime 因 `plaintext` 签名变化被重建，data 连接以明文建连（对应设计 §9 端到端验收）；
2. `healthLoop` 保持停止，reload 后不再产生任何健康探针连接。

对应实现：`TestTC08_PlaintextWithCheckDisabled`。

## 前置条件

- 单 mock EPP（dual mode）决策地址为 mock backend。
- 初始：健康检查开启（`CheckInterval=200ms`、`FailThreshold=1`、`Cooldown=30s`、`SuccessThreshold=1`），
  `EPPTLS={"Insecure": true}`（TLS）；`EPPTimeout.Call=2s`。

## 执行步骤

1. 基线请求 → 200；等待 `TLSConnCount >= 1`（TLS data 连接）。
2. reload：`EPPCheck.Disabled=true`（保留同一滞回参数）且 `EPPTLS={"Plaintext": true}`。
3. 调用 `RestrictProtocols(tls=false, plaintext=true)`：关闭已有 TLS 连接并拒绝新 TLS 连接。
4. 再次请求 → 200；断言 `epp_fallback_local_total == 0`、`StreamCount >= 2`（经 EPP 明文决策）、
   `PlaintextConnCount >= 1`。
5. 记录 `PlaintextConnCount`，休眠 1.5s（若健康检查仍在运行，约 7 个 200ms 探针周期会产生新连接），
   断言连接数**保持不变**。

## 预期结果

- reload 后 data 连接为明文：旧 TLS 连接被打断后请求仍由 EPP 明文通道决策，无本地回退。
- `EPPCheck.Disabled=true` 生效：reload 后明文连接数在多个探针周期内不增长（无健康探针）。

## 反证（未修复时）

- 签名判据回退为"仅比较地址"后，data 连接仍停留旧 TLS，被 `RestrictProtocols` 打断后回退本地 →
  `epp_fallback_local_total >= 1`，步骤 4 断言失败（已实测验证）。
- 若 `Disabled` 热切换不生效（D3），`healthLoop` 继续运行，步骤 5 中明文连接数持续增长。