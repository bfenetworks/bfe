# BFE Issue #1440 修复方案（v1.8.9-dev 前向移植）：Key 耗尽返回 (nil, nil) 导致 cluster fallback 失效

- Issue: https://github.com/bfenetworks/bfe/issues/1440
- 生产事件编号：prod-20261021-02
- 性质：v1.8.8 分支修复（commit `4780f251`）向 v1.8.9-dev 的前向移植。缺陷描述、根因分析、边界裁定与 v1.8.8 完全一致，见 v1.8.8 分支同名文档；本文只记录 **dev 分支的移植差异与验证**。
- 缺陷（摘要）：`aiClusterInvoke()` 在首次选 Key 即失败时以 `res=nil, err=nil` 返回，外层 `shouldTriggerFallback(nil, nil)` 得到 `status=0`，既不 `>=500` 也不在 fallback 白名单中 → 不触发 cluster fallback → 请求落成空 body 500。生产 932 笔请求本可降级却返回 500。

## 一、dev 分支移植差异（相对 v1.8.8 版）

### 1. 合成 429 采用统一 AiError 目录形态

v1.8.8 版用 `bfe_basic.CreateSpecifiedContentResp` 构造普通 JSON 响应；dev 分支有统一错误目录（`bfe_basic.AiError`）与上游错误归一化器（`reverseproxy_ai_error.go` 的 `normalizeUpstreamError`，在 fallback 循环收敛后运行）。移植时合成响应改为：

```go
res = bfe_basic.NewAiError(bfe_basic.CodeUpstreamRateLimited, bfe_basic.TypeRateLimitError,
    "no available upstream api key").CreateErrorResponse(basicReq)
```

收益：

- `AiError.CreateErrorResponse` 自带 `X-Bfe-Gw-Error: 1` 网关标记，`normalizeUpstreamError` 对网关生成的响应**直接跳过**（其既有语义："Gateway-generated responses ... are already in the unified format; skip them"）——避免统一 429 被归一化器二次改写（双重包装）；
- body 为统一目录形态 `{"error":{"code":"UPSTREAM_RATE_LIMITED","type":"rate_limit_error","message":"..."}}`，与 dev 分支其余网关错误响应一致；
- 状态码 429 来自 `ErrorCodeToStatusCode[CodeUpstreamRateLimited]`，429 ∈ `aiFallbackStatusCodes` 白名单的判定不变。

### 2. `shouldTriggerFallback` 的 nil/nil 不变量（步骤 3）

与 v1.8.8 版一致：`res == nil → return true`，位于 `ErrAiRateLimit` 短路（`ServeHTTPForAI` 中 `:1341` 附近）之后，不改变本地限流拒绝语义。dev 上该函数还经过 `modelprotocol` 归一化接缝，nil 分支置于其之前，不受影响。

### 3. 其余结构性差异（无需改动，已核对）

- dev 的 `aiClusterInvoke` 增加了 batch 透传（`isBatchPassthroughMode` / `batchHint`）与 `srv.AIKeyAffinityRedis`（Redis 句柄由 server 持有，不再从模块借用）；耗尽出口在选 Key 失败分支，与这些逻辑无交叠。
- `mod_ai_cache` 的缓存命中 200 走 `goto send_response`，不经过 fallback 循环，与本修复无关。
- `mod_ai_batch` 的 404/413 生成点在进入 fallback 循环前退出，不受 nil/nil 不变量影响。

## 二、改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | 选 Key 失败分支返回统一 AiError 合成 429；`shouldTriggerFallback` 增加 nil/nil ⇒ true 不变量 |
| `bfe_server/reverseproxy_ai_test.go` | `TestShouldTriggerFallback` 增加 `(nil, nil)` 断言；新增 `TestAiClusterInvokeExhaustionReturnsSynthetic429`（额外断言 `X-Bfe-Gw-Error: 1` 网关标记）；测试需传非 nil `srv`（dev 无条件解引用 `srv.AIKeyAffinityRedis`） |
| `docs/zh_cn/sys_design/multi_api_key.md` | §3 伪代码耗尽分支、§4.2「Key 耗尽契约」与「nil/nil 不变量」、§5 关键日志同步（契约描述含网关标记与归一化器交互） |
| `tests/integration/.../scenario-SC02-multi-api-key/` | 同 v1.8.8：新增 TC-09（两臂）+ `sendRequestToHost`/`bfeLogContains` helper + testdata 新增 `multikey-nofallback.example.org` 无 fallback 路由 |
| `tests/integration/测试设计文档/` | SC02 新增 TC-09 设计文档；场景说明与总体说明 TC 数 8→9 |

## 三、验证结果（v1.8.9-dev，本次实跑）

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、SC02 测试包） | 通过 |
| `bfe_server` 单测（含新增 2 例） | 通过 |
| SC02 `TestTC09_FirstSelectionFailureSynthetic429`（两臂） | 通过（实测日志出现 `all ai keys exhausted ... return synthetic 429` 与 `fallback triggered ... status[429]`） |
| SC02 全量回归 | 见提交前验证 |

## 四、与 v1.8.8 版的行为差异说明

唯一对外可见差异：无 fallback 时客户端收到的 429 body 形态——v1.8.8 为 `{"error":{"type":"rate_limit_error","message":"..."}}`，v1.8.9-dev 为统一目录形态（多 `code` 字段）。两者 `error.type` 均为 `rate_limit_error`，状态码与 fallback 语义完全一致。
