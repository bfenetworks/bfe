# TC-07 value 2048 截断

## 目的

验证超长 header 值按下游 Doris 列宽截断至 2048 字节：行内其余字段完整，Routine Load 导入契约不破。

## 前置条件

- `mod_access_pb.data`：`req_headers` 含 `x-big`，`res_headers` 含 `x-trace-id`；
- mock 后端注入 `X-Trace-Id`（正常长度）。

## 请求构造

- 请求头 `X-Big: <3000 字节随机可见字符>`；
- 客户端 token：`ak_hdr_a`。

## 执行步骤

1. 发送请求 → 200；
2. 解码 b2log 取 `RequestLog`。

## 预期结果

- `req_headers` 中 `X-Big` 的值长度恰为 2048 字节；
- 同一条日志的状态码、token 字段、`res_headers` 均完整正常；
- 无截断相关错误日志（截断为静默语义，设计如此）。

## 清理

测试结束时关闭 BFE 与 mock 后端。
