# TC-02 anthropic 协议路径改写

## 用例编号与名称

TC-02 anthropic 协议路径改写

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 anthropic 协议请求 `/v1/messages`（`x-api-key` 认证头）被改写为 `{ProtocolPaths["anthropic"]}/v1/messages`（百炼形态：`/apps/anthropic/v1/messages`）。anthropic 语义是**追加完整路径**（Anthropic SDK 的 base_url 不含 `/v1`，自行拼接 `/v1/messages`），与 openai 的去前缀语义不同，本用例锁定该差异。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

同 TC-01。`cluster_primary.AIConf` 本用例为：

```json
{
    "Type": 0,
    "ModelProtocols": ["openai", "anthropic"],
    "ProtocolPaths": {"anthropic": "/apps/anthropic"}
}
```

> `ModelProtocols` 必须显式声明 `anthropic`：空 `ModelProtocols` 默认仅 openai，anthropic 请求会在协议匹配处被 `PROVIDER_PROTOCOL_MISMATCH` 拒绝。

## BFE 请求

| 字段 | 值 |
|------|-----|
| 目标地址 | `http://127.0.0.1:<bfePort>/v1/messages` |
| Host | `api.example.org` |
| x-api-key | `ak_user_a` |
| Body | `{"model":"test-model","messages":[...],"max_tokens":1}` |

## 改写计算

1. 协议识别：`x-api-key` → AuthStyle=anthropic；
2. 协议匹配：`ModelProtocols` 含 anthropic，放行；
3. 路径判定：`/v1/messages` 命中标准入口；
4. 改写：anthropic 语义为追加完整路径——`upstream = /apps/anthropic + /v1/messages`。

## 预期结果

| 断言 | 预期 |
|------|------|
| 响应状态码 | 200 |
| `cluster_primary` 后端收到的请求路径 | `[/apps/anthropic/v1/messages]` |
| `cluster_fallback` 命中数 | 0 |
