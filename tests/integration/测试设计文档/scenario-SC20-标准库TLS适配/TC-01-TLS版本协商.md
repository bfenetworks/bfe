# TC-01 TLS 版本协商（1.3 默认 / 1.2 兼容 / 1.0-1.1 拒绝）

## 目的

验证删除 fork 后 TLS 版本边界与设计一致：默认协商 TLS 1.3；仅支持 1.2 的客户端仍可协商 1.2；TLS 1.0/1.1 被标准库移除，握手在协议层失败。

对应实现（Phase 6）：`TestTC01_TLSVersionNegotiation`。

## 前置条件

- BFE 已启动，https 端口就绪，默认服务器证书有效。
- `bfe.conf` 不显式设置 TLS 版本上下限（使用标准库默认）。

## 执行步骤

1. 默认客户端（不设 MaxVersion/MinVersion）发起 TLS 握手并发送一次 HTTP 请求。
2. 客户端锁定 `MaxVersion=tls.VersionTLS12` 再次握手。
3. 客户端锁定 `MaxVersion=tls.VersionTLS11` 握手。
4. 客户端锁定 `MaxVersion=tls.VersionTLS10` 握手。

## 预期结果

- 第 1 步握手成功，`ConnectionState.Version == tls.VersionTLS13`，HTTP 请求 200。
- 第 2 步握手成功，`Version == tls.VersionTLS12`，HTTP 请求 200。
- 第 3、4 步 `tls.Dial` 返回错误（`no supported versions satisfy MinVersion and MaxVersion` 或服务端版本不兼容告警），连接建立失败。

## 补充说明

旧 fork 最高只支持 TLS 1.2，标准库最低支持 TLS 1.2；该用例是版本边界的双向锁定。

## 清理

- 停止 BFE、关闭 mock 后端。
