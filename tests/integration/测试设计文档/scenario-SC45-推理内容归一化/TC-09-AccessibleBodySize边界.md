# TC-09 AccessibleBodySize 边界

## 用例编号与名称

TC-09 AccessibleBodySize 边界

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证超大请求的盲区处理：请求体超过 `Server.AccessibleBodySize` 时，网关只能拿到前缀字节，**必须跳过改写**并计 `ReasoningSkip`，不得对截断的 body 做任何写入。同一实例下用小 body 做对照臂，证明跳过只由大小触发、不是开关失效。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `bfe.conf` 配 `AccessibleBodySize = 4096`（远小于默认 2MB，便于构造超限请求）；
2. `cluster_ds` 的 `EnsureReasoningContent: ["openai"]`；
3. mock 记录请求体字节。

## 配置构造

```json
"AIConf": {
    "ModelProtocols": ["openai", "anthropic"],
    "EnsureReasoningContent": ["openai"],
    "Keys": [{"Name": "key-primary", "Key": "sk-upstream-secret", "Weight": 100}]
}
```

## BFE 请求

| 步骤 | 臂 | Body |
|------|----|------|
| 1 | A：超限（>4KB） | `messages[0].content` 填充约 8KB 字符串的样张（窗口内 assistant 缺字段） |
| 2 | B：未超限（<4KB） | 场景说明 §4.4 样张 |

## 后端响应

同 TC-01（200）。

## 执行步骤

1. 起 BFE（带小 `AccessibleBodySize`）；
2. 发送臂 A，取 mock 收到的字节，检查是否含 `reasoning_content`；读 `ReasoningSkip`；
3. 发送臂 B，取 mock 收到的字节；读 `ReasoningPatched`。

## 预期结果

- 臂 A：上游收到的字节与客户端发送的**逐字节一致**（无任何注入，即使窗口内 assistant 缺字段）；`ReasoningSkip` +1；客户端 200，**不得出现 5xx**；
- 臂 B：`messages[1].reasoning_content == ""`；`ReasoningPatched` +1；
- 两臂区分明确：跳过由大小触发，而非开关或协议门禁。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录；恢复默认 `AccessibleBodySize`（临时目录销毁即恢复）。
