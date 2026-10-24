# TC-01 RSHost与RSCAList握手成功

## 用例编号与名称

TC-01 RSHost与RSCAList握手成功

## 所属场景

SC38 HTTPS 后端 TLS

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 RSHost 决定 SNI 与校验名，配合 RSCAList 完成握手。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. HTTPS mock 后端 `cluster_tls` 已启动，证书由测试内生成的 CA 签发。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. `cluster_tls.BackendConf.Protocol=https`；`HTTPSConf.RSInsecureSkipVerify` 显式写入（避免 BFE 缺省 nil 解引用缺陷）。

## 配置构造

- 服务端证书 SAN=`backend.example.org`，由测试 CA 签发。
- `HTTPSConf`：`RSHost=backend.example.org`、`RSCAList=[测试 CA]`。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | tls.example.org | /v1/chat/completions | 普通 POST | 经 BFE 转发到 HTTPS 后端 |

## 预期结果

- HTTP 200；后端握手记录 SNI=`backend.example.org`。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC01_*` 函数
