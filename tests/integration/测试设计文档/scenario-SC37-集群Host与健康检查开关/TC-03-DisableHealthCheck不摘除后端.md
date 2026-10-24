# TC-03 DisableHealthCheck不摘除后端

## 用例编号与名称

TC-03 DisableHealthCheck不摘除后端

## 所属场景

SC37 集群 Host 与健康检查开关

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 DisableHealthCheck=true 时 5xx 不计入失败、不摘除后端、不启动健康检查。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_switch` 已启动，可按路径区分业务请求与 `/healthcheck`。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. 健康检查用例的 `CheckConf`：`FailNum=1`、`SuccNum=1`、`CheckInterval=100ms`、`Uri=/healthcheck`、`StatusCode=200`。

## 配置构造

- `ClusterBasic.DisableHealthCheck=true`、`BackendConf.OutlierDetectionHttpCode=5xx`。
- 业务路径返回 500，`/healthcheck` 返回 200。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1-3 | switch.example.org | /v1/chat/completions | 连续 3 次 | 业务均返回 500 |

## 预期结果

- 3 次请求均到达后端（业务命中 3 次）。
- `/healthcheck` 请求数为 0。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC03_*` 函数
