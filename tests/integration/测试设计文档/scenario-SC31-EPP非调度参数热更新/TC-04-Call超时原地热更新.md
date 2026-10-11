# TC-04 `EPPTimeout.Call` 原地热更新

## 目的

验证"读取型"参数 `EPPTimeout.Call` 变化后在下个请求生效，且不破坏在途语义（设计上该参数走原地 `updateConf`，
不重建 runtime）。

对应实现：`TestTC04_CallTimeoutHotUpdate`。

## 前置条件

- 单 mock EPP，`SetDecisionDelay(800ms)`；`EPPCheck.Disabled=true`（避免探针干扰）；`EPPTimeout.Call=150ms`。

## 执行步骤

1. 请求 → 因 Call 超时（150ms < 800ms）EPP 调用超时，回退本地均衡；等待 `epp_fallback_local_total >= 1`，记录基线值。
2. reload：`EPPTimeout.Call=3s`（地址与签名其余字段不变）。
3. 再次请求 → 200；等待 `epp_calls_total{result="ok"}` 增加。
4. 断言 `epp_fallback_local_total` 与步骤 1 基线相同（无新增回退）。

## 预期结果

- 新的 Call 超时对下个请求生效：EPP 决策成功，无本地回退。

## 说明

- 进程外部无法直接断言"未重建 runtime"（该判据由单元用例 `TestEPPRuntimeHotUpdate_ReadTypeInPlace` 的
  `assert.Same` 覆盖）；本用例从外部验证新超时确实生效，二者互补。