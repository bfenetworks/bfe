# TC-04 fallback 路径重算

## 用例编号与名称

TC-04 fallback 路径重算（专利场景）

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 route 级 fallback 切换到不同 `ProtocolPaths` 配置的 cluster 时，第二次 attempt 从**原始客户端路径**按 fallback cluster 自身的配置重算，而不是继承第一次 attempt 的改写结果。

这是 rewrite 特性的正确性关键：`applyAIProtocolPathRewrite` 只改写 `outreq` 的私有 URL 拷贝、永不修改入站请求，因此每个 attempt 的 `*outreq = *req` 拷贝都携带原始路径。本用例用两个差异显著的前缀（百炼 `/apps/anthropic` vs Kimi Code `/coding`）把"继承错误"变成可观测的路径断言。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

| cluster | AIConf.ProtocolPaths | 后端行为 |
|---------|----------------------|----------|
| cluster_primary | `{"anthropic": "/apps/anthropic"}` | 返回 503，触发 fallback |
| cluster_fallback | `{"anthropic": "/coding"}` | 返回 200 |

两个 cluster 均声明 `ModelProtocols: ["openai", "anthropic"]`。

## BFE 请求

同 TC-02（anthropic 请求 `/v1/messages`）。

## 改写计算

| attempt | cluster | 计算 | 结果 |
|---------|---------|------|------|
| 第 1 次 | cluster_primary | `/apps/anthropic` + `/v1/messages` | `/apps/anthropic/v1/messages`（503，fallback） |
| 第 2 次 | cluster_fallback | 原始 `/v1/messages` → `/coding` + `/v1/messages` | `/coding/v1/messages` |

若改写污染入站请求，第 2 次 attempt 的 `reqPath` 将是 `/apps/anthropic/v1/messages`——不命中标准入口判定，落入透传，fallback 后端会收到错误的路径。

## 预期结果

| 断言 | 预期 |
|------|------|
| 响应状态码 | 200 |
| `cluster_primary` 命中数 / 路径 | 1；`/apps/anthropic/v1/messages` |
| `cluster_fallback` 命中数 / 路径 | 1；`/coding/v1/messages`（**不得**为 `/apps/anthropic/v1/messages`） |
