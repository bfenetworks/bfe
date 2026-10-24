# TC-04 SSE逐事件审查不缓冲

## 用例编号与名称

TC-04 SSE逐事件审查不缓冲

## 所属场景

SC35 body_process 流式处理与内容审查

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 SSE 事件逐条送审并在持有期间即可到达客户端。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_audit` 与审计服务（`/text-filter`、`/text-replace`）已启动。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`。
4. `ai_route.data` 中 `ak_audit` 命中 `cluster_audit`；`token_rule.data` 声明不限量 Key。

## 配置构造

- 规则 `ResponseProcess`：`Dec=sse`、`Proc=[{textfilter, [审计 URL]}]`。
- 后端先发 3 个 SSE 事件，再由 `SSEHold` 阻塞；审计结果为 `AUDITED <原事件>`。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | audit.example.org | /v1/chat/completions | JSON body | 读取首帧后释放 hold |

## 预期结果

- 首帧在 hold 释放前即可读到，且包含 `AUDITED`。
- 释放后共收到 3 个已审查事件；审计 `/text-replace` 调用 3 次。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC04_*` 函数
