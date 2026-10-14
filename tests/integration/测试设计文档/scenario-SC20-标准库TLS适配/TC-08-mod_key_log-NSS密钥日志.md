# TC-08 mod_key_log NSS 密钥日志（P1）

## 目的

验证 `mod_key_log` 从 fork 私有钩子改为标准库 `tls.Config.KeyLogWriter` 后，在握手阶段输出 NSS 标准格式密钥日志，可被 Wireshark 等工具直接使用；旧 `mod_key_log.conf` 的废弃字段（products 白名单）解析不报错但被忽略。

对应实现（Phase 6）：`TestTC08_KeyLogWriterNSSFormat`。

## 前置条件

- `Modules` 加载 `mod_key_log`；`mod_key_log/mod_key_log.conf` 指定密钥日志文件路径（位于测试临时 log 目录），并保留旧版 products 配置项。
- BFE 已启动。

## 执行步骤

1. 发起 TLS 1.3 握手（任意 HTTP 请求后关闭）。
2. 发起 TLS 1.2 握手（客户端 `MaxVersion=1.2`）。
3. 读取密钥日志文件。

## 预期结果

- 文件存在且非空：
  - TLS 1.3 连接产生以 `CLIENT_HANDSHAKE_TRAFFIC_SECRET`（及 `SERVER_HANDSHAKE_TRAFFIC_SECRET`、`CLIENT_TRAFFIC_SECRET_0` 等）开头的行；
  - TLS 1.2 连接产生以 `CLIENT_RANDOM` 开头的行。
- 每行格式为 `<标签> <ClientRandom 64hex> <secret hex>`；写入时机在握手期间（标准库回调），而非旧实现的请求结束时。
- BFE 日志对废弃 products 字段输出一次忽略类 WARN，不影响加载。

## 补充说明

列为 P1：该用例依赖文件系统落盘与模块加载顺序，行为本身由标准库保证；若 Phase 6 实施成本过高，可降级为 `mod_key_log` 单元测试覆盖，集成层只验证模块加载后 BFE 可正常启动与握手。

## 清理

- 停止 BFE、关闭 mock 后端、删除临时密钥日志文件（密钥材料不得提交到仓库）。
