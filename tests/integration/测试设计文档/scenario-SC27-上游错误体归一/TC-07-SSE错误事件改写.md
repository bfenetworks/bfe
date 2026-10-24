# TC-07 SSE错误事件改写

## 用例编号与名称

TC-07 SSE错误事件改写

## 所属场景

SC27 上游错误体归一

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.NormalizeUpstreamError` 实现）

## 测试目的

验证流式归一：HTTP 200 的 SSE 流中，OpenAI 形态错误事件
（`data: {"error":...}`）与 Anthropic 形态错误事件（`event: error`）的
data 载荷被改写为统一错误 JSON；响应状态保持 200、SSE 事件结构（event 名、
分块、换行）保形、正常内容事件透传。

## 运行模式

单组件模式：真实 `bfe` + 脚本化 SSE mock 后端，无 Redis。

## 前置条件

1. `cluster_err` AIConf：`NormalizeUpstreamError={StreamEnabled:true}`
   （`Enabled` 缺省 false——验证流式开关独立于非流式开关）。
2. `ak_front` 无限额配额计划。

## 配置构造

```json
"NormalizeUpstreamError": {"StreamEnabled": true}
```

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_front` | `{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`（OpenAI 脚本） |
| 2 | `front.example.org` | `/v1/messages` | `x-api-key ak_front` | `{"model":"claude-sonnet","stream":true,"messages":[{"role":"user","content":"hi"}]}`（Anthropic 脚本） |

> 说明：按请求 path 区分后端脚本（`/v1/chat/completions` 走 OpenAI 脚本，
> `/v1/messages` 走 Anthropic 脚本）。

## 后端响应

OpenAI 脚本（200 SSE）：

```text
data: {"choices":[{"delta":{"content":"部分结果"}}]}

data: {"error":{"message":"stream failed with key sk-upstream-secret","type":"authentication_error"}}

data: [DONE]

```

Anthropic 脚本（200 SSE）：

```text
event: message_start
data: {"type":"message_start"}

event: error
data: {"type":"error","error":{"type":"rate_limit_error","message":"stream rate limited"}}

```

## 执行步骤

1. 启动 BFE，依次发送两个流式请求，完整读取 SSE 响应；
2. 逐事件解析响应流；
3. 检查日志记录。

## 预期结果

- 步骤 1：
  - 响应状态 **200**、`Content-Type: text/event-stream`；
  - 内容事件 `data: {"choices":[{"delta":{"content":"部分结果"}}]}` 原样透传；
  - 错误事件 data 载荷为统一错误 JSON：`error.code=="UPSTREAM_AUTH_ERROR"`
    （`authentication_error` 类型映射）、message 中 `sk-upstream-secret` 被
    掩码；事件块以 `data:` 行重新承载，块间空行结构不变；
  - `data: [DONE]` 原样透传；
- 步骤 2：错误事件 `event: error` 行保留，data 载荷改写为
  `UPSTREAM_RATE_LIMITED` 统一错误 JSON；
- 访问日志（两请求均）：`ai_stream_error_rewritten=true`、
  `ai_err_normalized=true`、`ai_upstream_status=200`、
  `ai_upstream_err_code` 为对应上游错误码；
- 两请求均**无**截断标记（OpenAI 流有 `[DONE]`；Anthropic 流……）。

> 注意：Anthropic 脚本无 `message_stop`，按截断定义会标记
> `ai_stream_truncated=true`——若该断言与 TC-08 冲突，实现时给 Anthropic
> 脚本补 `event: message_stop` 收尾（本 TC 聚焦错误改写，不重复验证截断）。

## 清理

停止 `bfe` 进程与 mock 后端，删除临时目录。
