# TC-05 未变化 reload 不重置熔断窗口（D5）

## 目的

验证 reload 未改变 `EPPBreaker` 配置时，`eppBreaker.updateConf` 提前返回，熔断滑动窗口/计数保留（修复 D5：
旧实现每次 reload 都重建 `results` 并清零 `count/idx`，频繁 reload 会让熔断器无法累积到 OPEN）。

对应实现：`TestTC05_BreakerWindowSurvivesUnchangedReload`。

## 前置条件

- 单 mock EPP；`EPPCheck.Disabled=true`；`EPPTimeout={Connect:200ms, Call:1s}`；
  `EPPBreaker={WindowSize:5, MinVolume:5, ErrorRatePercent:50, OpenTimeout:30s}`。

## 执行步骤

1. 关闭 mock EPP（模拟 EPP 进程消失，所有 EPP 调用失败）。
2. 发送 4 次请求（窗口计数=4，未达 MinVolume=5，熔断器仍 CLOSED）。
3. reload：`EPPBreaker` 配置**保持不变**。
4. 依次发送 1~3 次请求，轮询 `epp_breaker_transitions_total{state="open"}`。
5. 断言 OPEN 迁移计数 `>= 1`。

## 预期结果

- 第 5 次失败（reload 后第 1 次）即满足窗口 5/5、错误率 100%，熔断器 OPEN。

## 反证（未修复时）

旧实现 reload 清零窗口，reload 后再失败 3 次仅 count=3 < 5，熔断器保持 CLOSED，断言失败。