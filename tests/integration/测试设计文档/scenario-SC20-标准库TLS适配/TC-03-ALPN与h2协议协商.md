# TC-03 ALPN 与 h2 协议协商

## 目的

验证 `NextProtosConf` 的灰度逻辑（murmur3 hash 远端 IP 与 rate 比较）和全局 h2 开关经 `GetConfigForClient` 逐连接下发后，ALPN 协商结果正确，且 h2 连接上端到端 HTTP/2 请求可正常代理。

对应实现（Phase 6）：`TestTC03_ALPNH2Negotiation`。

## 前置条件

- `tls_rule_conf.data` 中 `example_product` 的 `NextProtos` 为
  `["h2;rate=100;isw=65535;mcs=200;level=0", "http/1.1"]`（h2 全量灰度）。
- BFE 已注册 `bfe_http2` next proto handler；mock 后端就绪。
- BFE 已启动。

## 执行步骤

1. 客户端 ALPN 列表 `[h2, http/1.1]` 发起 TLS 握手。
2. 检查 `ConnectionState.NegotiatedProtocol`，并在该连接上以 HTTP/2 发送 `GET /`。
3. 客户端仅声明 ALPN `[http/1.1]` 再次握手并发送 HTTP/1.1 请求。
4. 通过 monitor 接口 `GET /reload/tls_conf?enable=-h2` 关闭全局 h2 后，再次以 `[h2, http/1.1]` 握手。

## 预期结果

- 第 1 步协商结果为 `h2`；第 2 步收到 mock 后端 200 响应（证明 fork 删除后 h2 链路完整）。
- 第 3 步协商结果为 `http/1.1`，HTTP/1.1 请求 200。
- 第 4 步 h2 被全局开关过滤，协商回退 `http/1.1`。

## 补充说明

灰度 hash 的数值边界（rate=0/100）由 `bfe_server` 单元测试覆盖；本用例验证全量开关两条确定路径与真实 h2 转发。

## 清理

- 停止 BFE、关闭 mock 后端。
