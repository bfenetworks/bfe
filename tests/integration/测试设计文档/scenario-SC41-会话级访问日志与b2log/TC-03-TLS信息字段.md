# TC-03 TLS 信息字段

## 用例编号与名称

TC-03 TLS 信息字段

## 所属场景

SC41 会话级访问日志与 b2log 封装

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 HTTPS 会话日志记录 TLS 版本、加密套件与握手耗时。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_log` 已启动。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_logid`、`mod_access_pb3`。
4. `tls_conf` 使用 testdata 中的 `example.org` 证书，`SniConf=example.org`。

## 配置构造

- HTTPS 请求：SNI=`example.org`，客户端 `InsecureSkipVerify=true`；
- `SessionLog` 当前无 SNI 字段，`tls_cert_common_name` 未填充，只断言模块实际写入的字段。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | log.example.org | /v1/chat/completions | HTTPS GET（SNI=example.org） | TLS 会话 |

## 预期结果

- `SessionLog.proto=https`；
- `tls_version != 0`；
- `cipher_suite != 0`；
- `handshake_time` 已设置。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC03_*` 函数