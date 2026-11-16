# TC-05 CRL 吊销客户端证书

## 目的

验证 fork 内建的 CRL 吊销校验迁移为标准库 `tls.Config.VerifyConnection` 回调后仍然生效：被 CRL 列入吊销序列号的客户端证书在握手阶段被拒绝，未被吊销的证书正常放行。

对应实现（Phase 6）：`TestTC05_ClientCertCRLRevoked`。

## 前置条件

- `ClientAuth=true, ClientCAName=example_ca`（同 TC-04）。
- 测试代码现场生成 `example_ca`，并：
  1. 用 CA 签发两张客户端证书，序列号固定为 `serial-ok`（1001）与 `serial-revoked`（1002）；
  2. 用 CA 私钥签发一份 CRL，吊销列表仅含 `1002`，写入 `tls_conf/client_crl/example_ca/revoke.crl`（目录名与 `ClientCAName` 一致，由 `ClientCRLLoad` 约定）。
- BFE 已启动。

## 执行步骤

1. 持 `serial-ok` 客户端证书握手并发送 HTTP 请求。
2. 持 `serial-revoked` 客户端证书握手（TLS 1.2）。

## 预期结果

- 第 1 步握手成功、HTTP 请求 200。
- 第 2 步握手失败：`VerifyConnection` 回调返回吊销错误（错误信息含证书序列号），连接不进入请求处理。

## 补充说明

标准库在客户端证书链校验通过后调用 `VerifyConnection`，回调中遍历 `ConnectionState.PeerCertificates` 调 `CRLPool.CheckCertRevoked`；TLS 1.3 下拒绝同样在首个请求前完成（行为表现见 TC-04 第 2 步说明），故吊销用例固定使用 TLS 1.2 以获得确定性的握手级失败。

## 清理

- 停止 BFE、关闭 mock 后端、删除临时 CA/CRL/证书。
