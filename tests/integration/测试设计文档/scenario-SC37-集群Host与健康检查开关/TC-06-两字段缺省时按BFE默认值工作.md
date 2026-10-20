# TC-06 两字段缺省时按BFE默认值工作

## 用例编号与名称

TC-06 两字段缺省时按BFE默认值工作

## 所属场景

SC37 集群 Host 与健康检查开关

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证配置缺省时按 BFE 默认值（Host 透传、失败可摘除）工作。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_switch` 已启动，可按路径区分业务请求与 `/healthcheck`。
3. `bfe.conf`：`EnableAiGateway=false`，不加载 AI 模块。
4. 健康检查用例的 `CheckConf`：`FailNum=1`、`SuccNum=1`、`CheckInterval=100ms`、`Uri=/healthcheck`、`StatusCode=200`。

## 配置构造

- 通过覆盖结构删除 `DisableHealthCheck` 与 `DisableHostHeader` 两个键，让 BFE 应用自身默认值。
- `OutlierDetectionHttpCode=5xx`；业务路径 500，`/healthcheck` 200。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | switch.example.org | /v1/chat/completions | 第 1 次 | 500，触发失败计数 |
| 2 | switch.example.org | /v1/chat/completions | 第 2 次 | 后端应已被摘除 |

## 预期结果

- 第 1 次：上游 500，后端记录 `Host=switch.example.org`。
- 第 2 次：网关 5xx，业务命中保持 1（默认 false 会摘除）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC06_*` 函数
