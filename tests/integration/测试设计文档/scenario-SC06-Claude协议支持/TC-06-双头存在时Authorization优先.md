# TC-06 双头存在时 Authorization 优先

## 用例编号与名称

TC-06 双头存在时 Authorization 优先

## 所属场景

SC06 Claude 协议支持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证请求同时携带 `Authorization` 与 `x-api-key` 时，BFE 按 OpenAI 风格处理（`DetectAuthStyle` 优先识别 Authorization）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_both_protocols` 已启动，返回 200。
3. 临时 BFE 配置已生成并加载，`cluster_both_protocols` 配置 `ModelProtocols = ["openai", "anthropic"]`。

## 配置构造

- `cluster_both_protocols.AIConf`：
  - `ModelProtocols`: `["openai", "anthropic"]`
  - `Keys`: 多把 key（详见 TC-05）

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `both.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| x-api-key | `ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 响应状态码：200。
- upstream 收到的 `Authorization` 头携带集群 key（`Bearer sk-both-openai-key` 或 `Bearer sk-both-anthropic-key`）。
- upstream 收到的 `x-api-key` **为空**：客户端残留的凭证头在出站方向被删除（issue #1447；网关接管上游凭证时只保留本次注入的那一个凭证头）。
- upstream 未收到 `x-goog-api-key`。

## 清理

停止 `bfe` 进程、mock 后端，删除临时目录。
