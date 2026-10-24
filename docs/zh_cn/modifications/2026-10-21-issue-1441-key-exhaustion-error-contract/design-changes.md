# BFE Issue #1441 修复方案（v1.8.9-dev 前向移植）：Key 耗尽与网关侧失败响应统一错误契约

- Issue: https://github.com/bfenetworks/bfe/issues/1441
- 性质：v1.8.8 分支修复（commit `db81828f`）向 v1.8.9-dev 的前向移植。缺陷描述、三处裁定点（`NO_AVAILABLE_KEY`→429+`rate_limit_error`；归因走 `err_code`；实施 `Retry-After`；范围=网关侧合成 500）的完整记录见 v1.8.8 分支同名文档；本文记录 **dev 分支的移植差异与验证**。
- 缺陷（摘要）：Key 耗尽路径的合成响应缺 `error.code`（违反 `ai_error_codes.md` §3.2 必填）、无退避信号、访问日志 `err_code` 为空，客户端按 5xx 无差别重试导致约 10 倍重试风暴放大。

## 一、dev 分支移植差异（相对 v1.8.8 版）

### 1. 码值对齐：`CodeUpstreamRateLimited` → `CodeNoAvailableKey`

dev 的 #1440 移植曾用 `CodeUpstreamRateLimited`（`UPSTREAM_RATE_LIMITED`，来自 `bfe_model_protocol` 上游归一目录）作为 Key 耗尽响应的码值。本移植**统一为 `NO_AVAILABLE_KEY`**（v1.8.8 裁定，owner：zhangmiao），理由：① 语义准确——本路径是"网关自身选不出 Key"，不是"上游返回的限流"（`UPSTREAM_*` 系列专供 `AIConf.NormalizeUpstreamError` 归一使用）；② 两分支错误码一致，客户端与对账逻辑可移植。`NO_AVAILABLE_KEY` 在 dev 目录中为独立字面量（不 alias `bfe_model_protocol`），同时登记进 dev 特有的 `ErrorCodeToErrorType` 映射（`request_ai_basic.go`）。

### 2. 网关标记与归一化器交互（dev 特有）

dev 的 `AiError.CreateErrorResponse` 自带 `X-Bfe-Gw-Error: 1` 网关标记（`request_ai_basic.go`），`normalizeUpstreamError`（`reverseproxy_ai_error.go`）对带标记的响应跳过归一——因此：

- Key 耗尽的合成 429：统一 AiError 体 + 网关标记 + `Retry-After` 头 + `details.retry_after_seconds`，归一器天然跳过，**无双重包装风险**（v1.8.8 无归一化器，靠"调用侧才加头"规避）；
- 步骤 3 的 `BACKEND_UNAVAILABLE`(502) 响应同样经 `CreateErrorResponse` 构造，自动带网关标记，归一器跳过；
- 新增 `isAIClientAbort` 谓词与 v1.8.8 相同：客户端中断（`ErrClientWrite/Close/Reset`）保持既有空体 500 记录（响应发不出去，构造 JSON 无意义）。

### 3. 其他结构性差异（已核对）

- dev 的错误码目录有 `ErrorCodeToErrorType` 映射（v1.8.8 无），`CodeNoAvailableKey` 已登记为 `TypeRateLimitError`；
- dev 的 `aiClusterInvoke` 有 batch 透传（`batchHint`），耗尽分支不受影响；`:1962` 的 batch `404-batch` 惩罚 TTL 仍为独立字面量 60（与本常量语义不同，未合并）；
- dev 的亲和 Redis 句柄来自 `srv.AIKeyAffinityRedis`，与本移植无关。

## 二、改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | 新增 `CodeNoAvailableKey = "NO_AVAILABLE_KEY"`（独立字面量，注释说明与 `CodeUpstreamRateLimited` 的语义边界）；`ErrorCodeToStatusCode` → 429；`ErrorCodeToErrorType` → `rate_limit_error` |
| `bfe_basic/error_code.go` | 新增哨兵 `ErrAiNoAvailableKey`（注释明确 != `ErrAiRateLimit`） |
| `bfe_server/reverseproxy.go` | 耗尽分支改 `NewAiErrorWithDetails(CodeNoAvailableKey, …, &AiErrorDetail{Model: attempt.Model, RetryAfterSeconds: 60})` + `Retry-After` 头 + `ErrCode`/`ErrMsg`；429 惩罚 TTL 抽常量 `aiKeyPenaltyTTL429`；`ServeHTTPForAI` 的 findProduct 失败与 `err != nil \|\| res == nil` 网关侧出口改 `BACKEND_UNAVAILABLE`(502) 统一错误体；新增 `isAIClientAbort` 谓词（客户端中断保持空体记录） |
| `bfe_server/reverseproxy_ai_test.go` | 扩展 `TestAiClusterInvokeExhaustionReturnsSynthetic429`（`error.code`/`details.retry_after_seconds`/`Retry-After`/`ErrAiNoAvailableKey` 断言，保留 `X-Bfe-Gw-Error` 断言）；新增 `TestBackendUnavailableErrorResponseContract`（含 Retry-After 护栏）与 `TestIsAIClientAbort` |
| `docs/zh_cn/sys_design/ai_error_codes.md` | §2.4 新增两行、§3.2/§5/§6 同步（同 v1.8.8） |
| `tests/integration/.../scenario-SC02/…` | TC-09 臂 B 扩展三条契约断言（`code=NO_AVAILABLE_KEY`、`Retry-After: 60`、`retry_after_seconds: 60`），设计文档同步 |

## 三、验证结果（v1.8.9-dev，本次实跑）

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、bfe_basic） | 通过 |
| 单测：`bfe_basic` / `bfe_server`（含新增/扩展用例） | 通过 |
| SC02 全量（含 TC-09 扩展，两臂） | 见提交前验证 |
| SC10 / SC01 / SC05 / SC07 / SC08 / SC12 回归 | 见提交前验证 |

## 四、可观测变化（变更说明用）

① Key 耗尽响应：`UPSTREAM_RATE_LIMITED` → `NO_AVAILABLE_KEY`（码值对齐），新增 `Retry-After` 与 `details.retry_after_seconds`；② `err_code` 由空变为 `NO_AVAILABLE_KEY`（`ClientReqFail` +1）；③ 网关侧失败（no-cluster/连接失败）由空体 500 变为 502 + JSON（`BfeStatusCode` 同步 502）；④ 客户端中断路径行为不变（既有空体记录 + 计费旁路）。

## 五、遗留事项

- 非 AI `ServeHTTP` 的本地错误与非 AI 模块的契约化另开卡（同 v1.8.8）；502 路径端到端集成测试列为后续增强；
- dev 侧 #1440 移植文档（`2026-10-21-issue-1440-...`）中"`CodeUpstreamRateLimited`"的表述已被本移植取代，以本文档为准。