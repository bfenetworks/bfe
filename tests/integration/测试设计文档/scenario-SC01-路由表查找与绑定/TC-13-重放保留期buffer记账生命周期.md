# TC-13 重放保留期内的 buffer 记账生命周期（issue #1494）

## 用例编号与名称

TC-13 重放保留期内的 bytes_body buffer 记账生命周期（监控可见性 + 限额语义）

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本（含 issue #1494 修复；请求体缓冲按请求生命周期结算，见 `bfe.conf.md` 的 `TotalBodyBufferSize` 说明与 `docs/zh_cn/sys_design/mod_ai_route_bfe_changes.md` §4.3.5/§8.3）。
- 本用例是系统测试 **SC3101-TC006**（issue #1494）的仓内复刻。

## 测试目的

验证 `total_bytes_body_buffer` 的记账生命周期与 `Server.TotalBodyBufferSize` 的限额语义：

1. **监控可见性**：请求体被缓冲供 fallback 重放后，即使已写完主 Target（transport 调用 `Close()`），buffer 在重放决策完成前仍属「活跃」，gauge 必须在整个在途窗口可见；请求终态（fallback 循环收敛）后回落基线，且不得因重复结算而低于基线。
2. **限额语义**：`TotalBodyBufferSize > 0` 时，在途请求保留的 buffer 计入全局额度，第二个 fallback 请求的 `prepareRequestBodyForRetry()` 必须看到该额度并拒绝包装、禁用 fallback。

修复前（issue #1494 现场）：`bytes_body.Close()`（主 Target 写完后立即调用）同时减账并置位 released；buffer 仍被保留供 `Rewind()`，但 `Rewind()` → `SetBytes()` 因 released 跳过加账，于是重放保留期内的字节被漏记，gauge 在每次 attempt 结束时即回落，限额检查也读不到保留中的字节、可被绕过。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与 mock 后端。

## 前置条件

1. `newTestEnv` 环境；`cluster_primary_a`（500）+ `cluster_fallback_1`（200）的 fallback 路由（`large.example.org`）。
2. `MockBackend.HoldResponse`（响应发送前阻塞的 channel）与 common 助手 `GetBFETotalBytesBodyBuffer`（轮询 `/monitor/server_stat`）。
3. 臂 B 需要 `withTotalBodyBufferSize` 配置 BFE 的 `TotalBodyBufferSize`。

## 关键事实（决定参数设计）

AI 路径在进入 reverseproxy 之前就会把请求体包装为 bytes_body（`http_conn.go` `extractClientModel` → `ReqBodyJsonFetch` → `GetBodyAccessor`），且本方案在 buffer 创建时即计入 gauge、`Close()` 不减账，因此**当前请求自己的 body 在 prepare 限额检查时已计入 gauge**。臂 B 的限值必须满足 limit ∈ (1×bodySize, 2×bodySize]：A 检查时 gauge=自身 bodySize < limit（放行），B 检查时 gauge=A 保留 + B 自身 = 2×bodySize ≥ limit（拒绝）。

## BFE 请求

| 臂 | 配置 | 操作 | 预期 |
|----|------|------|------|
| A（gauge 可见性） | 无限额；primary hold | 异步发送 512KiB body 请求；轮询 gauge（25ms 间隔）直至 ≥ baseline+512KiB；关 hold | 在途窗口内 gauge 持续 ≥512KiB（上升可见）；primary 500 → fallback 200；终态 gauge 精确回落 baseline |
| B（限额语义） | `TotalBodyBufferSize = 1.5×bodySize`；primary hold | 异步发送请求 A（512KiB）；轮询 gauge 直至 A 保留（≥512KiB）；同步发送请求 B；300ms 后关 hold | B 的 `prepareRequestBodyForRetry` 看到 gauge=A+B 自身=2×bodySize ≥ limit → 拒绝包装、fallback 禁用 → B 拿到 primary 的 500；A 经 fallback 200；fallback 命中仅 A（1 次）、primary 命中 2 次 |

## 预期结果

- 臂 A：gauge 在 hold 窗口内 ≥ baseline+512KiB；请求结束后 10s 内回落至 baseline。
- 臂 B：A=200（经 fallback）、B=500（fallback 被限额禁用）、`cluster_fallback_1` 命中 1、`cluster_primary_a` 命中 2；`bfe` 日志出现 `request body is not rewindable, disable fallback`。
- 公共：全部结束后 gauge 精确回到 baseline，无跨请求泄漏；全程不得出现低于 baseline 的负向漂移（重复结算）。

## 清理

关闭 holder 后端阻塞，停止 `bfe` 进程与所有 mock 后端，删除临时目录。

## 实施备注

- 实现为 `sc01_route_table_lookup_test.go` 中 `TestTC13_RetainedBodyBufferGaugeLifecycle`（两个子测试 ArmA/ArmB）。
- 与上游 PR #1496 移植版本的差异：
  - 终态断言从 `total <= baseline` 收紧为 `total == baseline`，并在回落过程中检测 `total < baseline` 立即失败——本方案的释放点有两个（fallback 循环收敛处 + `serveRequest` defer），重复结算会把 gauge 打成负数，宽松断言无法发现；
  - 臂 A 的语义表述为「fallback 循环收敛后释放」（而非「请求终态后释放」），以覆盖长 SSE 响应期间不长期占用重试额度的设计意图；
  - 臂 B 增加请求全部结束后的跨请求无泄漏检查。
- 臂 B 的 B 请求曾期望 500 精准值：hold 未释放时 B 的 primary attempt 会等超时返回 502（网关侧 BACKEND_UNAVAILABLE 契约），故改为「B 异步发送 + 300ms 后关 hold」，B 拿到 primary 直通的 500。