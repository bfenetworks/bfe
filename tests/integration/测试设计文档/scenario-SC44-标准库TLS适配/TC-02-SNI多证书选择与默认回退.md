# TC-02 SNI 多证书选择与默认回退

## 目的

验证 fork 的 MultiCert 能力由标准库 `tls.Config.GetCertificate` 接管后，服务器证书仍按 **VIP→SNI→默认** 的顺序选取；无 VIP（未启用 PROXY 协议）时以 SNI 命中，未知 SNI 回退默认证书。

对应实现（Phase 6）：`TestTC02_MultiCertSNISelection`。

## 前置条件

- 测试代码现场生成一套 CA 与两张服务器证书：
  - `cert-a`：DNSName=`example.org`；
  - `cert-b`：DNSName=`example.net`。
- `server_cert_conf.data` 配置两张证书（`CertName` 分别为 `example.org`、`example.net`），`tls_rule_conf.data` 为两个 SNI 配置规则；默认证书为 `cert-a`。
- BFE 已启动。

## 执行步骤

1. SNI=`example.org` 握手，检查服务端返回证书。
2. SNI=`example.net` 握手，检查服务端返回证书。
3. SNI=`unknown.example`（规则中不存在）握手。

## 预期结果

- 第 1 步 `PeerCertificates[0].DNSNames` 含 `example.org`。
- 第 2 步 `PeerCertificates[0].DNSNames` 含 `example.net`，证明逐连接动态选证生效（非单证书监听器）。
- 第 3 步握手成功（`InsecureSkipVerify`），返回默认证书 `cert-a`；不发生 TLS 告警断连。

## 补充说明

VIP 维度依赖 PROXY 协议注入 VirtualAddr，单组件集成环境不构造，由 `bfe_server` 单元测试覆盖；本用例固定验证 SNI 与默认两级。

## 清理

- 停止 BFE、关闭 mock 后端、删除临时证书。
