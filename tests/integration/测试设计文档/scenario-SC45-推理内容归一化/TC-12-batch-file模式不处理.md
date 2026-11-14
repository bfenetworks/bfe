# TC-12 batch/file 模式不处理

## 用例编号与名称

TC-12 batch/file 模式不处理

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证 batch/file 透传模式不被缓冲、不被改写：该模式下请求体可能很大且流式透传（`mod_ai_batch`），网关不得为做归一化而整体缓冲或改写它。跳过应计 `ReasoningSkip`。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `cluster_ds` 的 `EnsureReasoningContent: ["openai"]`；
2. `mod_ai_batch` 已加载（batch/file 端点识别依赖它）；
3. mock 记录请求体字节并返回 200。

## 配置构造

同 TC-02。

## BFE 请求

| 步骤 | 臂 | Path | Content-Type | Body |
|------|----|------|--------------|------|
| 1 | A：file 上传 | `/v1/files` | `multipart/form-data; boundary=<B>` | 多部分表单（`purpose=batch` + 一个 `file` part，内含含 `messages` 字段的 JSONL 文本） |
| 2 | B：batch 创建 | `/v1/batches` | `application/json` | `{"input_file_id":"file-1","endpoint":"/v1/chat/completions","completion_window":"24h"}` |

## 后端响应

臂 A：`{"id":"file-1","object":"file","purpose":"batch"}`；臂 B：`{"id":"batch-1","status":"validating"}`。

## 执行步骤

1. 起 BFE，发送臂 A，取 mock 收到的字节；
2. 发送臂 B，取 mock 收到的字节；
3. 读 `proxy_state` 指标。

## 预期结果

- 两臂上游收到的字节与客户端发送的**逐字节一致**（含 multipart boundary 与 JSONL 内容原样，未被补任何字段）；
- `ReasoningSkip` 每臂 +1（file/batch 显式计入跳过）；`ReasoningPatched=0`；
- 两臂客户端均 200；无 5xx。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录。
