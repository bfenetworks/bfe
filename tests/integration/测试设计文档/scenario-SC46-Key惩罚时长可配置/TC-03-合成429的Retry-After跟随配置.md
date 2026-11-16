# TC-03 合成 429 的 Retry-After 跟随配置

## 目的

验证「所有可用 Key 都在惩罚窗口内」时网关合成的 429，其 `Retry-After` 头与 `error.details.retry_after_seconds` 取 `SessionAffinityPenaltyTTL429`（本期配置为 30），而不是固定常量 60；同时验证该请求对目标集群零上游交互。

## 前置条件

- `cluster_penalty_ttl` 配 2 把 Key，`SessionAffinity` / `PenaltyEnable` 均开启；
- `SessionAffinityPenaltyTTL429=30`；
- 上游对任意请求返回 429（第 1 条请求即把两把 Key 都写入惩罚窗口）；
- 不配置 fallback cluster。

## 请求构造

同 TC-01。

## 后端响应

`429` + `{"error":"rate limited"}`。

## 执行步骤

1. 启动 BFE，记录 mock 后端命中数（应为 0）；
2. 发送第 1 条请求：两把 Key 依次 429，均被判为 used 并写惩罚键，最终返回上游 429；
3. 记录后端命中数 `hits1`（应为 2）；
4. 发送第 2 条请求；
5. 检查第 2 条请求的状态码、`Retry-After` 头、响应体 `error.details.retry_after_seconds`，以及后端命中数是否仍为 `hits1`。

## 预期结果

- 第 2 条请求：状态码 `429`；
- `Retry-After: 30`；
- 响应体 `error.code=NO_AVAILABLE_KEY`、`error.details.retry_after_seconds=30`；
- 后端命中数不变（零上游交互，说明走的是惩罚熔断分支而非真实上游 429）；
- 惩罚键 TTL 仍为 30s。

## 清理

同 TC-01。