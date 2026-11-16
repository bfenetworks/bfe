# TC-11 /responses 不处理

## 用例编号与名称

TC-11 /responses 不处理

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证端点形态门禁：Responses API（`/v1/responses`）的推理内容是 `input` 数组里的独立 item，没有 `messages[].reasoning_content` 这样的注入落点。即使开关开启、协议匹配，也必须逐字节透传——硬塞字段会污染请求语义。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `cluster_ds` 的 `EnsureReasoningContent: ["openai"]`，`ModelProtocols: ["openai"]`；
2. mock 记录请求体并返回 200；
3. 请求体采用 Responses API 形态（`input` 数组，包含 `reasoning` item）。

## 配置构造

同 TC-02。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `front.example.org` | `/v1/responses` | `Bearer ak_front` | 见下 |

```json
{"model":"deepseek-chat","input":[{"role":"user","content":"q"},
 {"type":"reasoning","summary":[{"type":"summary_text","text":"prev"}]},
 {"role":"assistant","content":[{"type":"output_text","text":"a"}]}]}
```

## 后端响应

```json
{"id":"resp_1","object":"response","status":"completed",
 "output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
 "usage":{"input_tokens":10,"output_tokens":2}}
```

## 执行步骤

1. 起 BFE，发送请求；
2. 取 mock 收到的字节，与客户端发送的比对；
3. 读 `proxy_state` 指标。

## 预期结果

- 上游收到的请求体与客户端发送的**逐字节一致**（既未补 `reasoning_content`，也未改动 `input` 数组）；
- `ReasoningPatched=0`、`ReasoningSkip=0`（该端点在设计上不在范围内，不计入跳过）；
- 客户端 200。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录。
