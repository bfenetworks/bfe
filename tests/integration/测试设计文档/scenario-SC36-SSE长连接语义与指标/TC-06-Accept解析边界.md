# TC-06 Accept解析边界

## 用例编号与名称

TC-06 Accept解析边界

## 所属场景

SC36 SSE 长连接语义与指标

## 版本声明

- `bfe`：当前源码版本

## 测试目的

固定请求侧 Accept 解析行为（普通转发路径）：混合大小写可识别，带参数后缀不识别。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_sse` 已启动；Redis（miniredis）已启动用于 token 规则加载。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_sse`；`token_rule.data` 声明不限量 Key `ak_auth`。
5. `cluster_sse.ClusterBasic.TimeoutWriteClient=300`（毫秒）。

## 配置构造

- `EnableAiGateway=false`（请求侧判定只在普通转发路径生效）。
- 后端为非 SSE 的慢响应：先写头并 flush，`HoldAfterHeader` 800ms 后写 JSON；集群写超时 300ms。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | sse.example.org | /v1/chat/completions | `Accept: application/json, TEXT/EVENT-STREAM` | 混合大小写与其他 media type |
| 2 | sse.example.org | /v1/chat/completions | `Accept: text/event-stream; q=0.9` | 带参数后缀 |

## 预期结果

- Arm 1：HTTP 200 且收到完整 JSON（识别为 SSE，超时豁免）。
- Arm 2：读取出错或正文不完整（未识别为 SSE，写超时生效）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC06_*` 函数
