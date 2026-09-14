# TC-03 双协议 cluster 独立改写

## 用例编号与名称

TC-03 双协议 cluster 独立改写

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证同一 cluster 同时配置两个协议的 `ProtocolPaths` 时（Kimi Code 会员形态：openai 在 `/coding/v1`、anthropic 在 `/coding`），两种协议的请求各自按本协议前缀独立改写、互不串扰。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

`cluster_primary.AIConf`：

```json
{
    "Type": 0,
    "ModelProtocols": ["openai", "anthropic"],
    "ProtocolPaths": {"openai": "/coding/v1", "anthropic": "/coding"}
}
```

（键名与 BFE 导出配置一致：`AIConf` 字段无 JSON tag，按 Go 字段名序列化。）

## BFE 请求

按序发送 2 次请求（Host 均为 `api.example.org`）：

| 序号 | 路径 | 认证头 | 协议 |
|------|------|--------|------|
| 1 | `/v1/chat/completions` | `Authorization: Bearer ak_user_a` | openai |
| 2 | `/v1/messages` | `x-api-key: ak_user_a` | anthropic |

## 改写计算

| 序号 | 计算 | 结果 |
|------|------|------|
| 1 | `/coding/v1` + `/chat/completions` | `/coding/v1/chat/completions` |
| 2 | `/coding` + `/v1/messages` | `/coding/v1/messages` |

## 预期结果

| 断言 | 预期 |
|------|------|
| 两次响应状态码 | 200 |
| `cluster_primary` 后端收到的路径序列 | `[/coding/v1/chat/completions, /coding/v1/messages]`（顺序一致） |
| `cluster_fallback` 命中数 | 0 |
