# TC-04 启动态双向认证 ClientAuth

## 目的

验证启动配置即为 `ClientAuth=true`（`tls.RequireAndVerifyClientCert`，由旧布尔配置经自定义 UnmarshalJSON 映射）时，标准库在 `GetConfigForClient` 返回的克隆配置上执行双向认证：无证书拒绝、信任 CA 签发的证书放行、非信任 CA 证书拒绝；并分别覆盖 TLS 1.2 与 1.3 的可观察性差异。

对应实现（Phase 6）：`TestTC04_ClientAuthAtStartup`。

## 前置条件

- 测试代码现场生成客户端 CA `example_ca`（cert+key），放入 `tls_conf/client_ca/`。
- `tls_rule_conf.data` 中 SNI=`example.org` 规则为 `"ClientAuth": true, "ClientCAName": "example_ca"`（旧布尔字面量格式，验证兼容解析）。
- BFE 已启动。

## 执行步骤

1. **TLS 1.2 + 无客户端证书**：`MaxVersion=1.2`，`tls.Dial`。
2. **TLS 1.3 + 无客户端证书**：默认版本 `tls.Dial` 后在连接上发送 `GET /`。
3. **TLS 1.2/1.3 + 合法客户端证书**：用 `example_ca` 私钥现场签发客户端证书，分别握手并发送 HTTP 请求。
4. **非信任 CA 证书**：用另一套自签 CA 签发客户端证书，TLS 1.2 握手。

## 预期结果

- 第 1 步握手阶段直接失败（服务端在 Finished 前拒绝）。
- 第 2 步 `tls.Dial` 可能成功（TLS 1.3 服务端 Finished 先发），但首个 HTTP 请求失败（收到 certificate required alert 或连接关闭）。
- 第 3 步两种版本均握手成功且 HTTP 请求 200。
- 第 4 步握手失败：证书链不被 `ClientCAs` 信任。

## 补充说明

与 SC13 TC01 的区别：SC13 验证 reload 之后规则从 false 变 true；本用例规则启动即为 true，覆盖初始 `tlsConfLoad` 路径与旧布尔配置兼容。

## 清理

- 停止 BFE、关闭 mock 后端、删除临时 CA 与证书。
