# TC-02 gemini 流式按最后一个含 usage 的 chunk 计费

## 用例编号与名称

TC-02 gemini 流式按最后一个含 usage 的 chunk 计费

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 `streamGenerateContent` 流式响应：上游按 SSE 推送多个携带**累积** `usageMetadata` 的 chunk（totalTokenCount 11 → 13 → 15），无 SSE 终止事件；BFE 按**最后一个**含 usage 的 chunk 入账（15），而非中间 chunk（11/13）或累加值（39）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. mock 后端 `cluster_gemini_only` 以 SSE 模式推送 3 个 chunk（MockBackend.SSEEvents），每个 chunk 均带 `usageMetadata`，最终 chunk 含 `cachedContentTokenCount:4`、`totalTokenCount:15`。
2. 其余配置同 TC-01。

## BFE 请求

POST `http://<bfe>/v1beta/models/gemini-2.5-flash:streamGenerateContent`，Host `gemini.example.org`，头同 TC-01。

## 预期结果

1. 响应 200；客户端读到全部 3 个 chunk（透传完整）。
2. `cluster_gemini_only` 命中 1 次；上游收到 `x-goog-api-key: goog-gemini-key`，未收到 `Authorization`。
3. 异步扣款完成后，`quota:plan_token` 余额 = 1000000 - 15（按最后 chunk 入账）。
