# TC-11 内部构造 404 与 mod_header 组合

## 用例编号与名称

TC-11 内部构造 404 与 mod_header 组合

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本（含 bfenetworks/bfe#1359 修复：`bfe_basic.CreateSpecifiedContentResp` 补挂 `HttpResponse`，`bfe_server` 两处 `response_got` 集中兜底）

## 测试目的

回归验证 bfenetworks/bfe#1359：当加载 `mod_header` 且配置了 global 响应头规则时，`mod_ai_route` 内部构造的 404 响应（"AI route not found"）必须被正常返回，而不是在 `mod_header` 处理 nil `request.HttpResponse` 时 panic 导致连接被拆除（客户端表现为 `EOF`）；且 global 响应头规则必须应用到该内部响应上。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动，所有 cluster 默认返回 200。
3. 临时 BFE 配置已生成并加载，`bfe.conf` 同时加载 `mod_ai_route` 与 `mod_header`。

## 配置构造

- `mod_header/mod_header.conf`：加载 `header_rule.data`。
- `mod_header/header_rule.data`：配置一条 global 规则 `RSP_HEADER_SET`，为所有响应设置 `X-Proxied-By: bfe`。
- `ApikeyRouteTableBindings` 中不包含 `ak_no_binding`（与 TC-03 相同，确保 `mod_ai_route` 构造内部 404 响应）。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_no_binding` |
| Body | `{}` |

## 预期结果

- 响应状态码：404（连接不被拆除，不出现 `EOF`）。
- 响应体包含 `AI route not found`。
- 响应头 `X-Proxied-By: bfe` 存在（global 响应头规则应用到内部构造的响应上）。
- 所有 mock 后端命中次数均为 0。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
