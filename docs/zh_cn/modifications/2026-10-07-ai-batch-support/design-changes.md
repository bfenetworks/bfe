# 批量与异步任务支持（一期）：OpenAI Batch API 透传正确化

## 1. 背景

本文档是需求《批量与异步任务支持需求分析（AI 网关场景）》一期（P0"批量透传正确化"）在 BFE 数据面的落地方案，主设计文档：`document-ai-gateway/迭代系统设计/v0.8/批量与异步任务支持/批量与异步任务支持-技术实现方案.md`。

**现状问题**：files/batches 端点不在 `openAIEndpointModes`（`bfe_basic/openai_endpoint.go:31-45`），批量流量落到 `DetectModeFromPath` 默认分支按 `ModeChat` 计费——走 provider 原生 Batch API（5 折计费）的请求被按标准 chat 价扣费；且批量多步操作（上传→创建→查询→下载）无显式粘性保障、大文件（百 MB jsonl）走重试缓冲路径存在内存风险。

**一期目标（BFE 侧）**：

| # | 目标 | 对应需求 |
|---|---|---|
| G1 | `/v1/files*`、`/v1/batches*` 端点被正确识别为独立计费模式（`ModeFile`/`ModeBatch`），不再落入默认 chat 模式 | FR-1 |
| G2 | 批量任务按批量价结算，用量取自结果文件；创建时余额预检+预留，完成时按实际 usage 结算并释放预留 | FR-2 |
| G3 | 批量多步操作路由粘性有明确保障（batch 级绑定 + 客户端亲和兜底），failover 有摘除与冷却 | FR-3 |
| G4 | 大文件上传/下载流式直转、禁缓冲、禁重试，100MB 级文件端到端一致 | FR-4 |
| G5 | 批量流量独立限流维度（创建 RPM/在途上限/文件大小/文件行数），不挤占同步流量配额 | FR-6 |
| G6 | 批量任务事件进访问日志与 Prometheus，供控制面对账与报表 | FR-8 |

**明确不做**（一期非目标）：协议翻译/请求响应体改写（透传原则不变）；在同步 API 上自建 batch 语义；Anthropic/Gemini 批量方言；fine-tuning/assistants files 等其它 stateful API（files 的其它 purpose 直接 400 `unsupported_purpose`）；网关侧文件内容审核；自建异步任务层（二期，控制面）。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 端点识别 | `openAIEndpointModes` 新增 `/files`、`/batches`；Mode 枚举新增 `ModeFile`/`ModeBatch` | `bfe/bfe_basic/openai_endpoint.go`、`bfe/bfe_basic/request_ai_basic.go` |
| 新模块 | 新建 `mod_ai_batch`：操作分类、文件计数、响应体字段提取、batch 粘性维护、预留/释放/结算编排 | `bfe/bfe_modules/mod_ai_batch/*.go`（新建） |
| 模块注册 | 按 §3 顺序挂入 AI 链路（token_auth 最前约束不变） | `bfe/bfe_modules/bfe_modules.go` |
| 计费 | token_auth 新增 `batch_quota.go`：`BatchPreCheck/BatchReserve/BatchRelease` 原语 + `BATCH_RESERVE(:BATCH)` 镜像键 + 下载结算/释放契约执行（SETNX 幂等，按 model 分组 batch 计价） | `bfe/bfe_modules/mod_ai_token_auth/batch_quota.go` |
| 粘性 | AIKeyAffinity 新增 batch 绑定命名空间（TTL 可配、滑动续期）+ 404 惩罚路径 | `bfe/bfe_server/reverseproxy.go`、`bfe/conf/bfe.conf [AIKeyAffinity]` |
| 大文件 | ModeFile/ModeBatch 显式跳过重试缓冲包装；上传计数 Reader；下载流式 usage 解析 | `bfe/bfe_server/reverseproxy.go`、`mod_ai_batch` |
| 限流 | `ai_rate_limit.data` 策略 `rules` 内新增 `batch` 段（data_load 解析）；`batch_check.go` 检查器 + `resolveBatchFileLimits` 有效上限单一计算点 | `bfe/bfe_modules/mod_ai_rate_limit/`、`conf/mod_ai_rate_limit/ai_rate_limit.data`（样例） |
| 基础信息 | `AiBasicInfo` 新增批量字段（BatchId/FileId/Op/Lines/Bytes/Status/Settle/SettleId/UsageByModel/EffMaxFile\*）+ 批量错误码（`BATCH_FILE_TOO_LARGE`/`BATCH_FILE_FORBIDDEN`，limit_type `batch_file`） | `bfe/bfe_basic/request_ai_basic.go` |
| 共享原语 | 操作分类 `ClassifyBatchOp` + 路径 id 提取 + `AiBatchRouteHint`（模块→reverseproxy 粘性传参，bfe_server 不 import 模块包） | `bfe/bfe_basic/batch_op.go`、`bfe/bfe_basic/ai_batch_hint.go` |
| 访问日志 | protobuf 新增批量字段（编号以 `bfe_access.proto` 实际空闲为准，只增不删） | `bfe-access-pb`（独立仓库）、`bfe/bfe_modules/mod_access_pb3/request_log.go` |
| 配置 | 新增模块配置样例 | `bfe/conf/mod_ai_batch/mod_ai_batch.conf`、`mod_ai_batch.data`（新建） |
| 测试 | 单元测试 + 集成测试 | `bfe/bfe_modules/mod_ai_batch/*_test.go`、`integration-test/`（独立仓库） |

配套变更（其他仓库，本文档不展开）：`ai-gateway-api`（批量价模型与导出、`batch_tasks`/`batch_files` 表、管控 API、对账 job）、`ai-gateway-web`（批量任务页）、`bfe-access-pb`/`log-reader`（pb 字段与映射）、`ai-gateway-observability`（Doris 加列与报表）。

## 3. 模块注册位置

`bfe/bfe_modules/bfe_modules.go` AI 模块顺序（`token_auth → ai_cache → ai_route → intent → traffic_mirror → body_process → ai_rate_limit → ai_context → access_pb3`）。`mod_ai_batch` 注册在 `mod_ai_rate_limit` 之后、`mod_ai_context` 之前（其 `HandleAfterAITargetModel` 需在批量限流解析之后执行，`HandleRequestFinish` 簿记需在 token_auth 消费结算契约之后执行；token_auth 保持 AI 链最前）：

```go
	//depends on token calc
	mod_ai_rate_limit.NewModuleAiRateLimit(),

	// mod_ai_batch
	// Requirement: after mod_ai_rate_limit (batch limits resolved into
	// AiBasicInfo first, local rejections already happened); after
	// mod_body_process (file/batch responses are excluded from the body
	// processor, so the response wrapper here sees the exact bytes).
	// HandleRequestFinish only does Redis bookkeeping: the settle/release
	// contract is consumed by mod_ai_token_auth at request finish.
	mod_ai_batch.NewModuleAiBatch(),

	// mod_ai_context
```

**实现期修正（相对早期草案，四处挂载/时序 + 三处缺陷修复，均由 SC28 集成测试暴露）**：

挂载/时序：

1. **粘性查询时机**：key 选择在 `aiClusterInvoke` 内、**早于** `HandleAfterAITargetModel` 回调——mod_ai_batch 在回调里才分类操作，hint 尚未写上。修法：reverseproxy 在 `aiClusterInvoke` 入口发现 hint 为空且 mode 为 file/batch 时，用 `bfe_basic.ClassifyBatchOp` + `BatchPathFileId/BatchPathBatchId` 自行推导（共享原语，不依赖模块执行顺序）。
2. **绑定/簿记写入点从 FinishReq 上移到响应流末**：`FinishReq`（`http_conn.go:608`）在响应刷给客户端**之后**执行，客户端立即发下一步时绑定必丢（SC28 TC-03 抖动暴露）。现所有小 JSON 操作的上传/任务/亲和写入挂在 `captureBody` 完成回调上，下载结算契约挂在 `usageScanBody` 完成回调上。
3. **完成回调按 Content-Length 驱动**：代理对 CL 已知的响应不会多读一次拿 EOF，纯 EOF 触发永不执行（SC28 TC-02 结算为 0 暴露）——缓冲字节数达到响应 CL 即触发，chunked/未知长度回退 EOF。
4. **结算/释放仍在 FinishReq 执行**（token_auth），客户端观察余额/镜像变化为异步；控制面对账以 DB 为权威，集成测试轮询断言。

缺陷修复：

5. **绑定 SETNX 抢占式**：先写者赢，迟到的上传绑定不迁移已建立的链；failover 重绑走 404 删除后重建。绑定值 = 上游 key 名（复用 SETNX 脚本时误写常量 "1"，集成测试抓到）。
6. **错误码状态码映射**：`ErrorCodeToStatusCode` 新增 `BATCH_FILE_TOO_LARGE→413`、`BATCH_FILE_FORBIDDEN→404`（缺映射时响应行为非法状态行 "HTTP/1.1 0"，连接被客户端断开，SC28 TC-04a 暴露）。
7. **`redis_client.Client` 新增 `HGetAll`**：Lua 返回的 HGETALL 嵌套表经 redigo `StringMap` 转换不可靠（release 读不到预留记录，SC28 TC-05 暴露）；读路径改直连命令，token_auth/ai_batch 的 mock 同步补方法。
8. **batch-only 策略合法性**：`ratePoliciesCheck` 的 hasRule 计入 batch 段（纯批量策略此前被拒绝加载，SC28 TC-04 暴露）。
9. **create 的粘性 hint 从请求体推导**：create 路径不带 file id，而 key 选择早于模块回调——reverseproxy 在 `aiClusterInvoke` 入口对 create 操作解析（已缓冲的）body 中的 `input_file_id` 补入 hint（`bfe_basic.ExtractBatchInputFileId`），上传→创建链路因此确定性地粘在同一 key（SC28 TC-03 抖动/TC-13 暴露）。
10. **404 惩罚依赖 `SessionAffinityPenaltyEnable`**：显式 KeyPolicy 会整体覆盖默认值（该 flag 零值 false），批量 failover 测试需显式开启；未开启时摘除后无惩罚过滤，下一请求可能再次命中坏 key。

1. **不用 HandleForward**：该回调无法返回响应（`FilterForward` 只返回 action，`reverseproxy.go:380-389`），413/404 类前置拒绝必须放在 `HandleAfterAITargetModel`（可返回响应）。模块只注册三个回调：`HandleAfterAITargetModel`（分类 + 前置检查 + 计数安装）、`HandleReadResponse`（响应包装）、`HandleRequestFinish`（簿记）。
2. **计数 Reader 包装 `OutRequest.Body` 而非 `HttpRequest.Body`**：`HandleAfterAITargetModel` 触发时 `doSingleAIForward` 已完成 `*outreq = *req` 浅拷贝（`reverseproxy.go:1559-1560`），transport 读的是 `basicReq.OutRequest`（`:317`）；包装原请求体对转发无效。

## 4. 端点识别与 Mode（FR-1）

改动：

1. `request_ai_basic.go:36-50` Mode 枚举新增 `ModeFile = "file"`、`ModeBatch = "batch"`（`ModeOcr` 等常量同一惯例）。
2. `openAIEndpoint.go:31-45` `openAIEndpointModes` 新增两条，利用现有子路径前缀匹配（`lookupOpenAIEndpointMode`，`:73-83`）一条覆盖每类操作：

| 端点 | mode | 覆盖路径 |
|---|---|---|
| `/files` | `ModeFile` | `/v1/files`（上传）、`/v1/files/{id}`（查询/删除）、`/v1/files/{id}/content`（下载） |
| `/batches` | `ModeBatch` | `/v1/batches`（创建/列表）、`/v1/batches/{id}`（查询）、`/v1/batches/{id}/cancel`（取消） |

3. `normalizeEndpointLookupPath`（`:92-100`）对 `/compatible-mode/v1/...` 等 base_url 形态归一化不变，新端点自动兼容。
4. 连带影响（刻意保持）：新端点同时获得 `ai_path_rewrite.go:83-95` 按 provider `protocol_paths` 改写的资格——路径改写与计费共用同一识别源。
5. mode 生产点唯一（`http_conn.go:570`），计费（token_auth）、日志（`mod_access_pb3/request_log.go:412`）自动获得新 mode。

files 的其它 purpose（fine-tune/assistants）：**一期不在网关侧校验 purpose**——上传是 multipart 流式透传，预读 purpose 必须解析 multipart 表单，会消耗上传流且无法无损转发（实现期修正，原"网关 400 拒绝"方案在流式路径上不可行）；交由 provider 侧校验拒绝。端点表结构已为二期预留网关侧校验的扩展点。

## 5. `mod_ai_batch` 模块

### 5.1 文件结构

```text
bfe/bfe_modules/mod_ai_batch/
├── mod_ai_batch.go        # 模块入口：Name/Version/Init，回调注册，热加载
├── conf_load.go           # mod_ai_batch.conf + mod_ai_batch.data 解析
├── file_counter.go        # 上传计数 Reader（字节/行数，O(1) 内存，§7）
├── resp_capture.go        # 小 JSON 响应捕获解析（上传/创建/查询/取消，上限 1MB）
├── result_parse.go        # 下载 jsonl 按行流式 usage 解析（单行缓冲上限，§7）
├── batch_state.go         # BATCH_FILE / BATCH_TASK / BATCH_ACTIVE 的 Redis Lua 读写
├── export.go              # 跨模块导出（BatchActiveCount/BatchGlobalFileCeilings，供 mod_ai_rate_limit）
└── metrics.go             # module_state2 计数器 + Prometheus 指标
```

### 5.2 回调挂载点（全部同步回调，无异步化）

| 回调 | 执行序约束 | 职责 |
|---|---|---|
| `HandleAfterAITargetModel` | 在 token_auth、ai_rate_limit 之后（key 已选定、批量限流已执行） | 操作分类；前置检查（上传 CL 超限 413、创建/下载归属校验读）；创建请求体读 `input_file_id`（KB 级，复用 mod_ai_context 读请求体先例）；上传请求体挂计数 Reader |
| `HandleForward` | 与 traffic_mirror 同级 | 上传计数 Reader 最终安装点（backend 选定后、写上游前），确保绕过重试缓冲包装 |
| `HandleReadResponse` | body_process 之前注册（外层包装） | 按操作装响应包装：小 JSON 捕获 / jsonl usage 解析（下载） |
| `HandleRequestFinish` | token_auth 保持 AI 链最前；本模块在其后 | 只做 Redis 簿记：绑定/任务状态/`BATCH_SETTLED`；填 `AiBasicInfo` 批量字段 |

### 5.3 分操作时序

| 操作 | 转发前 | 上游响应后（Finish 前） | Finish 动作 |
|---|---|---|---|
| 上传 files | CL 超限 413；挂计数 Reader | 捕获 `{id, purpose, bytes}`（失败响应不处理） | 写 `BATCH_FILE`；fail-open：Redis 失败放行 + 计数，file_id 已随访问日志落管道由控制面补登 |
| 创建 batches | 读请求体得 `input_file_id`；查 `BATCH_FILE` 行数；余额预检（不足拒绝，不发生转发） | 捕获 `{id, status, output_file_id?}`；**仅上游 2xx 后**写预留（两阶段：先检后留） | 写 `BATCH_TASK` + `BATCH_ACTIVE`；写 batch/file 粘性绑定 |
| 查询 batches/{id} | 读 `BATCH_TASK` 供粘性选 key | 捕获 `{status, output_file_id?, request_counts?}` | 更新任务状态；首次发现 output_file_id 补写 `BATCH_FILE`（dir=output）；expired/failed/cancelled → 幂等释放预留（completed 不释放，等结算） |
| 下载 content | 归属校验：绑定存在且 api_key 不匹配 → 404；绑定 miss → 按 `OwnerCheckMissPolicy` 放行+告警 / 拒绝 | jsonl 按行解析 usage（流式，响应原样透传）；解析完成将 `{batch_id, model, usage}` 写 `AiBasicInfo` | token_auth 依契约 batch 价结算 + 释放预留 + `SETNX BATCH_SETTLED`；本模块做任务簿记 |
| 取消 cancel | — | 捕获 batch 对象；仅上游接受才释放预留 | 任务标记 cancelling（终态以 provider + 控制面确认为准） |

解析边界：只提取控制面所需字段（file_id/batch_id/status/output_file_id/request_counts/usage），不解析其它字段；不改写请求/响应体。

操作分类实现位于 `bfe_basic/batch_op.go`（`ClassifyBatchOp`/`BatchPathFileId`/`BatchPathBatchId`）：mod_ai_batch 与 mod_ai_rate_limit 都需要按 (method, path) 判定批量操作，下沉到 bfe_basic 避免重复实现与口径分叉；`AiBatchRouteHint` 及其存取（`bfe_basic/ai_batch_hint.go`）供 mod_ai_batch 把 batch/file 标识传给 reverseproxy 的粘性选择，bfe_server 不 import 模块包。

## 6. 批量计费（FR-2，核心）

### 6.1 分操作计费语义

| 操作 | mode | 请求时行为 |
|---|---|---|
| 上传 files | file | 不计费；流式计数 |
| 查询/删除 files | file | 不计费 |
| 下载 files/{id}/content | file | 批量输出文件：按行解析 usage 汇总，**按 batch 价结算 + 释放预留**；普通文件不计费 |
| 创建 batches | batch | 余额预检 + 预留；不计费 |
| 查询 batches | batch | 不计费；响应拦截更新上下文 |
| 取消 cancel | batch | 释放预留（不结算） |

批量价：`LookupModelPrice(cluster, model, "batch")` 查显式价格行（控制面导出时把折扣系数展开为 batch mode 价格行，数据面只查表不算折扣）；查价 miss 沿用现有"0 费 + `PriceLookupMiss` 计数"语义，不回退 chat 价。定价函数在 `calcChatCost`（`mod_ai_token_auth.go:670`）基础上加 batch 分支（input/output/cache token 键 + tier，与 chat 同构）。

### 6.2 与 token_auth 的契约（不新增跨模块调用，不动注册顺序）

沿用现有"usage 生产者 → `AiBasicInfo` → token_auth 统一计价"契约（与 `QuotaUsageProcessor` 填 `TokenUsage` 同构）：

- 下载 jsonl 解析器完成时把 `{batch_id, target_model, usage, settle_kind}` 写入 `AiBasicInfo`。完成时机**按响应 Content-Length 驱动**（最后一块字节即触发；代理不会读超 CL 的尾随 EOF，纯 EOF 触发会永不执行），严格先于响应完成与 FinishReq；
- token_auth 的 Finish 处理器检测 settle 标记 → 批量分支：batch 价计价、扣 `QUOTA_*`、释放该 batch 的 `BATCH_RESERVE` 占用、结算去重，同一 Lua 内完成——**一笔请求仍只有一个扣费决策点**，访问日志 `ai_cost_value` 由 token_auth 正常产出；
- 创建/查询/取消触发的预留与释放不在请求计价路径上，由本模块调 token_auth 暴露的三个原语：`BatchPreCheck(apiKeyId, units)`（只读预检）/ `BatchReserve(apiKeyId, units, batchId)` / `BatchRelease(apiKeyId, units, batchId)`（内部与 RMB 扣减同 Lua 语义，维护 `BATCH_RESERVE` 镜像计数）。

### 6.3 Redis 键结构（与控制面 `QUOTA_*` 同集群、同 1e-8 定点口径）

**as-built 契约**（以 `mod_ai_batch/batch_state.go` 与 `mod_ai_token_auth/batch_quota.go` 实现为准）：

```
BATCH_FILE:<cluster>:<file_id>    HASH {api_key_id, key_name, lines, bytes, purpose, dir}   TTL 48h
                                  ※ 键中段为路由 cluster 名（BFE 运行时概念）；控制面
                                    读回后由 provider 列承载该值
BATCH_TASK:<batch_id>             HASH {api_key_id, cluster, key_name, input_file_id, est_lines,
                                      reserved_units, status, usage_in, usage_out,
                                      settle_units, settle_status}   TTL 48h
                                  ※ 下载拦截结算时原地写 usage_in/usage_out/
                                    settle_status=settled，控制面 job 从本键同步任务实体
BATCH_RESERVE:<api_key_id>        预留占用镜像计数（可用余额 = QUOTA_* 剩余 − 本计数）
BATCH_RESERVE_BATCH:<batch_id>    HASH {planRedisKey -> 各配额计划预留 units}
                                  ※ 按 batch 逐计划簿记：释放/结算原子扣镜像 + 删簿记，
                                    簿记空即幂等（token_auth 侧，见 batch_quota.go）
BATCH_SETTLED:<batch_id>          STRING "1"，SET NX EX   TTL 48h（结算/释放共用去重占位）
BATCH_ACTIVE:<api_key_id>         ZSET member=batch_id score=created_unix，限流维度
```

### 6.4 金额语义

- 预留（实现期修正：**价格无关的两路估算**，因 batch 创建请求体只有 endpoint 没有 model，无法按模型定价）：
  - RMB 路：预留额 = 输入文件行数 × `ReservePerLineMicros`（默认 0.002 元/行，1e-6 元定点，写入时 ×100 转 1e-8 配额单位）；
  - token 路：预留额 = 行数 ×（`ReserveInputTokens + ReserveOutputTokens`，默认 8192+4096），用于 total_token 配额计划；
  - 行数取自上传时 `BATCH_FILE` 元数据；查不到时按 1 行兜底（低估场景由结算修正 + `over_reserved` 标记）。
- 预检：`QUOTA_*` 剩余 − `BATCH_RESERVE` 占用 < 预留额 → 拒绝（`CodeQuotaExhausted` 语义）；`pass_when_no_enough_quota` 计划语义对批量同样生效。
- 结算：下载流按行解析 usage，**按行内 model 分组**，逐组查 `(cluster, model, mode=batch)` 价格计价（复用 `calcChatCost` 逻辑），汇总实扣；任一模型组查价 miss 计 0 并打 `PriceLookupMiss`（不回退 chat 价）。实际超出预留时不发生真透支——配额扣减 Lua 语义为 `min(余额, 应付)`（与存量扣减一致），差额由控制面在对账报表中按 `over_reserved` 口径呈现（实现期修正，原文档"允许透支"与存量扣减原语冲突）。
- 释放：cancel/expired/failed 无结果文件 → 整额释放；completed → 释放预留后按实结。

## 7. 大文件直转（FR-4）

现状事实：body ≥ `AccessibleBodySize`（默认 2MB、硬上限 8MB，`bfe_http/request.go:1004-1005`）时 `prepareRequestBodyForRetry`（`reverseproxy.go:1886-1921`）包装后实际不可回绕 → fallback 禁用（`:1284-1290`）、key 重试禁用（`:1702-1711`）；`MaxRetries` 默认 0。大文件今天本来就不重试——本方案显式化并加保护：

1. **ModeFile/ModeBatch 请求一律跳过重试包装**（显式短路，不依赖 size 判断），`TotalBodyBufferSize` 全局缓冲对批量路径完全不参与。
2. 上传：**网关层已缓冲体的前置拒绝 + 计数 Reader 双重防线**。`extractClientModel`（`http_conn.go:522`）对所有 AI 请求执行 `ReqBodyJsonFetch`，body 已被包装成 bytes_body 并缓冲（≤AccessibleBodySize，默认 4MB）——mod_ai_batch 在 `preCheckUpload` 先对**未包装前**的 body 取 `GetBytes`（all=true 即完整缓冲，chunked ≤4MB 同理）做精确行/字节计数，超有效上限直接 413（转发都不发生）；随后安装计数 Reader，覆盖 >4MB 文件的流式尾部（超限中断上游写入）。注意必须先取缓冲检查再装包装器（对已包装 body 再取 accessor 会二次缓冲且 `all=false` 误判，SC28 TC-10 暴露）。
3. 下载：逐行 bufio 扫描解析 usage，单行解析缓冲上限 `MaxLineParseBytes`（防异常行撑爆内存）；解析只影响计费，**响应字节原样流给客户**（处理链只读副本）。
4. ModeFile/ModeBatch 响应不进 `QuotaUsageProcessor`（SSE 帧解析器会把 jsonl 当 SSE 误解析）；下载响应 usage 由 mod_ai_batch 的 jsonl 解析产出——两套 usage 来源按 mode 互斥，同一响应只有一个 usage 生产者。

## 8. 粘性与资源归属（FR-3）

现状事实：AIKey SessionAffinity（`reverseproxy.go:1923-2228`）绑定键 `bfe:ai:key_affinity:<cluster>:<sessionID>`、TTL 600s 命中续期（滑动）、Redis 全 fail-open、惩罚 429→60s / 401-403→3600s。600s 客户端亲和与 24h 批量完成窗口不匹配——两级粘性：

1. **batch 级绑定（新增，主）**：命名空间 `bfe:ai:key_affinity:batch:<cluster>:<file_id|batch_id>`（`bfe_basic.BatchAffinityKey`，**刻意不跟随可配置的 SessionAffinityRedisPrefix**，避免互相遮蔽），由 mod_ai_batch 在 Finish 时 `Setex` 写入，reverseproxy 的 `chooseAIKeyWithAffinity` 优先读取（命中滑动续期 48h，TTL 为固定常量 `batchAffinityTTLSec=172800`，**未做成 conf 项**——P0 取舍，见 §10.3）。链式维护：上传响应写 `file:<input_file_id>`；创建读 `file:<input_file_id>` 影响选 key（miss 回退客户端亲和）、响应后写 `batch:<batch_id>` 与 `file:<output_file_id>`；查询/下载按 `batch:<id>`/`file:<id>` 优先选 key。`chooseAIKeyWithAffinity` 增加"batch 绑定优先于客户端亲和"分支。
2. **客户端亲和（现有，兜底）**：无 batch 绑定的首步操作仍走 600s 客户端亲和。

failover：batch/file 的 GET/cancel 在 key 上 404（渠道类 provider 资源挂在单 key 的典型症状）→ 删绑定 + 60s penalty（`setKeyPenalty` 现有机制），fail-open 换 key；GET 幂等安全，cancel 幂等由 provider 保证。降级开关 `BatchAffinityMode: file|client`（账号级资源归属的 provider 可降级为纯客户端亲和）。

归属校验（安全要求）：下载时查 `BATCH_FILE` 绑定，存在且 api_key 不匹配 → 404（不暴露存在性）；miss 按 `OwnerCheckMissPolicy` 处理（默认 `allow_log` 放行+计数告警，可配 `deny`）。

## 9. 限流（FR-6）

### 9.1 两层配置位置

**① 控制面 `rate_limit_policies`**（API/DB 层，`RateLimitPolicyParam` 新增 `batch_limits`，DB 加可空 JSON 列）：

```jsonc
{
  "enabled": true,
  "max_concurrency": 50,
  "tpm_configs": [ ... ],
  "rpm_configs": [ ... ],
  "batch_limits": {                  // 可为空：省略/null = 该策略不参与批量限流
    "max_create_rpm": 10,            // 单 apikey 批量创建 RPM
    "max_active_batches": 5,         // 单 apikey 在途 batch 上限（BATCH_ACTIVE ZSET 基数）
    "max_file_bytes": 104857600,     // 100MB
    "max_file_lines": 50000          // 对齐 OpenAI 单文件行数上限
  }
}
```

四个维度全部按 apikey 计、与 model 无关（batch 创建请求体只有 endpoint 没有 model，模型在输入文件行内）。**可为空语义**：`batch_limits` 省略/null = 该策略不参与批量限流（导出 `omitempty` 整体省略）；单个维度省略/为 0 = 不配此维度上限；存量策略零迁移（可空列，默认 NULL 行为不变）；全空 ≠ 无限制——仍受 `mod_ai_batch.data` 全局硬顶约束。

**② BFE 导出 `ai_rate_limit.data`**（每条策略 `rules` 内、`max_concurrency` 之后生成 `batch` 段）：

```jsonc
"rules": {
  "tpm": [ ... ],
  "rpm": [ ... ],
  "max_concurrency": 50,
  "batch": {
    "max_create_rpm": 10,
    "max_active_batches": 5,
    "max_file_bytes": 104857600,
    "max_file_lines": 50000,
    "redis_key": "RL_BATCH_rlp-0001_rpm"   // 仅创建 RPM 计数需要；文件上限为本地校验、
                                           // 在途上限复用 BATCH_ACTIVE ZSET，均不重复配 key
  }
}
```

### 9.2 多策略命中组合语义

沿用现有 tpm/rpm/concurrency 多策略语义（`mod_ai_rate_limit.go:177-216` 逐策略检查、任一失败即拒绝、错误带策略名）：

1. **逐策略 AND**：仅对配置了 `batch` 段的策略逐个检查，任一策略任一维度超限即拒绝（`Rate limit exceeded for policy %s`，limit_type 新增 `batch_file`/`batch_rate`）。
2. **等效取 min**：`max_file_bytes`/`max_file_lines`/`max_active_batches` 效果等于已配置策略中取最小值；`max_create_rpm` 计数器按策略独立（各一个 redis_key），不合并。
3. **Models 过滤不适用**：策略的 `Models` 只约束 tpm/rpm/concurrency，不约束 batch 段——策略启用则其 batch_limits 对该 apikey 全部批量操作生效。
4. **全局硬顶兜底**：有效上限 = min(各策略配置…, `mod_ai_batch.data` 全局硬顶)；策略均未配 batch 段时仅全局硬顶生效（防误配敞口）。
5. **单一计算点**：mod_ai_rate_limit 在 `HandleAfterAITargetModel` 算出有效上限写入 `AiBasicInfo`，mod_ai_batch 的前置 413 与 chunked 流式中断读同一组值。

TPM 预占/回补对批量流量不适用（创建时无 prompt token），金额风险由 §6 预留机制承担——配置注释写明边界。

实现落点（`mod_ai_rate_limit/batch_check.go` + `policy_limiter.go` + `data_load.go`）：

- `batch` 段解析进 `LimitRulesConf.Batch`；`max_create_rpm` 用 `QPMLimiter`（60 秒窗口，key 取导出 `redis_key`，缺省 `default_bfe_<policyId>_batch_rpm`）；
- `max_active_batches` 调 `mod_ai_batch.BatchActiveCount(apiKeyId)`（共享 `BATCH_ACTIVE` ZSET，24h 滑窗自动清老条目）；
- `max_file_bytes` 已知 CL 时在 `HandleAfterAITargetModel` 拒绝（chunked 由 mod_ai_batch 计数 Reader 按有效上限中断）；`max_file_lines` 同理只作用于流式中断；
- 有效上限单一计算点：`resolveBatchFileLimits` 在策略循环结束后把 min(各策略, `mod_ai_batch.data` 全局硬顶) 写入 `AiBasicInfo.BatchEffMaxFileBytes/Lines`；
- 拒绝响应：bytes/lines 命中用 `BATCH_FILE_TOO_LARGE`（limit_type=`batch_file`），RPM/active 命中用 `RPM_LIMIT_EXCEEDED`，错误信息带策略名与维度。

## 10. 配置样例

### 10.1 `conf/mod_ai_batch/mod_ai_batch.conf`

只保留启动期固定项。**注意：本代码库中 `.conf` 仅 Init 时加载一次**（各模块 `reloadHandlers()` 只注册 data 文件加载函数，见 `mod_ai_rate_limit.go:440-446`、`mod_traffic_mirror.go:185-191`）——凡需动态调整的参数一律放 `mod_ai_batch.data`。

```ini
[basic]
# Redis 客户端复用 bfe.conf [AIKeyAffinity] 的连接（BATCH_* 键与其同集群，见 §6.3）
# 本文件无需 Redis 连接段

[log]
OpenDebug = false
```

### 10.2 `conf/mod_ai_batch/mod_ai_batch.data`

全部可调参数集中于此，经 `reloadHandlers` 注册热加载（conf-agent 标准 prober → file_store → trigger 流程），**改完即生效、无需重启进程**：

```json
{
    "Version": "1.0",

    "MaxFileBytes": 1073741824,
    "MaxFileLines": 100000,

    "ReserveInputTokens": 8192,
    "ReserveOutputTokens": 4096,
    "ReservePerLineMicros": 200000,
    "MaxLineParseBytes": 1048576,
    "OwnerCheckMissPolicy": "allow_log"
}
```

| 字段 | 说明 |
|---|---|
| `MaxFileBytes`/`MaxFileLines` | 全局文件硬顶（上传/下载双向生效，与策略 `batch_limits` 取 min；`0` 或负数 = 关闭，生产不建议） |
| `ReserveInputTokens`/`ReserveOutputTokens` | 预留估算的 token 路上限（total_token 配额计划用，§6.4） |
| `ReservePerLineMicros` | RMB 路预留单价（1e-6 元/输入行，默认 0.002 元；写入配额时 ×100 转 1e-8 单位） |
| `MaxLineParseBytes` | 下载结果文件单行解析缓冲上限（防异常行撑爆内存） |
| `OwnerCheckMissPolicy` | 归属校验 miss 策略：`allow_log` = 放行+计数告警（默认）；`deny` = 直接 404 |

### 10.3 batch 绑定 TTL

固定常量 48h（`bfe_server/reverseproxy.go` 的 `batchAffinityTTLSec`），不随 bfe.conf 配置。理由：batch 绑定服务于 24h 完成窗口 + 24h 下载宽限，业务上不需要按部署调节；做成 conf 项需穿透 AIKeyPolicy 导出 schema（控制面联动），P0 不做，二期按需补。

热加载：`mod_ai_batch.data` 经 `MonitorHandlers`/`reloadHandlers` 注册热加载（conf-agent 标准 prober → file_store → trigger 流程，返回 `文件名=Version`），可调参数改完即生效、无需重启。

## 11. 访问日志与监控

### 11.1 访问日志字段（`bfe-access-pb`）

`AiBasicInfo` 新增：batch_id、file_id、batch_op（upload/create/get/list/cancel/download）、file_lines、file_bytes、batch_status、batch_settle（settle/release/none）。protobuf 只增不删（旧 log-reader 忽略新字段），编号以 `bfe_access.proto` 实际空闲为准。log-reader 映射与 Doris/CK 加列在 observability 仓库跟进。

### 11.2 模块计数器（module_state2）

P0 提供 module_state2 计数器（`/{mod}.status` 与 `/{mod}.status.diff` 可查；`mod_ai_batch` 的 CounterKeys 见 `mod_ai_batch.go`）：REQ_TOTAL、FILE_BIND_WRITE(_FAIL)、TASK_WRITE(_FAIL)、UPLOAD_REJECT_SIZE、UPLOAD_ABORT_STREAM、OWNER_CHECK_REJECT/MISS、RESERVE_OK/RESERVE_PRECHECK_MISS、RELEASE_OK、SETTLE_USAGE_PARSE(_ERR)、REDIS_ERR。

Prometheus 指标（`bfe_ai_batch_*` 系列）P0 未做：批量是低频控制面操作，module_state2 + 访问日志字段已覆盖观测需求；二期按需仿 `mod_traffic_mirror/mirror_state.go` 补。

## 12. 兼容性与非功能性

| 维度 | 设计 |
|---|---|
| 存量行为 | 未配置 batch 价 → 查价 miss 计 0 费 + `PriceLookupMiss` 计数（不回退 chat 价）；未启用 mod_ai_batch → files/batches 仅被识别为 file/batch mode，透传不变、按现有 mode 语义计 0 |
| 未识别路径 | 行为完全不变（仍全量透传、默认 chat） |
| 主链路性能 | 批量操作均为低频控制面操作；同步路径只加一次操作分类 + 小 JSON 解析；100MB 文件内存曲线平坦（§7 流式） |
| 故障隔离 | 批量路径所有 Redis 操作 fail-open + 计数告警；Redis 全不可用时粘性/预留/限流降级放行，结算延迟至恢复，对账由控制面闭环 |
| pb 混布 | 只增字段，新老版本兼容 |

## 13. 开发任务拆分（WBS）

BFE 侧任务（本文档范围）：

| 编号 | 任务 | 主要改动文件 |
|---|---|---|
| 1 | Mode 枚举 + `openAIEndpointModes` 扩展 + 单测 | `bfe_basic/openai_endpoint.go`、`bfe_basic/request_ai_basic.go` |
| 2 | `mod_ai_batch` 骨架：模块入口、回调注册、conf/data 加载、操作分类 | `bfe_modules/mod_ai_batch/*`、`bfe_modules/bfe_modules.go`、`conf/mod_ai_batch/*` |
| 3 | 大文件直转：跳过重试包装、上传计数 Reader、下载 jsonl 流式解析 | `bfe_server/reverseproxy.go`、`mod_ai_batch/file_counter.go`、`result_parse.go` |
| 4 | 批量上下文：响应捕获解析、BATCH_* Redis 状态、归属校验 | `mod_ai_batch/resp_capture.go`、`batch_state.go` |
| 5 | 计费契约：token_auth 批量查价分支 + `BatchPreCheck/Reserve/Release` 原语 + AiBasicInfo settle 契约 | `mod_ai_token_auth/*`、`bfe_basic/request_ai_basic.go` |
| 6 | batch 级粘性绑定 + 404 惩罚 + `BatchBindingTTL` | `bfe_server/reverseproxy.go`、`conf/bfe.conf` 样例 |
| 7 | 限流：`ai_rate_limit.data` batch 段解析 + batch 检查器 + 有效上限写 AiBasicInfo | `mod_ai_rate_limit/*`、`conf/mod_ai_rate_limit/ai_rate_limit.data` 样例 |
| 8 | 访问日志字段（pb + access_pb3） | `bfe-access-pb/bfe_access.proto`（`sh build.sh` 生成，不得手改）、`mod_access_pb3/request_log.go` |
| 9 | 监控指标 | `mod_ai_batch/metrics.go` |
| 10 | 单元测试 | `*_test.go`（testing + testify） |
| 11 | 集成测试（独立仓库） | `integration-test/`：全生命周期功能等价、计费对账、粘性四步同 key、并发不退化、100MB 内存曲线、负例 |

依赖：1 → 2 → 3/4 → 5/6/7 → 8/9 → 10/11。任务 8 依赖 `bfe-access-pb` 先行合入。落码遵循 `bfe/AGENTS.md`：新文件 Apache 2.0 license 头、conf/ 样例同步、`make test` 通过。

配套仓库任务：`ai-gateway-api`（批量价导出、batch_tasks/batch_files 表与管控 API、对账 job）、`ai-gateway-web`（批量任务页）、`ai-gateway-observability`（报表加列与批量指标）。

## 14. 风险与应对

| 风险 | 应对 |
|---|---|
| 批量价未配置导致 0 费放行 | `PriceLookupMiss` 计数 + 告警接入监控；上线 checklist 要求配价 |
| 预留低估透支余额 | 照实结算 + `over_reserved` 标记进对账报表（不静默截断）；预留系数可调 |
| 大文件打爆内存/带宽 | 全局硬顶 + 策略 min 双重上限；流式 O(1) 内存；chunked 超限中断 |
| 渠道类 provider 资源挂在单 key 导致 404 | batch 绑定主粘性 + 404 删绑定/惩罚/换 key；`BatchAffinityMode` 可降级 |
| jsonl 解析失败 | 放行 + 计数，控制面 job 经 provider 对账兜底结算，对账误差以 job 为准 |
| Redis 不可用 | 全 fail-open + 计数；任务实体与终态对账由控制面 DB 闭环 |
| 响应解析影响下载吞吐 | 单行缓冲上限 + 只读副本零拷贝透传；压测验证 P99 不退化 |
