# TC-01 带 path 热加载使用版本目录 client CA

## 目的

验证 conf-agent#19 的核心修复：`/reload/tls_conf?path=tls_conf_<version>` 时，client CA 从版本目录加载，而不是从激活的 `tls_conf` 目录加载。

对应实现：`TestTC01_ReloadWithPathUsesVersionDirClientCA`（`scenario-SC13-tls-conf-reload-path/sc13_tls_conf_reload_path_test.go`）。

## 前置条件

- BFE 已启动，激活 `tls_conf/tls_rule_conf.data` 为 `ClientAuth=false`。
- 激活目录 `tls_conf/client_ca/example_ca.crt` 已被删除（模拟线上死锁现场：激活目录停留在旧版本）。

## 执行步骤

1. 基线校验：不带客户端证书的 TLS 握手成功（`ClientAuth=false`）。
2. 测试代码生成版本目录 `tls_conf_20260904205703`：完整复制激活 `tls_conf` 树；`tls_rule_conf.data` 改写为 `ClientAuth=true, ClientCAName=example_ca`；`client_ca/example_ca.crt` 只写入版本目录。
3. 调用 `GET /reload/tls_conf?path=tls_conf_20260904205703`。
4. 不带客户端证书再次握手。
5. 用模板中的 `example_ca` 私钥现场签发客户端证书，带证书握手。

## 预期结果

- 第 3 步返回 `{"error":null}`。**修复前**此步返回 `ClientCALoad` 错误（在激活目录找不到 `example_ca.crt`），conf-agent 因此永不切换符号链接。
- 第 4 步握手失败：服务端已按新规则要求客户端证书。
- 第 5 步握手成功：服务端从版本目录的 `client_ca/example_ca.crt` 校验客户端证书。

## 补充说明

该用例同时证明 reload 后的 TLS 规则真正生效（不只是接口返回成功）：握手行为从"不请求客户端证书"变为"要求并校验客户端证书"。

## 清理

- 停止 BFE、关闭 mock 后端。
