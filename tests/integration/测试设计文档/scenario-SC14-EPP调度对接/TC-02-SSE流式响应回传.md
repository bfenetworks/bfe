# TC-02 SSE 流式响应回传

## 目的

验证响应路径：对 `stream: true` 请求，BFE 把响应头与流式响应体分块回传 mock EPP（重组后内容完整、以 EndOfStream 终结），同时客户端收到完整 SSE 流。

对应实现：`TestTC02_SSEResponseBodyRelay`。

## 前置条件

- 同 TC-01。

## 执行步骤

1. 发送 `stream: true` 的 chat completion 请求（标记串 `hello-epp-tc02`）。
2. 断言客户端收到的响应为 SSE 流。
3. 读取 mock EPP 的响应头/响应体记录。

## 预期结果

- 响应 200，body 含 `data:`（SSE 帧）。
- mock EPP `ResponseHeadersList()[0]` 的 `Content-Type` 含 `text/event-stream`。
- mock EPP `ResponseBodies()[0]` 含 `data:` 内容，长度不小于客户端收到的 1/2（无截断；块重组完整且 EOS 送达）。

## 清理

- 同 TC-01。
