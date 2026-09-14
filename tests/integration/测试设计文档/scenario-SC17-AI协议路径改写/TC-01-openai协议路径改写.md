# TC-01 openai 协议路径改写

## 用例编号与名称

TC-01 openai 协议路径改写

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：当前源码版本（含 `bfe_server/ai_path_rewrite.go` 与 `doSingleAIForward` 接入）

## 测试目的

验证 openai 协议的标准入口请求 `/v1/chat/completions` 被改写为 cluster `AIConf.ProtocolPaths["openai"]` 前缀 + 资源路径（百炼形态：`/compatible-mode/v1/chat/completions`），且 fallback cluster 零命中。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；`AIConf` 由 `BFEConfigBuilder` 直接写入生成的 `cluster_conf.data`（不经控制面）。

## 前置条件

1. 已编译 `bfe` 可执行文件（修改源码后需重新编译；缓存二进制按 git 版本键控，不感知工作区改动）。
2. mock 后端已启动：`cluster_primary` 返回 200；`cluster_fallback` 返回 200（本用例不命中）；`cluster_default` 返回 200（basic 路由引用，测试断言不命中）。
3. 路由：`ak_user_a` 命中 `default_t()` 规则 → `cluster_primary`，fallback → `cluster_fallback`。

## 配置构造

### cluster_primary.AIConf

```json
{
    "Type": 0,
    "ModelProtocols": ["openai", "anthropic"],
    "ProtocolPaths": {"openai": "/compatible-mode/v1"}
}
```

`cluster_fallback` 与 `cluster_default` 本用例不配置 `ProtocolPaths`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| 目标地址 | `http://127.0.0.1:<bfePort>/v1/chat/completions` |
| Host | `api.example.org` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}` |

## 改写计算

1. 协议识别：`Authorization: Bearer` → AuthStyle=openai；
2. 协议匹配：`ModelProtocols` 含 openai，放行；
3. 路径判定：`reqPath=/v1/chat/completions` 命中标准入口（`/v1/` 前缀）；
4. 改写：openai 语义为去 `/v1` 前缀追加——`upstream = /compatible-mode/v1 + /chat/completions`；
5. 改写结果写入 `outreq` 的私有 URL 拷贝，入站请求路径不被修改。

## 预期结果

| 断言 | 预期 |
|------|------|
| 响应状态码 | 200 |
| `cluster_primary` 后端收到的请求路径 | `[/compatible-mode/v1/chat/completions]` |
| `cluster_fallback` 命中数 | 0 |
