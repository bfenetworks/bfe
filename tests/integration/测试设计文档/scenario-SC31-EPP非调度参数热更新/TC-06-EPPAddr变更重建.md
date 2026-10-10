# TC-06 `EPPAddr` 变更重建（拓扑型）

## 目的

验证拓扑型参数 `EPPAddr` 变化仍触发现有重建语义：新请求由新 EPP 决策，旧 EPP 不再被调用。
（既有行为，改造后必须保持。）

对应实现：`TestTC06_AddrChangeRebuild`。

## 前置条件

- 两个 mock EPP `[0]`、`[1]`，决策地址均为 mock backend；`EPPCheck.Disabled=true`。

## 执行步骤

1. 基线请求 → 由 `epp[0]` 处理（`StreamCount == 1`）。
2. reload：`EPPAddr=[epp[1].Addr()]`。
3. 再次请求 → 200；断言 `epp[1].StreamCount == 1`，`epp[0].StreamCount` 保持 1（未新增）。

## 预期结果

- 地址表变化触发 runtime 重建，调度切换到新 EPP；旧地址不再接收流。