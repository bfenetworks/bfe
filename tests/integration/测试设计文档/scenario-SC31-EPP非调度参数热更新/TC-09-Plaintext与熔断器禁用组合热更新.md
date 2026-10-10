# TC-09 `EPPBreaker.Disabled=true` + `EPPTLS.Plaintext=true` 组合热更新

## 目的

验证 issue #218 的**生产形态**：`EPPBreaker` 全程禁用，控制面只改 `EPPTLS.Plaintext=true` 并 reload（`EPPAddr`
不变）。要求：

1. runtime 因 `plaintext` 签名变化被重建，data 连接以明文建连（对应设计 §9 端到端验收）；
2. `EPPBreaker.Disabled=true` 语义保持：持续 EPP 失败**永不**使熔断器 OPEN，且每个请求**仍然调用 EPP**
   （不短路到本地均衡）；
3. EPP 恢复后请求重新经 EPP 明文通道决策。

对应实现：`TestTC09_PlaintextWithBreakerDisabled`。

## 前置条件

- 单 mock EPP（dual mode）决策地址为 mock backend。
- 初始：`EPPBreaker={"Disabled": true}`、`EPPCheck={"Disabled": true}`（聚焦 data 通道，避免探针干扰）、
  `EPPTLS={"Insecure": true}`（TLS）；`EPPTimeout={Connect:200ms, Call:2s}`。

## 执行步骤

1. 基线请求 → 200；等待 `TLSConnCount >= 1`（TLS data 连接）。
2. reload：仅 `EPPTLS={"Plaintext": true}`（`EPPBreaker` 保持不变）。
3. 调用 `RestrictProtocols(tls=false, plaintext=true)`：关闭已有 TLS 连接并拒绝新 TLS 连接。
4. 再次请求 → 200；断言 `epp_fallback_local_total == 0`、`StreamCount >= 2`、`PlaintextConnCount >= 1`。
5. `SetRequestError(Unavailable/"connection refused")` 让每次 EPP 调用失败；连续发送 5 次请求（均 200，本地回退）。
6. 断言 `epp_calls_total{result="transport"}` 增加 5（每请求都调用了 EPP，未短路）、
   `epp_fallback_local_total` 增加 5、`epp_breaker_transitions_total{state="open"} == 0`。
7. 清除错误后再发一次请求 → 200；断言 `epp_fallback_local_total` 不再增加（恢复经 EPP 明文决策）。

## 预期结果

- reload 后 data 连接为明文：旧 TLS 连接被打断后请求仍由 EPP 明文通道决策，无本地回退。
- 熔断器禁用语义：持续失败不 OPEN、不短路；恢复后明文 data 连接继续可用。

## 反证（未修复时）

签名判据回退为"仅比较地址"后，data 连接仍停留旧 TLS，被 `RestrictProtocols` 打断后回退本地 →
`epp_fallback_local_total >= 1`，步骤 4 断言失败（已实测验证）。