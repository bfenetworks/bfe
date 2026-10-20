# BFE 批量任务状态与结算设计（mod_ai_batch）

## 1. 背景与职责划分

BFE 以透传方式支持 provider 原生 OpenAI Batch API（`/v1/files` + `/v1/batches`），计费走 batch 价、支持"预留-结算-释放"三态对账。职责按面切分：

| 面 | 组件 | 职责 |
|----|------|------|
| 数据面 | `mod_ai_batch` | 端点识别、文件与任务簿记（`BATCH_FILE` / `BATCH_TASK` / `BATCH_ACTIVE`）、批量限流前置检查、下载输出文件时解析 result 文件 usage 并发布结算契约 |
| 数据面 | `mod_ai_token_auth` | 预留执行（`BATCH_RESERVE*`）、结算/释放执行（`BATCH_SETTLED` 去重），并把结算结果回写契约 |
| 控制面 | `ai-gateway-api`（见该仓库 `design-docs/sys-design/details/批量任务与对账.md`） | `batch_tasks` / `batch_files` 权威实体、管控 API、对账 job（Redis 同步 / 状态推进 / 兜底结算 / TTL 清理） |

**核心约定：Redis 是任务同步的主事件源**。数据面把请求链路上观测到的事实**原子地**写进 `BATCH_*` 哈希，控制面对账 job 每 1min SCAN 同步落库；控制面不重复实现簿记，只在数据面拿不到的事实上出网（provider 侧回读）。

## 2. Redis 键结构（as-built）

所有键与 `QUOTA_*` 同集群、同 1e-8 定点口径，TTL 48h（`defaultBatchStateTTL` / `batchReserveTTL`）。

```
BATCH_FILE:<cluster>:<file_id>   HASH {
                                   api_key_id, key_name, cluster,
                                   lines, bytes,             # 文件内容口径
                                   purpose, dir,             # dir: input|output
                                   filename, batch_id,       # 输出绑定带 batch_id
                                   lines_src, bytes_src }    # exact|approx
BATCH_TASK:<batch_id>            HASH {
                                   api_key_id, cluster, key_name,
                                   input_file_id, output_file_id, endpoint,
                                   request_counts,           # provider 原样 JSON
                                   est_lines, reserved_units, reserved,
                                   status, created_at,       # created_at: provider unix 秒
                                   usage_in, usage_out, settle_units,
                                   settle_status, usage_source }
BATCH_SETTLED:<batch_id>         STRING "1"（SET NX EX）    # 结算/释放去重占位
BATCH_ACTIVE:<api_key_id>        ZSET member=batch_id score=created_unix

BATCH_RESERVE:<planRedisKey>     INCRBY 计数（可用余额 = 余额 − 镜像）
BATCH_RESERVE_BATCH:<batch_id>   HASH planRedisKey -> reserved units
```

字段写入点是唯一契约，任何新增字段必须同时更新本节、`batch_state.go` 的键结构注释与控制面同步视图（`ai-gateway-api/model/ibatch/redis_view.go`）。

## 3. 写入时序

| 阶段 | 写点 | 写入内容 |
|------|------|----------|
| upload（`POST /v1/files` 2xx） | `finishUpload`（响应体读完时） | `BATCH_FILE{lines, bytes, filename, purpose, dir=input}`；`BATCH_AFFINITY` 链起点 |
| create（`POST /v1/batches` 2xx） | `finishCreate` | `BATCH_RESERVE*`（两阶段预留的第二步）、`BATCH_TASK{...}`、`BATCH_ACTIVE`、输出绑定（若响应已带 `output_file_id`） |
| get（`GET /v1/batches/{id}` 2xx） | `finishGet` | `BATCH_TASK{status, output_file_id, request_counts, endpoint}`；输出文件绑定；无结果文件终态时发布释放契约 |
| download（`GET /v1/files/{id}/content` 2xx） | `usageScanBody.finalize`（响应体扫描完时） | 仅发布结算契约到 `AiBasicInfo`（不写 Redis） |
| 任意批量的 request finish | `requestFinishHandler` | `usage_in` / `usage_out` / `settle_units` / `settle_status` / `usage_source`（**结算/释放的事实**） |

### 3.1 为什么结算类字段在 `HandleRequestFinish` 写

模块注册顺序为 `mod_ai_token_auth`（`bfe_modules.go` 中先注册）→ `mod_ai_batch`，因此 finish 回调按同序执行：

```
usageScanBody.finalize  →  aiMeta.BatchSettle/BatchSettleId/BatchUsageByModel
   ↓ (读响应体期间，早于任何 finish 回调)
mod_ai_token_auth.finish →  batchContractHandler → batchSettle/BatchRelease
   ↓  SETNX BATCH_SETTLED 抢占成功后才真正结算/释放，并置
      aiMeta.BatchSettleApplied / aiMeta.BatchSettledUnits
mod_ai_batch.finish     →  按 BatchSettleApplied 写 settle_*
```

这样 `settle_status` **只有在真正结算成功时才写**：抢占失败（控制面兜底已结算、或多路径并发）不会被误标为 settled。早期实现把该字段写在扫描器里（先标 settled、后抢占），存在"未结算却标 settled"的窗口，已修正。

## 4. 结果文件 usage 解析口径

OpenAI Batch 结果文件（jsonl）每行形如：

```json
{"custom_id":"...","response":{"status_code":200,"body":{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}
```

usage 与 model 位于 **`response.body`**。解析规则（`resp_capture.go:handleLine`）：

1. `response.body.usage` 优先，缺失时回退扁平 `response.usage`（兼容历史夹具与非标准上游）；
2. `response` 为 `null`/缺省的错误行（`error != null`）视为无 usage，跳过且不计错误；
3. `response` 存在但两条路径都取不到 usage → 计 `SETTLE_NO_USAGE`（该行不参与计费）；
4. 整行 JSON 解析失败 → 计 `SETTLE_USAGE_PARSE_ERR`；
5. 单行解析缓冲上限 `MaxLineParseBytes`（默认 1MiB），超长行整体丢弃并计错误。

控制面兜底结算（`ai-gateway-api/model/ibatch/settle.go`）必须保持同一口径，否则会出现"下载拦截正常、兜底结算 0 元"的两套结果。

## 5. 结算与释放

- **结算**（`BatchSettleSettle`）：`mod_ai_batch` 汇总 `usageByModel` 发布契约 → `mod_ai_token_auth.batchSettle` SETNX `BATCH_SETTLED` 抢占 → 按 `mode=batch` 模型价分组计价 → 扣减绑定计划 → `BatchRelease` 冲正预留 → 置 `BatchSettleApplied=true`、`BatchSettledUnits=costUnits`；`mod_ai_batch` 随后写 `usage_in/usage_out/settle_units/settle_status=settled/usage_source=download`。
- **释放**（`BatchSettleRelease`）：terminal 无结果文件（`expired`/`failed`/`cancelled`）或 cancel 受理 → `BatchRelease` 原子冲正预留镜像 → `BatchSettleApplied=true`；`mod_ai_batch` 写 `settle_status=released`。
- `completed` 不释放：先按实结金额结算，预留差额随结算自然冲正。

`BATCH_SETTLED` 与控制面 `WHERE settle_status='reserved'` 条件更新构成双重幂等防线（多实例、重跑、下载拦截 vs 兜底结算互不双倍）。

## 6. 文件行/字节统计口径

`BATCH_FILE.lines/bytes` 与 `BATCH_TASK.est_lines`（预留公式输入）必须描述**文件内容**，不是 HTTP 信封：

1. `bytes` 优先取 `POST /v1/files` 响应里的 File 对象 `bytes`（权威值），`lines` 取 multipart 中 `name="file"` 那一 part 的内容换行数；
2. 上传请求体已整体缓冲（AI 网关在提取 model 时已缓冲）时，用 `mime/multipart` 解析 part 得到 `filename`/`lines`/`bytes`，并**用同一结果**做前置限额检查（`BatchEffMaxFileBytes` / `BatchEffMaxFileLines`）；
3. 流式（chunked 或超缓冲上限）无法定位 part 时，退回整包近似计数，并在绑定上写 `lines_src=approx`/`bytes_src=approx`，同时计 `UPLOAD_COUNT_APPROX`；
4. 早期实现统计的是整个出站 multipart 请求体（含 part 头、boundary、CRLF），会把 1 行/206 字节的文件记成 11 行/509 字节，直接放大 `est_lines` 与预留额，已修正。

## 7. 监控计数器（`mod_ai_batch`）

| 计数器 | 含义 |
|--------|------|
| `REQ_TOTAL` | 被识别为批量操作的请求数 |
| `UPLOAD_REJECT_SIZE` / `UPLOAD_ABORT_STREAM` | 上传超限拒绝 / 流式上传中途超限中断 |
| `UPLOAD_COUNT_APPROX` | 无法解析 file part、退回整包近似计数的上传数 |
| `FILE_BIND_WRITE(_FAIL)` / `TASK_WRITE(_FAIL)` | 绑定与任务写入成功/失败（fail-open 计数） |
| `OWNER_CHECK_REJECT` / `OWNER_CHECK_MISS` | 下载归属校验拒绝 / 绑定缺失按策略放行 |
| `RESERVE_OK` / `RESERVE_PRECHECK_MISS` / `RELEASE_OK` | 预留成功 / 余额预检失败 / 释放 |
| `SETTLE_USAGE_PARSE` | 成功解析出 usage 的下载结算数 |
| `SETTLE_NO_USAGE` / `SETTLE_USAGE_PARSE_ERR` | 结果文件行存在但无 usage / 行解析失败 |
| `TASK_SETTLE_WRITE` | finish 阶段写结算字段的次数 |
| `REDIS_ERR` | Redis 操作错误（fail-open） |

`SETTLE_USAGE_PARSE` 长期为 0 而 `REQ_TOTAL` 有下载增长，等价于"下载没有结算"的强信号；`SETTLE_NO_USAGE`/`SETTLE_USAGE_PARSE_ERR` 用于区分"上游没写 usage"与"我们解析不了"。

## 8. 请求/响应体透传完整性（数据面不改写）

provider 原生 Batch API 的语义是**经网关透传**：`/v1/files` 的 multipart 原始字节（boundary、part 头、文件内容）与 `/v1/batches` 的创建 JSON 必须按客户端原样到达 provider，网关只在旁路做限流/预留/记账/簿记，不得改写请求体语义。对应地，provider 响应与回读对象（file id 等）原样返回客户端。

数据面实现约束（issue #1426，`bfe_server/reverseproxy.go`）：

- `doSingleAIForward` 的 target model 改写（`ReqBodyJsonSet(basicReq, "model", targetModel)`）对透传模式整体跳过：`isBatchPassthroughMode`（`ModeFile || ModeBatch`）为真时不注入/覆盖 `model` 字段，原始字节与 `Content-Length` 保持不变。
- 兜底守卫：请求 `Content-Type` 显式非 JSON（`multipart/form-data`、`text/*`、`application/octet-stream` 等）时同样拒绝改写，防止未来新增透传类 mode 漏接 mode 判定后重蹈覆辙；头缺失或非法值不拦截（部分客户端发 JSON 体不带 Content-Type）。
- `aiMeta.TargetModel` 的计算与记录不受守卫影响：它仍是路由/计费/访问日志的旁路决策记录，但**不代表**请求体内容——透传模式下 TargetModel 与请求体可以不一致，`TargetModel` 永远不得被消费方当作"请求体里的 model"使用。
- chat/completion 模式的 model 覆盖、fallback 重算、`stream_options.include_usage` 注入等既有改写行为不在本节范围内，不受影响。

## 9. 边界与限制

- **键 TTL 48h ≫ 对账周期 1min**：理论上无丢失窗口；BFE Redis 写失败（fail-open）由控制面对账巡检的日志补登兜底。
- **`ctx.batchId` 缺失**（输出文件绑定被 TTL 清理/历史键）：下载结算只能按 `file:<id>` 键去重，无法回写 `BATCH_TASK`，由控制面兜底结算补齐。
- **客户端中止下载**：`usageScanBody` 不 finalize（结算契约不发布），避免按半截文件结算；由控制面对账 job 兜底。
- **chunked 上传**：行/字节只能近似（`*_src=approx`），预留额可能偏大，属已知限制；精确化需要在流式路径上做 multipart 增量状态机。
- **`reserved_units` 与 `BATCH_RESERVE_BATCH` 双源**：前者是任务视图镜像（供控制面展示与核对），后者才是释放/结算的簿记源；两者不一致由对账巡检告警。
- **不改计费口径**：批量价、预留公式、`BATCH_SETTLED` 去重语义与 fail-open 策略均不在本节设计范围内变动。

## 10. 变更记录

| 日期 | 内容 |
|------|------|
| 2026-10-07 | 首版：批量透传一期（端点识别、`mod_ai_batch`、预留/结算/释放、粘性、限流）；见 `modifications/2026-10-07-ai-batch-support/` |
| 2026-10-19 | 终态字段与结算修复：`BATCH_TASK` 补 `output_file_id`/`request_counts`/`endpoint`/`created_at`/`reserved_units`/`settle_units`/`usage_source`；result 文件 usage 按 `response.body.usage` 解析；结算字段改在 finish 阶段按结算结果回写；行/字节改按 file part 与 File 对象统计；见 `modifications/2026-10-19-issue-254-ai-batch-task-state-and-settle-fix/` |
| 2026-10-20 | 透传体完整性：target model 改写对 `ModeFile`/`ModeBatch` 跳过，非 JSON Content-Type 兜底拒绝改写（issue #1426）；见 `modifications/2026-10-20-issue-1426-batch-passthrough-model-rewrite-fix/` |