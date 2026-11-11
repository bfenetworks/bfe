# TC-10 early usage 无 final usage 按转发内容估算计费

## 用例编号与名称

TC-10 early usage 无 final usage 按转发内容估算计费（issue #1497，产品裁定 2026-11-11 采 #1415 估算口径）

## 所属场景

SC29 usage 抹零修复与 include_usage 注入

## 版本声明

- `bfe`：当前源码版本（含 issue #1497 修复）

## 测试目的

验证「上游在非终止 delta 里提前给出 usage、随后发送不带 usage 的 `response.completed` 终止事件」的异常形态：early usage 数值不得成为计费基数（early usage 按设计只贡献协议上下文），无 final usage 时按已转发内容估算计费（#1415 批准口径），访问日志 `ai_usage_source=early_suppressed`。修复前该形态按 early usage 聚合值精确扣费（`input=100/output=50 → 200000` 定点单位，与报表对账不一致）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_usage_snapshot` 已启动，SSE 按顺序返回三帧：
   1. content chunk（无 usage）：`{"type":"response.output_text.delta","delta":{"text":"hel"}}`；
   2. 非终止事件携带 early usage：`{"type":"response.output_text.delta","delta":{"text":"lo"},"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`；
   3. 无 usage 的终止事件：`[DONE]`。
4. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`，`EstimateToken=true`（模板默认）。

## 配置构造

- 模型价格：`deepseek-chat`，`input_cost_per_token: 0.000003`（300 定点/token）、`output_cost_per_token: 0.000009`（900 定点/token）。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `usage.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}` |

## 预期结果

- 响应状态码：200，客户端收到全部三帧（含 early usage 帧原样透传）。
- Redis `quota:plan_rmb` 扣减 = `100×300 + completionEst×900`，其中 `completionEst = len(content帧)/4 + len("[DONE]")/4`（guess 帧累加；early usage 帧为 non-guess 贡献 0）——**精确排除修复前的缺陷签名**：修复前 early usage 直通计费副本，`prompt=100×300 + completion=50×900 = 75000` 定点（issue 生产 run 在另一价格表下为 `200000` 定点）。
- 访问日志：`ai_input_tokens=100`（early usage prompt 上下文）、`ai_output_tokens=completionEst`（仅转发内容估算，不含 early 的 50）、`ai_total_tokens=100+completionEst`（#1398 镜像语义）、`ai_usage_source=early_suppressed`。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
