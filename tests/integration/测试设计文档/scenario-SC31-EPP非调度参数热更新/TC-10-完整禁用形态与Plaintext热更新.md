# TC-10 `EPPCheck.Disabled=true` + `EPPBreaker.Disabled=true` + `EPPTLS.Plaintext=true` 完整形态热更新

## 目的

验证 issue #218 的**完整生产形态**：健康检查与熔断器全程禁用，控制面只改 `EPPTLS.Plaintext=true` 并 reload
（`EPPAddr` 不变）。要求：

1. runtime 因 `plaintext` 签名变化被重建，data 连接以明文建连（对应设计 §9 端到端验收）；
2. `EPPCheck.Disabled=true` 生效：reload 后不再产生任何健康探针连接；
3. `EPPBreaker.Disabled=true` 生效：持续 EPP 失败**永不**使熔断器 OPEN，且每个请求**仍然调用 EPP**（不短路）；
4. EPP 恢复后请求重新经 EPP 明文通道决策。

对应实现：`TestTC10_PlaintextFullDisabled`。

## 前置条件

- 单 mock EPP（dual mode）决策地址为 mock backend。
- 初始：`EPPCheck={"Disabled": true}`、`EPPBreaker={"Disabled": true}`、`EPPTLS={"Insecure": true}`（TLS）；
  `EPPTimeout={Connect:200ms, Call:2s}`。

## 执行步骤

1. 基线请求 → 200；等待 `TLSConnCount >= 1`（TLS data 连接）。
2. reload：仅 `EPPTLS={"Plaintext": true}`（健康检查与熔断器配置保持不变）。
3. 调用 `RestrictProtocols(tls=false, plaintext=true)`：关闭已有 TLS 连接并拒绝新 TLS 连接。
4. 再次请求 → 200；断言 `epp_fallback_local_total == 0`、`StreamCount >= 2`、`PlaintextConnCount >= 1`。
5. 记录 `PlaintextConnCount`，休眠 1.5s（若健康检查仍在运行，约 7 个 200ms 探针周期会产生新连接），断言连接数
   **保持不变**（`EPPCheck.Disabled` 生效）。
6. `SetRequestError(Unavailable/"connection refused")` 让每次 EPP 调用失败；连续发送 5 次请求（均 200，本地回退）。
7. 断言 `epp_calls_total{result="transport"}` 增加 5（每请求都调用了 EPP，未短路）、
   `epp_fallback_local_total` 增加 5、`epp_breaker_transitions_total{state="open"} == 0`。
8. 清除错误后再发一次请求 → 200；断言 `epp_fallback_local_total` 不再增加（恢复经 EPP 明文决策）。

## 预期结果

- reload 后 data 连接为明文：旧 TLS 连接被打断后请求仍由 EPP 明文通道决策，无本地回退。
- 无健康探针：明文连接数在多个探针周期内保持平坦。
- 熔断器禁用语义：持续失败不 OPEN、不短路；恢复后明文 data 连接继续可用。

## 反证（未修复时）

签名判据回退为"仅比较地址"后，data 连接仍停留旧 TLS，被 `RestrictProtocols` 打断后回退本地 →
`epp_fallback_local_total >= 1`，步骤 4 断言失败（已实测验证）。