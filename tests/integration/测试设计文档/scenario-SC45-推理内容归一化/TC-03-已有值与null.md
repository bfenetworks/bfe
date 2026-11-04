# TC-03 已有值与 null

## 用例编号与名称

TC-03 已有值与 null

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证字段条件的两条边界：已有非空 `reasoning_content` 一律不动；值为 `null` 归一为空串。前者保证不覆盖上游真实思考内容（否则会丢失上下文），后者覆盖"上游末帧发 `null`、客户端原样回传"这一常见形态。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `cluster_ds` 的 `EnsureReasoningContent: ["openai"]`；
2. 同一 BFE 实例内顺序发两次请求（两个臂），mock 分别记录请求体。

## 配置构造

同 TC-02。

## BFE 请求

| 步骤 | 臂 | Body（窗口内 assistant 部分） |
|------|----|------------------------------|
| 1 | A：已有非空值 | `{"role":"assistant","reasoning_content":"real thinking","content":"a"}` |
| 2 | B：值为 null | `{"role":"assistant","reasoning_content":null,"content":"a"}` |

两臂的完整请求体均为：`{"model":"deepseek-chat","messages":[{"role":"user","content":"q"},<上述 assistant>]}`

## 后端响应

同 TC-01（200）。

## 执行步骤

1. 启动 BFE；
2. 发送臂 A，取 mock 记录的请求体；
3. 发送臂 B，取 mock 记录的请求体；
4. 断言两臂的字段值与字节差异。

## 预期结果

- 臂 A：请求体与客户端发送的**逐字节一致**，`reasoning_content` 仍为 `real thinking`；`ReasoningPatched` 不增加；
- 臂 B：`messages[1].reasoning_content == ""`，且这是唯一差异——把 `"reasoning_content":""` 换回 `"reasoning_content":null` 后与原始请求体逐字节相等（值被**原地替换**，不是新增字段）；`ReasoningPatched` +1；
- 两臂客户端均收到 200。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录。
