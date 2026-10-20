# TC-04 DisableHealthCheck摘除并探测

## 用例编号与名称

TC-04 DisableHealthCheck摘除并探测

## 所属场景

SC37 集群 Host 与健康检查开关

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 DisableHealthCheck=false 时达到 FailNum 后摘除后端并启动主动健康检查。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_switch` 已启动，可按路径区分业务请求与 `/healthcheck`。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. 健康检查用例的 `CheckConf`：`FailNum=1`、`SuccNum=1`、`CheckInterval=100ms`、`Uri=/healthcheck`、`StatusCode=200`。

## 配置构造

- `ClusterBasic.DisableHealthCheck=false`、`BackendConf.OutlierDetectionHttpCode=5xx`。
- 业务与 `/healthcheck` 均返回 500。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | switch.example.org | /v1/chat/completions | 第 1 次 | 触发被动失败计数 |
| 2 | switch.example.org | /v1/chat/completions | 第 2 次 | 后端已被摘除 |

## 预期结果

- 第 1 次：上游 500 透传，业务命中 1。
- 第 2 次：网关 5xx，业务命中保持 1。
- 5 秒内出现 `/healthcheck` 请求。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC04_*` 函数
