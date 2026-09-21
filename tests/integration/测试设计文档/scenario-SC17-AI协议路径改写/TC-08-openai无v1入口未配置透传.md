# TC-08 openai 无 /v1 入口未配置透传（issue #1379）

## 用例编号与名称

TC-08 openai 无 /v1 入口未配置透传

## 所属场景

SC17 AI 协议路径改写（AIConf.ProtocolPaths）

## 版本声明

- `bfe`：含 `bfe_server/ai_path_rewrite.go` 无 `/v1` 入口兼容改写的版本（issue #1379 修复后）

## 测试目的

验证**未配置 `ProtocolPaths`** 时，openai 协议的带 `/v1` 与不带 `/v1` 入口均**逐字节透传**（issue #1379 预期表第 3、4 行）。防止"为兼容无 `/v1` 入口而引入的端点识别"在未配置场景产生任何行为漂移——透传是默认且唯一的未配置行为。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；`AIConf` 由 `BFEConfigBuilder` 直接写入生成的 `cluster_conf.data`（不经控制面）。

## 前置条件

1. 已编译 `bfe` 可执行文件（修改源码后需重新编译；缓存二进制按 git 版本键控 + 源码 mtime 感知工作区改动）。
2. mock 后端已启动：`cluster_primary` 返回 200；`cluster_fallback` 返回 200（本用例不命中）；`cluster_default` 返回 200（basic 路由引用，测试断言不命中）。
3. 路由：`ak_user_a` 命中 `default_t()` 规则 → `cluster_primary`，fallback → `cluster_fallback`。

## 配置构造

`cluster_primary` / `cluster_fallback` / `cluster_default` 均不配置 `ProtocolPaths`（`AIConf` 为 nil）。

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
3. 路径判定：`AIConf.ProtocolPaths` 无 openai 条目 → 未配置该协议 → **透传分支**，入口是否带 `/v1` 均不参与计算。

## 预期结果

| 断言 | 预期 |
|------|------|
| 响应状态码 | 200（两次） |
| `cluster_primary` 后端收到的请求路径 | `[/chat/completions, /v1/chat/completions]`（原样） |
| `cluster_fallback` 命中数 | 0 |
