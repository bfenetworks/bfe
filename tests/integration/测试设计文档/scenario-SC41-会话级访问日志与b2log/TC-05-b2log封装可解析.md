# TC-05 b2log 封装可解析

## 用例编号与名称

TC-05 b2log 封装可解析

## 所属场景

SC41 会话级访问日志与 b2log 封装

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证日志文件的 b2log 封装格式合法，记录载荷可反序列化为 `BfeLog`。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_log` 已启动。
3. `bfe.conf`：`EnableAiGateway=false`，`Modules = mod_logid`、`mod_access_pb3`。
4. `mod_access_pb3.conf`：`LogPrefix=pb_access3`、`LogDir=../log`。

## 配置构造

- 产生一条 HTTP 会话与一条 HTTPS 会话后停止 BFE；
- 直接读取 `log/pb_access3.log`，按 `b2log` 头部（HEADER_SIZE=24，LittleEndian）手工解析。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | log.example.org | /v1/chat/completions | HTTP GET | 产生请求/会话记录 |
| 2 | log.example.org | /v1/chat/completions | HTTPS GET（SNI=example.org） | 产生 TLS 会话记录 |

## 预期结果

- 每条记录：`magic=0xB0AEBEA7`、`version=1`、`compress_len=0`、`uncompress_len` 合法（1..MAX_RECORD_LEN）。
- 载荷可 `proto.Unmarshal` 为 `BfeLog`，`log_type` 为 `Request` 或 `Session`。
- 至少各 1 条会话记录与请求记录；文件解析结束后无残留字节。

## 说明

- b2log 当前不压缩记录，载荷长度取 `UnCompressLen`（`CompressLen` 必须为 0）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC05_*` 函数