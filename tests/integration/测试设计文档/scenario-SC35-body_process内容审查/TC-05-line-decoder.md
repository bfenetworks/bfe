# TC-05 line decoder

## 用例编号与名称

TC-05 line decoder

## 所属场景

SC35 body_process 流式处理与内容审查

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 line decoder 按行产生事件并保持字节不变。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端与审计服务均为进程内 mock。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_audit` 与审计服务（`/text-filter`、`/text-replace`）已启动。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`。
4. `ai_route.data` 中 `ak_audit` 命中 `cluster_audit`；`token_rule.data` 声明不限量 Key。

## 配置构造

- 规则 `RequestProcess`：`Dec=line`、`Proc=[{textfilter, [审计 URL]}]`。
- 请求体为两行 JSON。

## BFE 请求

| 步骤 | Host | Path | 关键设置 | 说明 |
|------|------|------|----------|------|
| 1 | audit.example.org | /v1/chat/completions | 两行 body | 逐行审查 |

## 预期结果

- HTTP 200；后端收到与原始字节完全一致的两行 body。
- 审计 `/text-filter` 调用 2 次（每行一次）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC05_*` 函数
