# TC-01 `EPPTLS` TLS→明文 热更新（D1）

## 目的

验证在 `EPPAddr` 不变、仅把 `EPPTLS.Plaintext` 由 false 改为 true 并 reload 后，EPP runtime 被重建：data 连接与
health 探针均以明文建连（修复 D1：旧实现只替换 `rt.conf`，data 连接仍用旧 TLS 凭据）。

对应实现：`TestTC01_TLSToPlaintextHotUpdate`。

## 前置条件

- 单 mock EPP（dual mode）决策地址为 mock backend。
- 初始 `EPPTLS={"Insecure": true}`（TLS）；`EPPCheck.CheckInterval=200ms`、`FailThreshold=1`；`EPPTimeout.Call=2s`。

## 执行步骤

1. 基线请求 → 200 且 body 含 `epp-reload-backend`；等待 `TLSConnCount >= 1`。
2. reload：`EPPTLS={"Plaintext": true}`（地址不变）。
3. 调用 `RestrictProtocols(tls=false, plaintext=true)`：关闭已有 TLS 连接并拒绝新 TLS 连接。
4. 再次请求 → 200；断言 `epp_fallback_local_total == 0`（未回退本地）。
5. 断言 `StreamCount >= 2`（请求确经 EPP 明文决策）且 `PlaintextConnCount >= 1`。

## 预期结果

- reload 后 data 连接为明文：旧 TLS 连接被打断后，请求仍由 EPP 明文通道决策，无本地回退。
- 探针以明文建连（`PlaintextConnCount >= 1`）。

## 反证（未修复时）

若不重建 runtime，data 连接仍为旧 TLS；被 `RestrictProtocols` 打断后重连 TLS 被拒 → EPP 调用失败 →
`epp_fallback_local_total >= 1`，断言失败。

## 清理

- 同 TC-01：`t.Cleanup` 停止 BFE、mock backend 与 mock EPP。