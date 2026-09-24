# BFE Issue #1391 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1391
- 缺陷：未调用后端即结束的请求（已确认：401 鉴权拒绝 `INVALID_API_KEY`），PB access 日志（`mod_access_pb3`）的 `proxy_delay_time` 被写为 `2217714954`，超过 MySQL `INT` 最大值 `2147483647`，导致 log-reader `mod_log_mysql` 批量 INSERT 报 `Error 1264`，同批正常记录随整批重试耗尽后被丢弃（观测窗口内 5 次丢批、至少 17 行）
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD `de0ab3b9`）
- 影响链路：`bfe`（PB 日志生产，病根）→ `log-reader/mod_log_mysql`（uint32 直写 INT 列，无钳制）→ `ai-gateway-api/db_ddl_report_mysql.sql:101`（`proxy_delay_time INT DEFAULT NULL`）

## 一、根因

### 1.1 缺陷点（唯一）

`bfe_modules/mod_access_pb3/request_log.go:359-360`（`reqTimeInfoGen`，与 issue 源码证据一致）：

```go
// proxy delay(in ms)
ms = req.Stat.BackendFirst.Sub(req.Stat.ReadReqEnd).Nanoseconds() / 1000000
reqLog.ProxyDelayTime = proto.Uint32(uint32(ms))
```

未校验 `BackendFirst` 是否有效，也未防御负值，直接 `uint32(ms)` 截断。

### 1.2 为什么未调用后端的请求会算出负值

`Stat.BackendFirst` 全库唯一赋值点在 `bfe_server/reverseproxy.go:404`（reverseproxy 首次真正发起后端调用时）：

```go
request.Stat.BackendStart = time.Now()
if i == 0 {
    // record start time of the first try
    request.Stat.BackendFirst = request.Stat.BackendStart
}
```

凡是在到达 reverseproxy 转发之前就被短路的请求——本 issue 的 401（`mod_ai_token_auth` 在 `HandleFoundProduct` 返回 `BfeHandlerResponse`，`ServeHTTPForAI` 于 `reverseproxy.go:1214-1216` 走 `response_got`，不进入转发循环）、无路由命中 404（`:1221-1229`）、`BfeHandlerRedirect`、`BfeHandlerClose`——`BackendFirst` 始终保持 `time.Time` 零值（0001-01-01）。

于是 `time.Time{}.Sub(ReadReqEnd)` 为巨大的负 Duration；且 Go 的 `Time.Sub` 对超出 Duration 表示范围的结果**饱和**到 `math.MinInt64`（约 -292 年），并非真实的 -2025 年差值。复算（以 issue 记录的时间戳 `1790226831` 为 `ReadReqEnd`）：

```
int64 ms = -9223372036854        // math.MinInt64 / 1e6 饱和值
uint32(ms) = 2217714954          // 与 issue 观测值逐位一致
```

`2147483647 < 2217714954 < 4294967295`，恰好越过 signed `INT` 上限 → MySQL strict mode 拒绝该值（`Error 1264`）→ 含此行的整批 INSERT 失败。因饱和值恒定，**所有**未调用后端的请求都会产生同一个溢出值 `2217714954`，而非随机值。

### 1.3 为什么同一条 401 记录的其他耗时字段都是 0

对照 `reqTimeInfoGen`（`request_log.go:312-361`）各字段的端点赋值情况：

| 字段 | 端点赋值位置 | 未调用后端时的状态 | 结果 |
| --- | --- | --- | --- |
| `AllTime` | `now` − `ReadReqStart`（`http_conn.go:196` 链路必赋值） | 两端均有效 | 正常（<1ms 记 0） |
| `ReadClientTime` | `ReadReqEnd` − `ReadReqStart`，请求解析时成对赋值 | 两端均有效 | 正常 |
| `ClusterServeTime` | `ClusterStart`/`ClusterEnd` 在 `reverseproxy.go:321-324` **成对**赋值（`defer`） | 两端均零值 | 0−0=0，不溢出 |
| `BackendServeTime` | `BackendStart`/`BackendEnd` 在 `reverseproxy.go:401/411` **成对**赋值 | 两端均零值 | 0−0=0，不溢出 |
| `ConnectBackendTime` | `req.OutRequest.State`，且已有 `ms >= 0` 防护（`:348-356`） | `OutRequest` 为 nil | 0 |
| **`ProxyDelayTime`** | `BackendFirst`（仅 reverseproxy 赋值）− `ReadReqEnd`（解析时必赋值） | **零值 − 有效值** | **溢出** |
| `SessionOffsetTime` | `ResponseEnd` − `Session.StartTime`（`session.go:71` 必赋值） | 见 1.4 | 本次记录为 0，存在次生隐患 |

只有 `ProxyDelayTime` 是「零值 − 已赋值」的组合，因此在 401 场景下唯一溢出。

### 1.4 同函数次生隐患（建议一并修复）

`SessionOffsetTime`（`request_log.go:342-343`）：`ResponseEnd` 仅在 `ResponseStart` 非零时才会被赋值（`http_conn.go:589-590`），而 `BfeHandlerClose` 路径（`reverseproxy.go:1199-1202` 等，直接关闭连接、无任何响应）不经过 `send_response`（`reverseproxy.go:1447` 才设置 `ResponseStart`）→ `ResponseEnd` 保持零值 → `zero.Sub(Session.StartTime)` 同样饱和溢出，产生与 `proxy_delay_time` 同形态的垃圾值。本次 401 记录因走了 `send_response` 两端均有效而记 0，属"未触发"而非"无风险"。

### 1.5 代码库内已有的正确范式（本修复与之对齐）

1. 文本日志模块 `mod_access` 对同一字段已有防护（`bfe_modules/mod_access/request_log.go:280-283`）：`if !req.Stat.BackendFirst.IsZero() { ... }`，无效时输出 `-`；
2. `bfe_server/http_conn.go:617-618` 监控路径同样有 `IsZero()` 防护，并带注释 *"In redirect and some other cases, BackendFirst may be not set"*；
3. 同函数 `ConnectBackendTime`（`request_log.go:348-356`）使用 `ms >= 0` 才赋值的写法。

即：`BackendFirst` 可能无效是代码库已知契约，PB 日志模块漏掉了防护，属实现缺陷而非设计歧义。

### 1.6 下游放大效应（log-reader，病根不在此）

- `log-reader/reader_modules/mod_fields/field_registry.go:678-686`：`proxy_delay_time` 经 `reqLog.GetProxyDelayTime()`（uint32）提取，`isZeroUint32` 只能区分 0/非 0，**无法区分"未设置"与"真实 0"**；
- `log-reader/reader_modules/mod_log_mysql/field_mapper.go:114`：直接映射到 `proxy_delay_time` 列，写入前无 `math.MaxInt32` 钳制；
- 列类型 `INT DEFAULT NULL`（`ai-gateway-api/db_ddl_report_mysql.sql:101`）。

一行溢出 → 整批 `Error 1264` → 按设计重试 4 次 → 整批丢弃（含同批正常行）。这是"可预防的字段溢出持续触发丢批兜底"的形态，符合 issue 中产品 Oracle 条款"不应由可预防的字段溢出持续触发"。

## 二、修复步骤

### 步骤 1（必改，病根）：`reqTimeInfoGen` 为 `ProxyDelayTime` 增加有效性防护

文件：`bfe_modules/mod_access_pb3/request_log.go`，`:358-360`

```go
// proxy delay(in ms)
// BackendFirst is only set when the request actually invoked a backend;
// in auth-reject / no-route / redirect / close cases it stays zero.
pd := uint32(0)
if !req.Stat.BackendFirst.IsZero() {
    if ms := req.Stat.BackendFirst.Sub(req.Stat.ReadReqEnd).Nanoseconds() / 1000000; ms >= 0 {
        pd = uint32(ms)
    }
}
reqLog.ProxyDelayTime = proto.Uint32(pd)
```

兜底值选 `0`（而非留空走 NULL）的理由：

1. PB 该字段为 uint32 标量，下游 log-reader `GetProxyDelayTime()` 对"未设置"与"显式 0"均返回 0（`isZeroUint32`），留空无法向下游传递"未知"语义，需改 PB 消费协议才有收益；
2. 正常进入后端的 200 请求该字段本就记 0（代理无额外延迟时），写 0 与现有数据形态一致；
3. issue 中"0 或 NULL 的具体约定待确认"——在协议能区分之前，`0` 是改动最小、落库安全的选择。若后续要区分"未调用后端"，正确做法是 log-reader 侧按 `err_code` / `res_status_code` / `ai_auth_reject_reason` 派生，而非该耗时字段。

### 步骤 2（建议同改）：消除同函数次生溢出点

同一函数内对「Duration → uint32」转换统一收口为一个 helper，全部走 `IsZero` + `>= 0` 防护：

```go
// durationMsUint32 returns end.Sub(start) in milliseconds, or 0 when either
// endpoint is unset (zero time) or the difference is negative.
func durationMsUint32(end, start time.Time) uint32 {
    if end.IsZero() || start.IsZero() {
        return 0
    }
    if ms := end.Sub(start).Nanoseconds() / 1000000; ms >= 0 {
        return uint32(ms)
    }
    return 0
}
```

`reqTimeInfoGen` 内 `ClusterServeTime`、`BackendServeTime`、`WriteClientTime`、`SessionOffsetTime`、`ProxyDelayTime` 全部改经该 helper。其中 `SessionOffsetTime` 的 `ResponseEnd` 零值场景（1.4）是真实可达路径，其余字段当前端点成对赋值为"双零"虽不溢出，但收口后可防御未来新增的单端点赋值路径。`AllTime`（以 `now` 为端点）与 `ConnectBackendTime`（已有 `ms >= 0` 防护）维持原状。

> 若实施时希望控制改动面，最小集为：`ProxyDelayTime`（步骤 1，本 issue 病根）+ `SessionOffsetTime`（真实隐患）；其余三字段仅做 helper 收口、不改变行为。

### 步骤 3（可选，log-reader 侧纵深防御，另仓）：写入前钳制

`log-reader/reader_modules/mod_log_mysql` 在构造批量 INSERT 前，对映射到 `INT` 列的 uint32 时长字段做 `> math.MaxInt32` 钳制（置 NULL 或改写 `math.MaxInt32` 并计数告警）。

定位为**纵深防御而非替代修复**：病根在数据生产者（BFE），钳制会掩盖上游缺陷、且存量/第三方 PB 生产者仍可能注入其他越界字段；但可避免"单行坏值拖垮整批"的批次级连带丢失。若实施，建议钳制时输出 WARN 日志便于发现上游问题。

### 不需要修改的部分

- `bfe_modules/mod_access`（文本日志）：`request_log.go:280-283` 已正确防护；
- `bfe_server/http_conn.go:617-627`（ProxyDelay 监控）：已有 `IsZero()` 防护；
- `mod_ai_token_auth` 等鉴权模块：401 响应路径本身无缺陷，是日志侧未防御；
- DDL：列保持 `INT` 即可，修复后合法值上限为真实代理延迟毫秒数，不可能接近 `2^31`。

### 存量数据说明

溢出批次已在重试耗尽后丢弃，MySQL 报告库中**不存在**该垃圾值行，无需数据清理；但若 Kafka 等下游留存了 PB 原文，其中的 `2217714954` 属历史脏数据，分析时需按"未调用后端"对待（或过滤）。

## 三、回归测试

均放在 `bfe_modules/mod_access_pb3/request_log_test.go`（沿用 `testing` + `testify`），扩展现有 `TestReqTimeInfoGen`（`:312`）：

1. `TestReqTimeInfoGen_ProxyDelayNoBackend`：`BackendFirst` 保持 `time.Time{}` 零值（模拟 401 鉴权拒绝，其余 Stat 字段正常赋值）→ 断言 `ProxyDelayTime != nil && *ProxyDelayTime == 0`（修复前该用例会得到 `2217714954`，用 issue 时间戳可精确复现）；
2. `TestReqTimeInfoGen_ProxyDelayNormal`：`BackendFirst = ReadReqEnd.Add(3ms)` → 断言值为 3，确认正常路径不回归；
3. `TestReqTimeInfoGen_SessionOffsetNoResponse`：`ResponseStart`/`ResponseEnd` 均零值（模拟 `BfeHandlerClose`）→ 断言 `SessionOffsetTime == 0`；
4. 若采用 helper 收口：`TestDurationMsUint32` 表驱动——双零、单零（两种方向）、负值、正常值、`math.MinInt64` 饱和方向共 6 个用例；
5. 既有 `TestReqTimeInfoGen` 及 mod_access_pb3 全部用例回归。

集成测试（`bfe/tests/integration` SC05 访问日志字段校验）：新增 `TestTC18_NoBackendTimeFieldsFallback`（无效 Key 401 `INVALID_API_KEY` 臂 + 配额耗尽 429 臂，断言 `proxy_delay_time`/`cluster_serve_time`/`backend_serve_time`/`connect_backend_time` 为 0 且全部耗时字段 ≤ `2147483647`，修复前该用例因 `proxy_delay_time=2217714954` 而红）；设计文档 `TC-18` 及 SC05 场景说明已同步。既有 TC-01~TC-17 全量回归。

端到端验证（按 issue 复现步骤）：AI gateway 模式下用脱敏占位 Key 请求兼容模式接口 → `bfe-pblog-tool` 查看 401 `INVALID_API_KEY` 记录 → `proxy_delay_time` 应为 0 → log-reader 不再出现 `Error 1264` 与丢批日志。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | 未进入后端的鉴权失败请求，`proxy_delay_time` 使用数据库可接受的兜底值 | 步骤 1：无效时写 0 |
| 2 | Request 记录成功落库，不因可预防的字段溢出触发持续重试和丢批 | 步骤 1（病根消除）；步骤 3（log-reader 兜底，可选） |
| 3 | 正常进入后端的请求延迟字段不回归 | 步骤 1 仅新增守卫，正常路径行为不变 + 第三节回归用例 2 |
| 4 | 重试/丢批机制本身 | 不动，其语义（异常兜底）正确 |

## 五、影响面与边界

- 修复后 401 记录的 `proxy_delay_time` 为 0，与同字段的正常零延迟记录不可区分；如需识别"未调用后端"，应使用 `err_code` / `res_status_code` / `ai_auth_reject_reason` 等字段，不引入新的字段语义（见步骤 1 理由 3）；
- 文本日志 `mod_access` 的 `proxy_delay_time` 输出 `-` 与 PB 日志写 0 的差异是既有协议差异（文本日志本来就输出 `-`），本方案不统一两者；
- 本修复不改变任何请求转发、鉴权、计费行为，仅修正日志字段生成。

## 六、实施记录（2026-09-24）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_modules/mod_access_pb3/request_log.go` | 新增 `durationMsUint32(end, start)` helper（`IsZero` + `ms >= 0` 双防护）；`ClusterServeTime`、`BackendServeTime`、`WriteClientTime`、`SessionOffsetTime`、`ProxyDelayTime` 五字段收口（步骤 1+2 全量收口，非最小集）；`AllTime`、`ReadClientTime`、`ConnectBackendTime` 按计划维持原状 |
| `bfe_modules/mod_access_pb3/request_log_test.go` | 新增 `TestReqTimeInfoGenProxyDelayNoBackend`（401 同构：`BackendFirst` 零值 → `ProxyDelayTime==0`，修复前该断言得 `2217714954`）、`TestReqTimeInfoGenProxyDelayNormal`（精确 3ms）、`TestReqTimeInfoGenSessionOffsetNoResponse`（`BfeHandlerClose` 同构）、`TestDurationMsUint32`（双零/单零×2/负值/正常/亚毫秒截断 6 表驱动臂） |
| `tests/integration/implementation/scenario-SC05-access-log-ai-fields/sc05_access_log_ai_fields_test.go` | 新增 `TestTC18_NoBackendTimeFieldsFallback`（401 无效 Key 臂 + 429 配额耗尽臂：四后端耗时字段为 0、全部 8 个耗时字段 ≤ `math.MaxInt32`、拒绝原因与空路由字段断言）；新增 `assertNoBackendTimeFields`；`sendRequestToPath` 重构为委托 `sendRequestWithKey`（行为不变） |
| 设计文档 | 新增 SC05 `TC-18-未调用后端请求的耗时字段兜底.md`；SC05 `场景说明.md` 验证点列表与 TC 表同步 TC-18 |

验证：

- `go test ./bfe_modules/mod_access_pb3/` 全部通过（含新增 4 例）；
- `go vet` 干净；`gofmt` 干净（`b2log.go`、`mod_access_pb3.go` 为历史遗留未格式化文件，与本次无关，未触碰）；
- SC05 集成测试 `TestTC18_NoBackendTimeFieldsFallback` PASS（真实 BFE 进程，双拒绝臂各产生 1 条日志，`proxy_delay_time = 0`，后端命中 0 次）；
- SC05 全量回归 TC-01~TC-18 全部通过（37.9s）。

实施中对计划的偏离：

1. 单测命名采用文件既有无下划线风格（`TestReqTimeInfoGenProxyDelayNoBackend`），与计划草案的 snake 写法不同，语义一致。
2. 步骤 2 采用全量收口（5 个字段全部经 helper），未采用"最小集"备选。
3. 步骤 3（log-reader 写库钳制）未实施：定位为可选纵深防御且属 `log-reader` 仓，如需实施应另行立项；本修复后 BFE 不再生产溢出值，存量溢出批次已在重试耗尽后丢弃，无 MySQL 存量脏数据。
