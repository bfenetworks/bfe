# TC-07 TLS失败基线

## 用例编号与名称

TC-07 TLS失败基线

## 所属场景

SC38 HTTPS 后端 TLS

## 版本声明

- `bfe`：当前源码版本

## 测试目的

固定 TLS 失败的处理基线：快速失败、后端 handler 不被调用、进程继续服务。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. HTTPS mock 后端 `cluster_tls` 已启动，证书由测试内生成的 CA 签发。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. `cluster_tls.BackendConf.Protocol=https`；`HTTPSConf` 不写 `RSInsecureSkipVerify`（缺省按 `false` 校验证书）。

## 配置构造

- 服务端证书由 CA-A 签发，`RSCAList=[CA-B]`（无关 CA）。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | tls.example.org | /v1/chat/completions | 第 1 次 | 握手失败 |
| 2 | tls.example.org | /v1/chat/completions | 第 2 次 | 再次失败 |

## 预期结果

- 两次均返回网关 5xx，总耗时 < 10s。
- 后端业务命中数为 0（握手未完成）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC07_*` 函数
