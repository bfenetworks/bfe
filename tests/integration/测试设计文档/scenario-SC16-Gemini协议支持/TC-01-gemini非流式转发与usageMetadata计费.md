# TC-01 gemini 非流式转发与 usageMetadata 计费

## 用例编号与名称

TC-01 gemini 非流式转发与 usageMetadata 计费

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 gemini 风格请求（`x-goog-api-key` + `:generateContent` 路径）被正确路由到仅支持 `gemini` 协议的集群，上游收到 `x-goog-api-key` 注入的 cluster key、无 `Authorization` 头；非流式响应 `usageMetadata.totalTokenCount` 被解析并按 token 配额精确扣减一次。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 已编译含 gemini 协议支持的 `bfe` 可执行文件。
2. mock 后端 `cluster_gemini_only` 返回 200 与 gemini 非流式响应体：`usageMetadata{promptTokenCount:10, candidatesTokenCount:5, cachedContentTokenCount:3, totalTokenCount:15}`。
3. `cluster_gemini_only.AIConf`：`ModelProtocols=["gemini"]`，Key `goog-gemini-key`。
4. `ai_route.data` 中 `apikey_ak_user_a` 命中 `user_a-gemini`，target 为 `cluster_gemini_only`。
5. `mod_ai_token_auth` 配置 token 配额方案 `plan_token`（Unit=`total_token`，Redis Key `quota:plan_token`），初始配额 1000000。

## BFE 请求

POST `http://<bfe>/v1beta/models/gemini-2.5-flash:generateContent`，Host `gemini.example.org`：

| 字段 | 值 |
|------|----|
| x-goog-api-key | ak_user_a |
| Content-Type | application/json |
| body | `{"model":"gemini-2.5-flash","contents":[{"parts":[{"text":"hi"}]}]}` |

## 预期结果

1. 响应 200，响应体含 `"totalTokenCount":15`（透传）。
2. `cluster_gemini_only` 命中 1 次；上游收到 `x-goog-api-key: goog-gemini-key`，未收到 `Authorization`。
3. 异步扣款完成后（sleep 500ms），`quota:plan_token` 余额 = 1000000 - 15。
