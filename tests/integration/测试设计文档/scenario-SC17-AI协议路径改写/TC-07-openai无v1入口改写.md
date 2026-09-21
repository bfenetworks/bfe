# TC-07 openai 无 /v1 入口改写（issue #1379）

## 用例编号与名称

TC-07 openai 无 /v1 入口改写

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：含 `bfe_server/ai_path_rewrite.go` 无 `/v1` 入口兼容改写的版本（issue #1379 修复后）

## 测试目的

验证 openai 协议的**无 `/v1` 前缀入口**（Trae 等直连 `base_url` 的 OpenAI 兼容客户端发送的 `POST /chat/completions`）与带 `/v1` 入口被改写为**相同**的上游路径——客户端入口带不带 `/v1` 不影响最终上游路径（provider path = 上游 API 基路径语义）。

issue #1379 现象：Trae 调用百炼 glm-5.2 发送 `POST /chat/completions`，旧实现仅改写 `/v1` 或 `/v1/...` 入口，配置不生效，原样转发 `/chat/completions` 到上游返回 404。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；`AIConf` 由 `BFEConfigBuilder` 直接写入生成的 `cluster_conf.data`（不经控制面）。

## 前置条件

1. 已编译 `bfe` 可执行文件（修改源码后需重新编译；缓存二进制按 git 版本键控 + 源码 mtime 感知工作区改动）。
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

依次发送 2 次 POST 请求：

| 字段 | 值 |
|------|-----|
| 目标地址 | `http://127.0.0.1:<bfePort>/chat/completions`，随后 `http://127.0.0.1:<bfePort>/v1/chat/completions` |
| Host | `api.example.org` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}` |

## 改写计算

1. 协议识别：`Authorization: Bearer` → AuthStyle=openai（两条请求一致）；
2. 协议匹配：`ModelProtocols` 含 openai，放行；
3. 路径判定：
   - `/chat/completions`：无 `/v1` 前缀，剥离步骤恒等，命中 OpenAI 标准端点 `/chat/completions`；
   - `/v1/chat/completions`：剥离 `/v1` 前缀后同样命中 `/chat/completions`；
4. 改写：`upstream = /compatible-mode/v1 + /chat/completions`，**两条请求的改写结果相同**；
5. 改写结果写入 `outreq` 的私有 URL 拷贝，入站请求路径不被修改。

## 预期结果

| 断言 | 预期 |
|------|------|
| 响应状态码 | 200（两次） |
| `cluster_primary` 后端收到的请求路径 | `[/compatible-mode/v1/chat/completions, /compatible-mode/v1/chat/completions]` |
| `cluster_fallback` 命中数 | 0 |
