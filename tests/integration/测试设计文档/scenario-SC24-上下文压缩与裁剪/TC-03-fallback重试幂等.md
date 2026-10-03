# TC-03 fallback 重试幂等

## 用例编号与名称

TC-03 fallback 重试幂等（只压缩一次）

## 所属场景

SC24 上下文压缩与裁剪

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_context`）

## 测试目的

`HandleAfterAITargetModel` 每次 cluster attempt 触发。验证 cluster fallback 时：`AiBasicInfo.ContextCompressStatus` 幂等守卫使同一请求只压缩一次，fallback attempt 重放压缩后的 body（与 primary 收到的字节一致），且不发生二次压缩。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 同 TC-01。
2. mock 后端 `cluster_ctx_primary` 返回 **503**（触发集群 fallback），`cluster_ctx_fallback` 返回 200。
3. `ai_route.data` 中 `ak_ctx` 规则 targets=`cluster_ctx_primary`、fallbacks=`cluster_ctx_fallback`。

## 配置构造

同 TC-01。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body 要点 |
|------|------|------|---------------|-----------|
| 1 | `ctx.example.org` | `/v1/chat/completions` | `Bearer ak_ctx` | TC-01 裁剪体（估算超触发线） |

## 预期结果

- 客户端收到 fallback 的 200 响应，且带 `x-ai-context-compression: tokens=...; mode=balanced`。
- `cluster_ctx_primary` 与 `cluster_ctx_fallback` 各被命中 1 次。
- primary 收到的 body 已压缩（含 `...[truncated]`）。
- fallback 收到的 body 与 primary **逐字节一致**（重放压缩产物），且 `...[truncated]` 恰好出现 1 次（无二次压缩痕迹，无重复 `[COMPRESSED:rewrite]` 标记）。
- 访问日志恰好 1 条记录：`ai_context_compress_status=trim`，`before > after`，`mode=balanced`。
