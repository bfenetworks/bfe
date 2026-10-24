# TC-08 tool_call 配对修复（可修复路径）

## 用例编号与名称

TC-08 tool_call 配对修复（可修复路径）

## 所属场景

SC24 上下文压缩与裁剪

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_context`）

## 测试目的

与 TC-05 的"修复失败回滚"相对，验证 repair 的**可修复路径**：请求自带破损的 tool 配对时，`repair.go` 多趟修复（删除孤儿 tool 消息 / 剥离悬空 tool_calls）使 messages 重新合法，请求**以修复后产物正常转发**（状态 `trim`/`rewrite`，而非 `repair_rollback`）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 同 TC-01（默认 balanced 规则，budget 150）。

## 配置构造

无特殊配置（默认 testdata 规则）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body 要点 |
|------|------|------|---------------|-----------|
| 1 | `ctx.example.org` | `/v1/chat/completions` | `Bearer ak_ctx` | user + assistant(`tool_calls:[call_1]`) + tool(call_1) + **孤儿 tool(`tool_call_id=orphan`，600 字符内容)** + user——孤儿 tool 无任何 assistant 声明其 id，repair 删除之 |
| 2 | 同上 | 同上 | 同上 | user + assistant（600 字符 content + `tool_calls:[call_1, call_2]`）+ tool(call_1) + user——call_2 无 tool 应答，repair 剥离悬空 tool_calls |

两个 body 估算均超触发线（>120），确保管线触发。

## 预期结果

- 均返回 200，均带 `x-ai-context-compression` 标注。
- 步骤 1 后端：messages 从 5 条变 4 条（孤儿 tool 被删除）；无任何 role=tool 消息缺少前置 assistant 声明；600 字符孤儿内容不复存在。
- 步骤 2 后端：assistant `tool_calls` 只剩 `call_1`（悬空 `call_2` 被剥离）；`call_1` 与其 tool 应答成对保留。
- 两条请求的 tool 配对均合法（每 tool 消息有前置声明、每 tool_call 有后续应答）。
- 访问日志 2 条：状态为 `trim` 或 `rewrite`（取决于修复后估算是否仍超 budget；步骤 2 的 600 字符 filler 使 P2 介入，通常为 `rewrite`），**均不得为 `repair_rollback`**。
