# TC-06 压缩体与非 JSON 门禁

## 用例编号与名称

TC-06 压缩体与非 JSON 门禁

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证两条形态门禁，确保补丁不会在"字节不是声明媒体类型"的输入上工作：请求体带 `Content-Encoding`（gzip）时跳过；`Content-Type` 显式声明非 JSON 时跳过。跳过意味着请求体**原样到达上游**且计数 `ReasoningSkip`，绝不能出现"改写后上游解析失败"。

## 运行模式

单组件模式：真实 `bfe` + 记录原始字节的 mock 上游。

## 前置条件

1. `cluster_ds` 的 `EnsureReasoningContent: ["openai"]`；
2. mock 记录请求体的**原始字节**（不解压），并返回 200；
3. 臂 A 的请求体由测试代码 gzip 压缩后发送。

## 配置构造

同 TC-02。

## BFE 请求

| 步骤 | 臂 | Headers | Body |
|------|----|---------|------|
| 1 | A：压缩体 | `Content-Type: application/json`、`Content-Encoding: gzip` | 场景说明 §4.4 样张的 gzip 压缩字节 |
| 2 | B：非 JSON 类型 | `Content-Type: application/x-www-form-urlencoded` | 同上样张的明文（故意声明错媒体类型） |

## 后端响应

同 TC-01（200）。

## 执行步骤

1. 起 BFE；
2. 发送臂 A，取 mock 记录的原始字节，与客户端发送的压缩字节比对；
3. 发送臂 B，取 mock 记录的字节，与客户端发送的字节比对；
4. 读 `proxy_state` 指标。

## 预期结果

- 臂 A：上游收到的字节与客户端发送的 gzip 字节**逐字节一致**；`ReasoningSkip` +1；`ReasoningPatched` 不增加；客户端 200；
- 臂 B：上游收到的字节与客户端发送的明文**逐字节一致**（未插入任何字段）；`ReasoningSkip` +1；
- 两臂均**不得出现 5xx**（这是本条门禁要防的主要事故）。

## 清理

停止 `bfe` 进程与 mock 上游，删除临时目录。
