# TC-02 DisableHostHeader改写为后端地址

## 用例编号与名称

TC-02 DisableHostHeader改写为后端地址

## 所属场景

SC37 集群 Host 与健康检查开关

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 DisableHostHeader=true 时后端 Host 回落为后端地址。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_switch` 已启动，可按路径区分业务请求与 `/healthcheck`。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. 健康检查用例的 `CheckConf`：`FailNum=1`、`SuccNum=1`、`CheckInterval=100ms`、`Uri=/healthcheck`、`StatusCode=200`。

## 配置构造

- `ClusterBasic.DisableHostHeader=true`。
- 后端返回 200。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | switch.example.org | /v1/chat/completions | `Host: switch.example.org` | 普通请求 |

## 预期结果

- HTTP 200；后端记录 `Host=127.0.0.1:<backend port>`。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC02_*` 函数
