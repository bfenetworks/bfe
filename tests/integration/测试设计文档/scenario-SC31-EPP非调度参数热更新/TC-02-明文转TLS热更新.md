# TC-02 `EPPTLS` 明文→TLS 热更新（D2）

## 目的

验证在 `EPPAddr` 不变、仅把 `EPPTLS.Plaintext` 由 true 改为 false 并 reload 后，runtime 被重建并构建出非 nil 的
`tlsConf`，data 连接与探针均切回 TLS（修复 D2：旧实现 `plaintext` 时跳过 `BuildTLSConfig`，原地更新不重建导致
`rt.tlsConf` 仍为 nil）。

对应实现：`TestTC02_PlaintextToTLSHotUpdate`。

## 前置条件

- 单 mock EPP（dual mode）决策地址为 mock backend。
- 初始 `EPPTLS={"Plaintext": true}`；`EPPCheck.CheckInterval=200ms`、`FailThreshold=1`；`EPPTimeout.Call=2s`。

## 执行步骤

1. 基线请求 → 200；等待 `PlaintextConnCount >= 1`。
2. reload：`EPPTLS={"Insecure": true}`（明文置 false，地址不变）。
3. 调用 `RestrictProtocols(tls=true, plaintext=false)`：关闭已有明文连接并拒绝新明文连接。
4. 再次请求 → 200；断言 `epp_fallback_local_total == 0`。
5. 断言 `StreamCount >= 2` 且 `TLSConnCount >= 2`（新建 TLS data + 探针）。

## 预期结果

- reload 后 data 连接与探针均为 TLS；旧明文连接被打断后请求仍由 EPP TLS 通道决策。
- 未出现 `rt.tlsConf == nil` 导致的探测异常（防御性校验不触发）。

## 反证（未修复时）

若不重建 runtime，data 连接停留在明文；被 `RestrictProtocols` 打断后重连明文被拒 → 回退本地 →
`epp_fallback_local_total >= 1`，断言失败。