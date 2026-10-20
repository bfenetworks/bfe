# TC-06 双向TLS

## 用例编号与名称

TC-06 双向TLS

## 所属场景

SC38 HTTPS 后端 TLS

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 BFE 配置客户端证书后 mTLS 握手成功；未配置时被后端拒绝。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. HTTPS mock 后端 `cluster_tls` 已启动，证书由测试内生成的 CA 签发。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. `cluster_tls.BackendConf.Protocol=https`；`HTTPSConf.RSInsecureSkipVerify` 显式写入（避免 BFE 缺省 nil 解引用缺陷）。

## 配置构造

- 后端要求客户端证书（`RequireAndVerifyClientCert`，CA=测试 CA）。
- Arm 1：`BFECertFile/BFEKeyFile` 写入测试客户端证书（CN=bfe-client）；Arm 2：不配置。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | tls.example.org | /v1/chat/completions | Arm 1 | 带客户端证书 |
| 2 | tls.example.org | /v1/chat/completions | Arm 2 | 无客户端证书 |

## 预期结果

- Arm 1：HTTP 200，后端记录客户端证书 CN=`bfe-client`。
- Arm 2：网关 5xx（握手被拒）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC06_*` 函数
