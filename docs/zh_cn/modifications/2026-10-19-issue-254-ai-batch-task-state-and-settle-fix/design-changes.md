# 批量任务终态字段与下载结算修复（BATCH_TASK 状态落库 + jsonl usage 解析）

日期：2026-10-19

状态：**代码与测试已实现并验证（2026-10-19，见 §八实施记录）**。本文件是 BFE 数据面侧本次变更的完整说明（根因核实／改动点／测试／边界）；
配套的 `ai-gateway-api` 控制面改动见 §四，**尚未实施**（另一仓库，方案见该仓库 `design-docs/modifications/2026-10-19-issue-254-batch-task-state-and-settle-fix/`）。

- Issue：[ai-gateway-api#254](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/254)（缺陷与证据在控制面链路被发现，根因主要在 BFE 数据面）
- 代码库：`bfe/`（主责）+ `ai-gateway-api/`（配套，§四）
- 关联：`2026-10-07-ai-batch-support`（批量一期实现，本文档修正其数据面实现缺口）、[bfenetworks/bfe#1426](https://github.com/bfenetworks/bfe/issues/1426)（同链路 multipart 问题）
- 行号基于当前 HEAD（2026-10-19 逐条核实）

---

## 一、缺陷摘要

批量链路（上传 → 建批 → 轮询 → 下载输出文件）在数据面可用，但**批量任务的终态事实没有落到 Redis `BATCH_TASK`**，且**下载输出文件没有触发结算**。控制面 `GET /open-api/v1/batches/{id}` 因此对同一 completed 批次缺失
`entity_id`/`endpoint`/`output_file_id`/`request_counts`/`usage_input_tokens`/`usage_output_tokens`/`usage_source`/`reserve_units`/`settle_units` 九个字段，`settle_status` 永驻 `reserved`，`batch_files` 只登记输入文件。

同时暴露两个统计失真：输入文件 1 行 / 206 字节，`BATCH_TASK.est_lines` 记为 11、`BATCH_FILE.lines=11`/`bytes=509`——统计对象是出站 multipart 整体（含 part 头与 boundary），不是 `file` part 的文件内容，直接放大预扣额度。

## 二、根因（当前 HEAD 逐条核实）

### 2.1 下载结算不触发：jsonl usage 解析结构与输出契约不符

`bfe_modules/mod_ai_batch/resp_capture.go:180-222` 的 `handleLine` 按**扁平**结构解析：

```go
var row struct {
    Response struct {
        Model string `json:"model"`
        Usage *struct { ... } `json:"usage"`
    } `json:"response"`
}
```

而 OpenAI Batch 结果文件（受控后端实测、issue 证据）的 usage 位于 `response.body.usage`、model 位于 `response.body.model`：

```json
{"custom_id":"...","response":{"body":{"model":"...","usage":{"prompt_tokens":10,...}},"status_code":200}}
```

`row.Response.Usage == nil` 成立 → `:200` 直接 return → `ctx.usageParsed` 永不为真 → `finalize`（`:132-135`）首行 `if !u.ctx.usageParsed || u.aiMeta == nil { return }` 跳过全部副作用：
`SETTLE_USAGE_PARSE` 不 +1、`BATCH_SETTLED` 不写入、`usage_in`/`usage_out`/`settle_status` 不落 `BATCH_TASK`、`aiMeta.BatchSettle` 不置 `settle`，于是 `mod_ai_token_auth` 的批量结算分支（`batch_quota.go:224-325`）根本不会被触发，下载输出文件对**该批次**不产生任何结算副作用（issue 实测下载前后 `REQ_TOTAL` 87→88 证明模块已命中，而 `SETTLE_USAGE_PARSE=0`、`SETTLE_USAGE_PARSE_ERR=0`、`BATCH_SETTLED:<batch_id>` 不存在）。

单元测试夹具同样按错误契约书写（`mod_ai_batch_test.go:88-90`），所以缺陷在测试中不可见。

另有一处观测缺口：`SETTLE_USAGE_PARSE_ERR` 只在 state 列表里声明（`mod_ai_batch.go:76`），全仓**没有任何 `Inc` 调用**——解析失败的行使信号缺失，这正是 issue 观察到"两个计数器同时为 0"的原因之一，修复时须一并接入。

### 2.2 终态字段不落 `BATCH_TASK`

`mod_ai_batch_handlers.go:444-466`（`finishGet`）只写一个字段：

```go
m.writeTaskRecord(br.Id, map[string]string{"status": br.Status}, ttl)
```

`br.OutputFileId` 仅用于写 `BATCH_FILE` 输出绑定（`:457-461`），**从不写回任务 hash**；`request_counts` 在 `bfe_modules/` 全仓无任何引用（`batchResponse` 结构 `resp_capture.go:236-241` 也没有该字段）。

`mod_ai_batch_handlers.go:410-418`（`finishCreate`）的字段集为
`api_key_id/cluster/key_name/input_file_id/est_lines/status/reserved`，同样缺 `reserved_units`、`created_at`、`endpoint`。

而 `batch_state.go:30-35` 的键结构注释已把 `reserved_units`/`usage_in`/`usage_out`/`settle_units`/`settle_status` 声明为契约——**注释先行、实现未跟上**；`request_counts`/`endpoint`/`created_at` 则在数据面任何位置都不存在。

### 2.3 读写视图缺字段

`batch_state.go:60-69`（`batchTaskMeta`）与 `:209-230`（`readTaskRecord`）只读 `api_key_id/cluster/key_name/input_file_id/est_lines/status/reserved`。即便 hash 里有新字段，模块自身也读不出（当前读路径主要用于判定 `est_lines`/归属，影响面小，但补字段时须同步，避免后续使用者再踩）。

### 2.4 `settle_units` 与 `settle_status=released` 无人回写

- 结算额：`mod_ai_token_auth/batch_quota.go:249-318` 的 `batchSettle` 已算出 `costUnits`（1e-8 RMB，`tu.UsedCost = costUnits` 于 `:323`），但**不写 `BATCH_TASK`**；`settle_units` 在 `bfe_modules/` 全仓无写入点。
- 释放态：`batch_quota.go:184-217`（`BatchRelease`）只删 `BATCH_RESERVE_BATCH` 与写 `BATCH_SETTLED`，不写 `settle_status=released`。
- 顺序缺陷：`resp_capture.go:146-158` 在**扫描完 body 时**就把 `settle_status=settled` 写进 hash，而 SETNX 抢占发生在之后的 `batchSettle`——若抢占失败（对账 job 已结算），hash 仍被标为 settled，形成"未真正结算却标 settled"的窗口。

### 2.5 行/字节按出站 multipart 整体统计

`file_counter.go:43-61` 的 `countingBody` 包在 `req.OutRequest.Body` 上（`mod_ai_batch_handlers.go:124`），统计**整个出站请求体**的字节与换行；`finishUpload`（`:372-380`）原样写进 `BATCH_FILE.lines/bytes`，`preCheckCreate`（`:180-188`）再读它得 `estLines` 参与预留额计算。
multipart 的 `--boundary`、`Content-Disposition`/`Content-Type` 头行、空行全部被计入——issue 实测 1 行/206 字节被记为 11 行/509 字节（+10 行、+303 字节）。

同类问题还有一处：`preCheckUpload:112-123` 的**前置限额检查**同样对整体缓冲体 `bytes.Count(buf, "\n")`，限流判定口径与文件内容不一致。

### 2.6 `endpoint` / `filename` 数据面未采集

`preCheckCreate:166-172` 只从建批请求体里取 `input_file_id`，同体里的 `endpoint`（透传端点，如 `/v1/chat/completions`）被丢弃；`parseFileResponse`（`:243-255`）只取 `id`/`purpose`/`bytes`，`POST /v1/files` 响应里的 `filename` 被丢弃。`endpoint` 在 provider 的 batch 对象里也有（控制面状态推进时可补，§四-2），但**数据面是最省一次出网的权威来源**；`filename` 则只有 File 对象能给出，控制面必须靠额外 `GET /v1/files/{id}` 才能拿到。

## 三、BFE 侧修复步骤

设计原则：

1. **`BATCH_TASK` 的写入点收敛**：非终态/创建期字段在 `finishCreate`/`finishGet` 写；结算类字段（`usage_in`/`usage_out`/`settle_units`/`settle_status`）**统一在 `HandleRequestFinish` 写**——已核实模块注册顺序（`bfe_modules.go:155` token_auth 在 `:202` mod_ai_batch 之前），finish 阶段 token_auth 先执行并完成结算/释放决策，mod_ai_batch 随后写库，天然拿到"是否真的结算成功"的事实，消除 §2.4 的顺序缺陷。
2. **口径以 provider 返回为准**：文件字节数、行数、usage、model 均优先取 provider 响应/结果文件，不再从 multipart 信封推断。
3. **不改计费规则**：批量价、预留公式、`BATCH_SETTLED` 去重语义、fail-open 策略全部保持原样。

### 步骤 1（必改）：jsonl usage 解析支持 `response.body.usage`

文件：`bfe_modules/mod_ai_batch/resp_capture.go`。

`handleLine` 改为双形态解析，内层优先 `response.body`，回退 `response` 扁平形态（兼容既有非标准上游与历史夹具）：

```go
var row struct {
    Response struct {
        Model string          `json:"model"`
        Usage *usageFields    `json:"usage"`
        Body  *struct {
            Model string       `json:"model"`
            Usage *usageFields `json:"usage"`
        } `json:"body"`
    } `json:"response"`
}
```

判据：`body.usage` 非空优先；两者皆空视为无 usage 行（错误行），跳过。`model` 同源（`body.model` 优先，缺省 `unknown`）。

同时改写 `mod_ai_batch_test.go:88-90` 夹具为 `response.body.usage` 真实契约，并**新增一条扁平旧格式兼容用例**，防止回归时把两种形态任一改坏。

同时把两个异常信号拆开接线（实施定稿，见 §八）：`SETTLE_USAGE_PARSE_ERR` 只计**整行 JSON 解析失败**；`SETTLE_NO_USAGE` 计**存在 `response` 对象但两条路径都取不到 usage**。`response: null` 的错误行两者都不计——它是正常业务行的形态，计入会把信号打满。

### 步骤 2（必改）：终态字段落 `BATCH_TASK`

文件：`bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go`、`resp_capture.go`、`batch_state.go`。

**2.1 `batchResponse` 扩容**（`resp_capture.go:236-241`）：

```go
type batchResponse struct {
    Id            string          `json:"id"`
    Status        string          `json:"status"`
    InputFileId   string          `json:"input_file_id"`
    OutputFileId  string          `json:"output_file_id"`
    Endpoint      string          `json:"endpoint"`
    RequestCounts json.RawMessage `json:"request_counts"`
    CreatedAt     int64           `json:"created_at"`
}
```

`request_counts` 用 `json.RawMessage` 原样保存（契约要求"provider 返回的请求计数原样"），落 hash 时做一次紧凑化即可，不做字段重排。

**2.2 `finishCreate` 补字段**（`:410-418`）：追加
`endpoint`（取自 `ctx.endpoint`，见步骤 4）、`output_file_id`（若创建响应已带，通常为空）、`request_counts`、`created_at`（provider 返回的 unix 秒）、`reserved_units`（`ctx.reserved ? ctx.rmbUnits : 0`——RMB 计划的预留额恰为 `rmbUnits`，见 `batch_quota.go:97-101` `reserveUnitsForPlan`；布尔 `reserved` 保留一版供兼容读取）。

**2.3 `finishGet` 补字段**（`:455`）：由
`{"status": ...}` 改为 `{"status", "output_file_id", "request_counts", "endpoint"}`（仅写非空值，避免用空串覆盖已有字段）。

**2.4 `batchTaskMeta` / `readTaskRecord`**（`batch_state.go:60-69`、`:209-230`）同步扩容，并更新 `:30-35` 的键结构注释为最终布局：

```
BATCH_FILE:<cluster>:<file_id>  HASH {api_key_id, key_name, lines, bytes,
                                purpose, dir, filename, batch_id}   TTL 48h
BATCH_TASK:<batch_id>           HASH {api_key_id, cluster, key_name,
                                input_file_id, output_file_id, endpoint,
                                request_counts, est_lines, reserved_units,
                                reserved, status, created_at, usage_in,
                                usage_out, settle_units, settle_status}
```

### 步骤 3（必改）：结算/释放字段在 `HandleRequestFinish` 统一回写

文件：`bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go`（`requestFinishHandler`，`:333-358`）、`bfe_basic/request_ai_basic.go`、`bfe_modules/mod_ai_token_auth/batch_quota.go`。

**3.1 从扫描器移除写库**：删除 `resp_capture.go:145-158` 的 `writeTaskRecord`（保留 `Inc("SETTLE_USAGE_PARSE", 1)` 与 `aiMeta.BatchSettle*` 契约发布），`usage_in`/`usage_out`/`settle_status` 改由 finish 阶段写。

**3.2 携带结算结果**：`AiBasicInfo`（`request_ai_basic.go:168-186` 批量字段段）新增两个字段：

```go
// BatchSettledUnits carries the RMB amount (1e-8 units) actually charged by
// the batch settlement, published by mod_ai_token_auth at request finish so
// mod_ai_batch can persist it as BATCH_TASK.settle_units.
BatchSettledUnits int64
// BatchSettleApplied reports whether this request actually applied the
// settle/release contract (SETNX claim won). False means another path
// already settled/released the batch: the task record must not be rewritten.
BatchSettleApplied bool
```

`batchSettle`（`batch_quota.go:224-325`）在写库成功后置 `BatchSettledUnits = costUnits` + `BatchSettleApplied = true`；`BatchRelease` 成功路径（`:184-217`，及 `batchContractHandler` 的 release 分支）置 `BatchSettleApplied = true`。

**3.3 finish 阶段写库**：`requestFinishHandler` 末尾按契约补写：

| 契约 | 写入字段 | 条件 |
| --- | --- | --- |
| `BatchSettleSettle` + `BatchSettleApplied` | `usage_in`、`usage_out`、`settle_units`、`settle_status=settled`、`usage_source=download` | `ctx.batchId != ""`；usage 取 `aiMeta.BatchUsageByModel` 汇总 |
| `BatchSettleRelease` + `BatchSettleApplied` | `settle_status=released` | `ctx.batchId != ""` |
| 两者之外 | 不写 | 未结算/未释放（对账 job 兜底） |

`usage_source=download` 是契约枚举 `download|reconcile` 的数据面来源：控制面目前**只会**写 `reconcile`（`settle.go:222`），`download` 在全仓无任何写入点，所以下载拦截结算成功的批次必须由数据面标注来源，否则该字段在正常路径上永远为空（issue 九个缺失字段之一）。

`ctx.batchId` 在下载路径由输出文件绑定解析（`preCheckDownload:227-229`）；绑定缺失时 settleKey 退化为 `file:<id>`，此时**无任务可回写**，仅完成 file 级结算——与现状一致，由控制面对账兜底。

### 步骤 4（必改）：文件行/字节改取 `file` part 与 File 对象

文件：`bfe_modules/mod_ai_batch/file_counter.go`、`mod_ai_batch_handlers.go`、`batch_context.go`。

**4.1 精确来源优先**：`finishUpload` 不再用 `ctx.uploadLines/uploadBytes` 直接落库，改为：

- `bytes`：取 `POST /v1/files` 响应的 `fileResponse.Bytes`（File 对象权威值；`resp_capture.go:230-234` 已解析）；
- `lines`：取 multipart `file` part 的内容换行数（4.2）；
- `filename`：取 `fileResponse.Filename`（新增字段）；
- 二者不可得时（如响应未带 `bytes`）才回退计数，并写 `bytes_src=approx`/`lines_src=approx` 供控制面对账时识别近似值。

**4.2 只统计 `file` part**：新增 `multipartPartStats(body []byte) (filename string, lines, bytes int64, ok bool)`，在 `preCheckUpload` 已整体缓冲的分支（`:112-123`）解析 `Content-Disposition: form-data; name="file"` 段的头部与内容，只对该段计数；限额检查亦改用该结果。流式未缓冲（chunked、超缓冲上限）时 `ok=false`，退回整包近似计数但**必须计数**（新增 state `UPLOAD_COUNT_APPROX`），使近似值可观测。

**4.3 `AiBasicInfo.BatchBytes` 语义固定**为"文件内容字节"（`request_ai_basic.go:172` 注释已如此声明，实现未遵守），访问日志随之纠正。

**4.4 `purpose` 保持 provider 原值**（`batch`），契约值 `batch_input`/`batch_output` 由控制面按 `dir` 映射（§四-3）；数据面不引入契约枚举，避免与 provider 方言耦合。

### 步骤 5（必改）：`endpoint` / `created_at` 采集

- `endpoint`：`preCheckCreate:166-172` 的匿名结构体加 `Endpoint string \`json:"endpoint"\``，存入 `ctx.endpoint`（`batch_context.go:26-64` 新增字段），由步骤 2.2 落库；`finishGet` 若 provider 响应带 `endpoint` 亦可用于校正。
- `created_at`：建批响应 `created_at`（unix 秒）落 hash，控制面据此填 `batch_tasks.created_at`，消除"回源窗口时间戳为零"（issue 次要观察项的数据面源头）。

### 步骤 6（必改）：配置与文档同步

- `bfe/conf/mod_ai_batch/mod_ai_batch.conf` 与 `mod_ai_batch.data`：本次不改配置项（无新增可调参数），仅确认注释无过期描述。
- `docs/zh_cn/modifications/2026-10-07-ai-batch-support/design-changes.md`：在实施记录中追加"§2 键结构已扩容"的说明，避免与实际布局冲突。
- `ai-gateway-api/design-docs/sys-design/details/批量任务与对账.md#§2`：`BATCH_TASK`/`BATCH_FILE` 键结构表需同步本文档 §三-2.4 的最终布局（其他仓库）。

## 四、配套修复（`ai-gateway-api`，另一批变更）

数据面写全也读不出来，以下 5 处必须同批修，否则 §三 的收益为零：

| # | 位置 | 问题 | 改法 |
| --- | --- | --- | --- |
| 1 | `model/ibatch/redis_view.go:49-91`、`:141-190` | `ParseRedisTaskView`/`applyTaskView` 只解析 status/est_lines/reserved_units/usage_in/usage_out/settle_units/settle_status，**不解析** `output_file_id`/`request_counts`/`endpoint`/`created_at`（DB 列已存在：`database/db_ddl_upgrade_to_0.0.11.sql:199-203`） | 按需补字段 + 保持"hash 未携带不覆盖 DB"的指针语义 |
| 2 | `model/ibatch/advance.go:56-59` + `job.go:142-160` | sync 先跑且会把 `completed` 直接落库，`AdvanceStatus` 只查 `StatusNotIn: TerminalStatuses` → 终态任务的 `output_file_id`/`request_counts` 永不被补 | 增加"终态但产物缺失"的补漏轮次（`completed AND (output_file_id IS NULL OR request_counts IS NULL)`），或让 `providerBatchResponse`/补漏路径同时写 `endpoint` |
| 3 | `endpoints/openapi_v1/batches/one.go:50-60`、`model/ibatch/types.go:122-144` | `batch_files` 由 `task.InputFileID`/`OutputFileID` 拼装，后者空 → 输出文件不出现；`ToMeta` 直传 `purpose`（provider 原值 `batch`），契约要 `batch_input|batch_output`；`batch_files` 表无 `filename` 列（DDL `:170-188`） | `purpose` 按 `direction` 映射；`filename` 加列并承接数据面新字段 |
| 4 | `model/ibatch/settle.go:387-410` | `handleUsageLine` 与 BFE 同款扁平解析缺陷 → 兜底结算解析出 0 token、`model` 空 → `settle_units=0` 却把 `settle_status` 置 `settled`（**静默漏结算，比不结算更糟**） | 与 §三-步骤 1 同步改为 `response.body.usage` |
| 5 | `model/ibatch/types.go:67`、`redis_view.go:49-91` | `usage_source` 的 `download` 取值**全仓无写入点**（`UsageSourceReconcile` 只在 `settle.go:222` 出现），且 `ParseRedisTaskView` 不解析 `usage_source` → 下载拦截结算成功的批次，该字段恒为空 | 数据面按 §三-3.3 写 `usage_source=download`，控制面补解析与落库 |

补充（非阻塞）：

- 回源窗口 `created_at`/`updated_at` 为零（`applyTaskView` 不设置时间字段）：数据面补 `created_at` 后可直取；仍缺失时保持现状或按同步时刻近似，issue 已注明不作为判据。
- 输出文件 `filename` 的权威来源只有 provider File 对象；控制面 `advance` 已出网，可选用 `GET /v1/files/{id}` 补齐，或与产品确认留空是否可接受。
- `entity_id`（issue 九个缺失字段之一）**不由数据面提供**：控制面 `applyTaskView` 已从 `api_key` 解析（`redis_view.go:162-164`）。该字段在复现中为空须先确认对应 api_key 在库里是否真有 `entity_id`（数据问题），不应作为本次代码修复项。

## 五、回归测试

单测（`testing` + `testify`，随代码放置）：

1. `bfe_modules/mod_ai_batch/mod_ai_batch_test.go`
   - 改写 `TestUsageScanBody` 夹具为 `response.body.usage`；新增"扁平旧格式仍解析"用例；
   - 新增 `response.body` 与 `response.usage` 同时存在的优先级用例，以及错误行（`error != null`）跳过用例；
   - **可测性改造**：把 `finishCreate`/`finishGet` 的字段组装抽成纯函数（`buildCreateTaskFields`/`buildGetTaskFields` 返回 `map[string]string`），单测直接断言 map——现模块测试无 redis 替身，抽纯函数比引入 redis 桩更轻；
   - 新增 `multipartPartStats` 用例：真实 multipart 体（含 `purpose` + `file` 两段）→ 断言 `filename`/`lines`/`bytes` 等于文件内容而非信封；无 `file` 段 → `ok=false`（限额退回近似计数）；
   - 新增 settle 契约发布用例：`usageParsed=true` 时 `aiMeta.BatchSettle=Settle`、`BatchSettleId` 正确、`SETTLE_USAGE_PARSE` +1，且**不**直接写 hash（写库已移到 finish）。
2. `bfe_modules/mod_ai_token_auth/batch_quota_test.go`（若不存在则新建）
   - `batchSettle` 抢占成功 → `BatchSettleApplied=true`、`BatchSettledUnits=costUnits`；
   - 抢占失败（`BATCH_SETTLED` 已存在）→ `BatchSettleApplied=false`（finish 不得改写任务记录）。

集成：

3. `bfe/tests/integration` 批量场景：上传 1 行/206 字节 jsonl → 断言 `BATCH_TASK.est_lines=1`、`BATCH_FILE.lines/bytes` 等于文件内容；下载输出文件 → 断言 `SETTLE_USAGE_PARSE=1`、`BATCH_SETTLED` 存在、`BATCH_TASK.settle_status=settled` 且 `usage_in/usage_out/settle_units` 非零。
4. `integration-test` 仓库 SC1401-TC026（本 issue 的失败用例）修复后应转绿；同链路 SC1302-TC015（批量限额）验证行数口径修正后限额判定正确。

验证命令：

```bash
cd bfe
go test ./bfe_modules/mod_ai_batch/ ./bfe_modules/mod_ai_token_auth/ ./bfe_basic/
make test   # 全仓单测 + vet（集成场景按需单跑）
```

## 六、修复后目标行为

一次 `completed` 批量 + 一次输出文件下载后，Redis 应满足：

```
BATCH_TASK:<batch_id>
  api_key_id, cluster, key_name, input_file_id, output_file_id,
  endpoint=/v1/chat/completions, request_counts={"completed":1,"failed":0,"total":1},
  est_lines=1, reserved_units=<行数×预估×batch价>, reserved=1, status=completed,
  created_at=<provider unix>, usage_in=10, usage_out=5, usage_source=download,
  settle_units=<batch 价实结>, settle_status=settled

BATCH_FILE:<cluster>:<input_file_id>   lines=1 bytes=206 purpose=batch dir=input  filename=<原文件名>
BATCH_FILE:<cluster>:<output_file_id>  dir=output batch_id=<batch_id>
BATCH_SETTLED:<batch_id> = 1
```

控制面 `GET /open-api/v1/batches/{id}` 随之补齐九个字段，`batch_files` 含输入与输出两条；对账 `usage_source` 由触发路径决定（下载拦截 → `download`，job 兜底 → `reconcile`）。

监控侧：`SETTLE_USAGE_PARSE` 与 `SETTLE_USAGE_PARSE_ERR` 恢复为可区分信号；新增 `UPLOAD_COUNT_APPROX`（流式上传退回近似计数）用于识别口径降级。

## 七、边界与残余风险

- **存量键无新字段**：`BATCH_*` TTL 48h，滚动升级期间旧键缺 `output_file_id` 等字段；控制面已具备"hash 未携带不覆盖 DB"语义（步骤 §四-1 必须保持），不做数据回填。
- **下载不经 BFE**（客户端直连 provider）：不触发拦截结算，仍依赖控制面对账 job——因此 §四-4 的 usage 解析修复是**必须**，否则兜底路径静默结算为 0。
- **客户端中止下载**：`usageScanBody` 未 finalize（既有设计），结算走 job 兜底，行为不变。
- **`ctx.batchId` 缺失**（输出文件绑定被 TTL 清理或扩容前历史键）：无法回写任务记录，仅 file 级结算；由对账 job 补齐。
- **`lines` 精度**：仅在 multipart 可整体缓冲且 `file` part 可定位时精确；chunked/超大文件退回近似并计数 `UPLOAD_COUNT_APPROX`——预扣额仍可能偏大，需在控制面对账巡检（设计 §5 职责④）里以 `lines_src=approx` 兜底核对，彻底解决需在流式路径上做 multipart 状态机增量解析，属独立改造。
- **`reserved_units` 与 `BATCH_RESERVE_BATCH` 双源**：前者是任务视图镜像，后者才是释放/结算的簿记源（`batch_quota.go:184-217`）；两者不一致由对账巡检比对告警，本变更不合并二者。
- **不改对外语义**：无新增配置项、无模块顺序调整、无计费价格与预留公式变化；`BATCH_SETTLED` 去重、fail-open 策略、错误码与 HTTP 透传行为均保持原样。

## 八、实施记录（2026-10-19）

按 `bfe-rd-workflow` Phase 1–7 落地，代码与测试与 §三/§五 方案一致。

### 8.1 改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `AiBasicInfo` 新增 `BatchSettleApplied` / `BatchSettledUnits`（结算执行结果回传 mod_ai_batch） |
| `bfe_modules/mod_ai_batch/resp_capture.go` | `batchResultRow` + `usageAndModel()`：`response.body.usage\|model` 优先、扁平 `response.usage\|model` 回退；`SETTLE_USAGE_PARSE_ERR`（非法行）/`SETTLE_NO_USAGE`（有 response 无 usage）接线；`finalize` 不再写 `BATCH_TASK`（只发布契约 + `SETTLE_USAGE_PARSE`）；`batchResponse` 扩 `endpoint`/`request_counts`/`created_at`，`fileResponse` 扩 `filename` |
| `bfe_modules/mod_ai_batch/mod_ai_batch_handlers.go` | `buildUploadFileFields`/`buildCreateTaskFields`/`buildGetTaskFields`/`buildSettleTaskFields` 四个纯函数；`finishUpload` 取内容行数/字节 + `filename` 并标 `lines_src`/`bytes_src`；`finishCreate`/`finishGet` 写终态字段；`requestFinishHandler` 在 `BatchSettleApplied` 时写 `usage_in/usage_out/settle_units/settle_status/usage_source`；`preCheckUpload` 用 file part 做限额检查；`preCheckCreate` 采集 `endpoint` |
| `bfe_modules/mod_ai_batch/file_counter.go` | 新增 `multipartFilePartStats`/`countPartContent`（只统计 `file` part 内容）；`countingBody` 注释收敛为"限额中断 + 兜底计数" |
| `bfe_modules/mod_ai_batch/batch_state.go` | 键结构契约注释更新为最终布局；`batchFileMeta`/`batchTaskMeta` 与读路径补 `cluster`/`filename`/`lines_src`/`bytes_src`/`output_file_id`/`endpoint`/`request_counts`/`reserved_units`/`created_at`；新增 `fileSrcExact`/`fileSrcApprox` |
| `bfe_modules/mod_ai_batch/batch_context.go` | 新增 `endpoint` 与 upload part 统计字段（`uploadFilename`/`uploadPartBytes`/`uploadPartLines`/`uploadPartOK`） |
| `bfe_modules/mod_ai_batch/mod_ai_batch.go` | 计数器新增 `SETTLE_NO_USAGE`/`TASK_SETTLE_WRITE`/`UPLOAD_COUNT_APPROX`（`SETTLE_USAGE_PARSE_ERR` 保留，语义收窄为"非法行"） |
| `bfe_modules/mod_ai_token_auth/batch_quota.go` | `batchSettle` 成功后置 `BatchSettledUnits=costUnits`、`BatchSettleApplied=true`；release 分支仅在 `BatchRelease` 真正执行时置 `BatchSettleApplied` |
| `bfe_modules/mod_ai_batch/mod_ai_batch_test.go` | 夹具改 `response.body.usage`；新增旧形态兼容、错误行/无 usage 行/非法行计数、优先级；`multipartFilePartStats`、`buildUploadFileFields`、`buildCreateTaskFields`、`buildGetTaskFields`、`buildSettleTaskFields`、`parse*Response` 用例 |
| `bfe_modules/mod_ai_token_auth/batch_quota_test.go`（新建） | release 契约未生效时不得标记 applied；无契约不介入；三原语在无 Redis 时 fail-open |
| `docs/zh_cn/sys_design/ai_batch.md`（新建）+ `SUMMARY.md` | 批量状态与结算设计：键结构、写入时序、解析口径、双源说明、边界 |
| `tests/integration/测试设计文档/scenario-SC28-批量任务支持/TC-14-终态字段与文件内容统计落库.md`、`TC-15-结果文件多形态解析与异常行信号.md` | 新增用例文档；`场景说明.md` 与 `测试场景总体说明.md` 登记（SC28 计划用例数 6→16，补齐此前缺失的 SC28 对应关系表） |
| `tests/integration/implementation/scenario-SC28-ai-batch-support/sc28_ai_batch_test.go` | 结果文件夹具改 `response.body.usage`；mock provider 返回 `filename`/`endpoint`/`created_at`/`request_counts` 并支持 `resultBody` 覆盖；新增 `TestTC14_*`/`TestTC15_*` 与 `batchTaskHash`/`batchFileHash`/`requireField`/`batchMetric` 辅助 |
| `tests/integration/common/redis_server.go` | 新增 `HGetAll`（miniredis 无直接 API，用 `HKeys`+`HGet` 组合） |
| `tests/integration/common/monitor.go` | 新增 `ModuleCounterJSON`/`WaitForModuleCounterJSON`（mod_ai_batch 走 `module_state2.StateData.FormatOutput`，**不支持 prometheus**）；`CamelToSnake` 改为标准 camel 边界算法，使已是大写下划线的计数器键（如 `SETTLE_NO_USAGE`）不再被拆成 `S_E_T_T_L_E...` |

### 8.2 实施中确认的偏差

1. **计数器拆两个**（§三-步骤 1 原写"都计 ERR"）：`SETTLE_USAGE_PARSE_ERR` 只表示"整行非法 JSON"，`SETTLE_NO_USAGE` 表示"有 response 但无 usage"。理由：上游未开 `include_usage` 时整个结果文件都会落进 ERR，会把真正的解析错误淹没。
2. **`UPLOAD_COUNT_APPROX` 计数点在 `finishUpload`**（原写 `preCheckUpload`）：`preCheckUpload` 在 body 未整体缓冲时看不到 `all=false` 的结论，放在绑定写入处才能与 `lines_src=approx` 严格同源。
3. **`BATCH_FILE` 增加 `cluster` 字段**（方案未列）：控制面 `ai-gateway-api` 的 `batch_files.provider` 直接取哈希读回的 provider 列；写入集群名可让回源路径在 `BATCH_FILE` 被单独读取时自证归属，且与 `BATCH_TASK.cluster` 对齐。
4. **集成监控读法变更**：`mod_ai_batch` 的 monitor 处理器不支持 `?format=prometheus`，测试改用 JSON 文档读计数器；同时修正了 `CamelToSnake` 对"已是大写下划线"键的错误拆分（原实现会把 `SETTLE_USAGE_PARSE` 变成 `S_E_T_T_L_E_...`，导致读不到计数而"静默判 0"——该缺陷会让断言假通过，故在 `waitBatchMetric` 里补了 `got < want` 的显式失败）。

### 8.3 验证结果

| 范围 | 结果 |
| --- | --- |
| `go build ./...` / `go vet`（mod_ai_batch、mod_ai_token_auth、bfe_basic、tests/integration/common、SC28） | 通过 |
| 单测 `bfe_modules/mod_ai_batch`（含新增 6 例）/ `mod_ai_token_auth`（新增 3 例）/ `bfe_basic` / `bfe_server` | 全部通过 |
| 集成 SC28 全量（TC-01…TC-13、TC-14、TC-15） | 通过 |
| 回归 SC02 / SC03 / SC05 / SC07 | 通过 |

### 8.4 未做与待办

- **§四 控制面配套改动未实施**（属 `ai-gateway-api` 仓库，方案见 `ai-gateway-api/design-docs/modifications/2026-10-19-issue-254-batch-task-state-and-settle-fix/change-summary.md`）：数据面已按契约写全字段，控制面仍缺 `ParseRedisTaskView` 映射、终态产物补漏、`purpose` 映射 + `filename` 列、`settle.go` 同款 usage 解析。两侧不同批上线时控制面字段仍为空。
- 输出文件 `filename`：数据面只在 upload 的 File 对象里拿到输入文件文件名；输出文件的权威来源仍是 provider `GET /v1/files/{id}`（控制面待定项）。
- Git 提交：**未执行**，按流程需人工确认提交内容后另授权。