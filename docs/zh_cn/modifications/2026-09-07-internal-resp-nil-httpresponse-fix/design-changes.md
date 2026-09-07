# BFE 内部构造响应未设置 HttpResponse 导致 mod_header panic（bfenetworks/bfe#1359）

## 1. 背景与问题

社区 issue [bfenetworks/bfe#1359](https://github.com/bfenetworks/bfe/issues/1359)：

AI 网关模式下，某 API Key（entity A）访问其路由表未覆盖的模型前缀（`providerB/*`）时，预期返回 404 "AI route not found"，实际客户端收到 `empty reply from server`。stdout.log 中出现 panic：

```
panic: conn.serve(): ..., runtime error: invalid memory address or nil pointer dereference
github.com/bfenetworks/bfe/bfe_modules/mod_header.getHeader(...)
        /src/bfe_modules/mod_header/action.go:449
github.com/bfenetworks/bfe/bfe_modules/mod_header.(*ModuleHeader).applyProductRule(...)
        /src/bfe_modules/mod_header/mod_header.go:105
github.com/bfenetworks/bfe/bfe_modules/mod_header.(*ModuleHeader).rspHeaderHandler(...)
        /src/bfe_modules/mod_header/mod_header.go:152
```

触发配置特征：加载 `mod_header`，且 `header_rule.data` 的 `global` product 配置了 `RSP_HEADER_SET` 响应规则（如注入 `%bfe_log_id` 响应头）。用户环境为 BFE 1.8.5，当前 v1.8.7 代码同样存在该问题。

## 2. 根因分析

### 2.1 缺陷本体：内部构造的响应没有写入 `request.HttpResponse`

BFE 内部构造响应的构造函数中，只有部分会把响应挂到请求上：

| 构造函数 | 位置 | 是否设置 `request.HttpResponse` |
|---|---|---|
| `CreateInternalResp` | `bfe_basic/common.go:78-85` | 是（`request.HttpResponse = res`） |
| `AiError.CreateErrorResponse` | `bfe_basic/request_ai_basic.go:452` | 是 |
| **`CreateSpecifiedContentResp`** | **`bfe_basic/common.go:87`** | **否（缺陷）** |

`CreateSpecifiedContentResp` 共 3 个调用点，构造的响应均不挂到 `basicReq.HttpResponse`：

| 调用点 | 场景 |
|---|---|
| `bfe_server/reverseproxy.go:1218` | AI 路由未命中，返回 404 "AI route not found"（本 issue 触发点） |
| `bfe_server/reverseproxy.go:871` | 请求预处理失败，返回 400 |
| `bfe_modules/mod_unified_waf/mod_unified_waf.go:478` | WAF 拦截响应 |

### 2.2 触发链路（以 issue 的 404 场景为例）

1. API Key A 访问 `providerB/*`：`mod_ai_route` 在其绑定的路由表链（apikey → entity → global）中无命中 → `aiResult == nil`；
2. `reverseproxy.go:1216-1223` 走 "AI gateway mode: no route hit" 分支，`CreateSpecifiedContentResp(basicReq, 404, ...)` 构造响应后 `goto response_got`——**`basicReq.HttpResponse` 为 nil**；
3. `response_got` 之后执行 `HandleReadResponse` 回调链（`reverseproxy.go:1400` 附近）；
4. `mod_header.rspHeaderHandler`（`mod_header.go:147`）→ `applyProductRule(request, RspHeader, "global")`：用户配置的 global `RSP_HEADER_SET` 规则命中 → `getHeader(request, RspHeader)`（`mod_header/action.go:449`）执行 `h = &req.HttpResponse.Header`，对 nil 指针解引用 → panic；
5. panic 在 `conn.serve` 被捕获后连接直接拆除，404 响应未写出 → 客户端表现为 `empty reply from server`。

### 2.3 触发条件（三个条件同时成立）

1. **请求走在内部构造响应路径上**（`CreateSpecifiedContentResp` 的三个调用点之一）——缺陷本体，无条件存在；
2. **加载了在 `HandleReadResponse` 回调中解引用 `request.HttpResponse` 的模块**——`mod_header` 是已知触发者：绝大多数 `HandleReadResponse` 模块操作的是回调参数 `res`（有值，安全），而 `mod_header` 经 `getHeader(request, ...)` 读的是 `request.HttpResponse`（可能为 nil）；未来若其他模块直接读 `request.HttpResponse` 同样会触发；
3. **mod_header 规则表命中**——`applyProductRule` 仅当 `ruleTable.Search(product)` 命中才调用 `getHeader`；`global` 规则对所有请求生效，必然命中。

补充说明：`HandleRequestFinish` 回调（如 `mod_ai_token_auth` 的计费收尾）接收的同样是 `request.HttpResponse`，nil 响应也会让这些模块拿到空响应对象，修复后一并受益。

## 3. 修复方案

### 3.1 核心修复（治本，一处覆盖全部调用点）

在 `CreateSpecifiedContentResp` 中补上与另两个构造函数一致的赋值：

```go
func CreateSpecifiedContentResp(request *Request, responseCode int, contentType string, content string) *bfe_http.Response {
    resp := new(bfe_http.Response)
    ...
    request.HttpResponse = resp  // 新增
    return resp
}
```

影响面：`reverseproxy.go` 两处与 `mod_unified_waf` 一处全部修复；正常转发路径不受影响（`HttpResponse` 已在其它路径赋值，此处仅补齐缺失路径）。

### 3.2 集中兜底（防未来回归，替代逐模块防御）

把"凡是要发送给客户端的响应，`request.HttpResponse` 必然已赋值"从隐式约定升级为框架强制的不变量，在内部响应进入统一发送路径的汇合点兜底。`reverseproxy.go` 有两个这样的汇合点（`response_got` 标签）：

- `ServeHTTPForAI` 的 `response_got:`（`reverseproxy.go:1358` 附近，AI 网关路径，本 issue 的 404/400 均汇入此处）；
- `ServeHTTP` 的 `response_got:`（`reverseproxy.go:903` 附近，普通转发路径）。

在两处 `response_got` 标签后各加一行：

```go
response_got:
    // 不变量兜底：凡是要发送的响应，HttpResponse 必须已挂接。
    // 内部构造响应的历史路径（如 CreateSpecifiedContentResp 早期版本）曾遗漏赋值，
    // 导致 HandleReadResponse/HandleRequestFinish 回调解引用 nil 而 panic。
    if res != nil && basicReq.HttpResponse == nil {
        basicReq.HttpResponse = res
    }
    ...
```

说明：

- 正常转发路径中 `basicReq.HttpResponse` 已赋值，兜底条件不成立、无副作用；
- 防御范围覆盖**所有**在 `HandleReadResponse`/`HandleRequestFinish` 回调中读 `request.HttpResponse` 的模块（现在的与未来的），无需逐个模块打补丁；
- 相比"mod_header 命中 nil 时静默跳过规则"的原始设想，集中兜底不会出现"响应头该注入却没注入"的掩盖效应——缺失赋值在框架层被直接纠正。

> 注：3.1 修复后，已无任何现存路径需要兜底命中；3.2 仅面向未来新增的响应构造代码，属防御性措施。

### 3.3 回归测试

在 SC01（路由表查找与绑定）场景新增测试例：

- testdata 增加 `mod_header/mod_header.conf` 与 `header_rule.data`（global product 配置 `RSP_HEADER_SET X-Bfe-Log-Id %bfe_log_id`），`bfe.conf` 的 `Modules` 追加 `mod_header`；
- 用未绑定路由表的 API Key（或访问路由未覆盖的模型前缀）请求，断言：
  1. 响应 404，body 含 "AI route not found"（回归 issue 预期行为）；
  2. 响应头含 `X-Bfe-Log-Id`（mod_header 规则在内部响应路径上正常生效）；
  3. BFE 无 panic（可通过响应正常返回隐式覆盖）。

单元测试：`bfe_basic` 增加 `CreateSpecifiedContentResp` 设置 `HttpResponse` 的断言。

## 4. 兼容性

- 行为变化仅体现在原本 panic/空响应的路径：修复后这些路径正确返回构造的响应（404/400/WAF 响应），且 `HandleReadResponse`/`HandleRequestFinish` 回调可正常访问响应对象。
- 无配置变更，无协议变更；正常转发路径行为完全不变。

## 5. 关键文件索引

| 文件 | 改动 |
|---|---|
| `bfe_basic/common.go` | §3.1 核心修复 |
| `bfe_server/reverseproxy.go` | §3.2 两处 `response_got` 集中兜底 |
| `bfe_basic/common_test.go` | 单元测试 |
| `tests/integration/implementation/scenario-SC01-route-table-lookup/` | §3.3 回归测试例 |
