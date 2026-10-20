# TC-02 非SSE响应被写超时切断

## 用例编号与名称

TC-02 非SSE响应被写超时切断

## 所属场景

SC36 SSE 长连接语义与指标

## 版本声明

- `bfe`：当前源码版本

## 测试目的

对照臂：同样的等待时序但响应非 SSE 时，正文写入应被集群写超时切断。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_sse` 已启动；Redis（miniredis）已启动用于 token 规则加载。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_sse`；`token_rule.data` 声明不限量 Key `ak_auth`。
5. `cluster_sse.ClusterBasic.TimeoutWriteClient=300`（毫秒）。

## 配置构造

- mock 后端：先写响应头并 flush，`HoldAfterHeader` 阻塞 800ms 后写 JSON 正文。
- 集群写超时 300ms。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | sse.example.org | /v1/chat/completions | 不带 `Accept` | 非 SSE 响应，正文延迟 800ms |

## 预期结果

- 客户端读取出错或响应体不完整（不包含完整 JSON 正文）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC02_*` 函数
