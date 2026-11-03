# TC-05 anthropic 分支

## 用例编号与名称

TC-05 anthropic 分支

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现；anthropic 分支实现但默认不启用）

## 测试目的

验证 anthropic 协议的等价动作：在窗口内 assistant 的 `content` 数组**头部**插入空 thinking 块 `{"type":"thinking","thinking":"","signature":""}`；已有 thinking 块的 assistant 不重复插入；原有 content 元素保持原字节。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `cluster_ds`：`ModelProtocols: ["openai","anthropic"]`、`EnsureReasoningContent: ["openai","anthropic"]`；
2. mock 按 `/anthropic/v1/messages` 路由并记录请求体；
3. 只发 `x-api-key`，不带 `Authorization`（两个头同时存在会被识别为 openai，见 TC-04 前置条件 4）；
4. 是否接受空 `signature` 属上游契约问题——本用例只断言网关侧字节产物，不断言上游接受度（验收见 `design-changes.md` 的 8.4 遗留项）。

## 配置构造

```json
"EnsureReasoningContent": ["openai", "anthropic"]
```

## BFE 请求

| 步骤 | 臂 | Body（`messages` 部分） |
|------|----|------------------------|
| 1 | A：无 thinking 块 | `[{"role":"user","content":"q"},{"role":"assistant","content":[{"type":"text","text":"a"}]}]` |
| 2 | B：已有 thinking 块 | `[{"role":"user","content":"q"},{"role":"assistant","content":[{"type":"thinking","thinking":"real","signature":"sig"},{"type":"text","text":"a"}]}]` |

Headers 同 TC-04（`x-api-key` + `anthropic-version`）。

## 后端响应

同 TC-04（anthropic 形态 200）。

## 执行步骤

1. 起 BFE，发送臂 A，取 mock 记录的请求体；
2. 发送臂 B，取 mock 记录的请求体；
3. 断言注入位置与原有元素。

## 预期结果

- 臂 A：
  - `messages[1].content[0].type == "thinking"`、`thinking == ""`、`signature == ""`；
  - `content[1]` 仍为原来的 `{"type":"text","text":"a"}`（原元素字节未变，只是整体右移一位）；
  - `ReasoningPatched` +1；
- 臂 B：请求体与客户端发送的**逐字节一致**（已有 thinking 块，不重复插入），`ReasoningPatched` 不增加；
- 两臂客户端均 200。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录。
