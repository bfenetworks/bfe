# BFE 变更说明：mod_access_pb3 请求/响应头白名单采集（动态配置 `mod_access_pb.data`）

- 一句话变更：mod_access_pb3 新增按白名单采集请求头/响应头并写入 proto 既有字段 `req_headers`（55）/`res_headers`（158）；白名单等动态配置放独立 JSON 数据文件 `mod_access_pb.data`（多段式通用载体），静态配置 `mod_access_pb3.conf` 仅登记 `DataConfPath`，数据文件经 web monitor **reload 接口热加载**（handler 名 `mod_access_pb3`，照搬 mod_ai_route 的 `RouteRulePath`/`ai_route.data` 模式）；Init 时文件缺失按全段缺省容忍（存量部署零配置升级行为不变）；**BFE 加载侧不限制白名单内容**（敏感头管控不在数据面做，由上层配置管理承担），保留 `maskSensitiveCredentials` 运行时兜底。
- 代码库：`bfe/`（本文件），分支 **v1.10.0-dev**。§二 行号为设计时工作区 `ea4a0580`（2026-11-15）逐条读码核实。
- 关联：[bfenetworks/bfe#1506](https://github.com/bfenetworks/bfe/issues/1506)（需求来源）；参照 `bfe_modules/mod_ai_route/`（动态数据文件加载五件套）、`bfe_modules/mod_access/`（`${Name}req_header` 模板先例）；红线 `bfenetworks/bfe#1357`（原始凭证永不落日志，字段 49 `authorization` 已恒空丢弃）。
- 状态：**已实现（2026-11-15），尚未提交/推送**。实施记录与验证结果见 §五/§六。
- 实现期决策修订：`DataConfPath` 由必填改为**可选**——必填将使发行包与约 30 个集成场景 testdata 中的存量 `mod_access_pb3.conf` 加载失败，违背"零配置升级"目标（§3.1、配置文档已同步）；
- 实现期行为发现：AI 模式下请求转发前 `Authorization` 被原地改写为上游 Provider Key，白名单采集到的是上游凭证（客户端原始 Key 仍由 `maskSensitiveCredentials` 掩码）——印证"数据面不限制白名单内容"的必要性（SC49 TC-08 固化该行为）。

## 一、背景

`bfe_access_pb.RequestLog` 的 `req_headers`（字段 55）/`res_headers`（字段 158）与 `message HttpHeader{key,value}` 自协议发布起即为"关注的请求/响应头列表"预留，但 `mod_access_pb3` 从未填充——Access Log 经 log-reader → Kafka → Doris 后两列恒空（Doris 表 `req_headers`/`res_headers` 列早已存在）。业务需要通过 HTTP Header（如 `business_type`、`x-request-id`、`x-trace-id`）做流量分析与问题排查，要求：

1. 可配置请求头/响应头白名单，按需采集写入既有 proto 字段；
2. 默认不采集（行为不变），大小写不敏感；
3. 避免全量采集的存储开销与敏感信息泄露风险。

## 二、现状核对

### 2.1 采集逻辑（`bfe_modules/mod_access_pb3/request_log.go`，行号基于 `ea4a0580`）

| 事实 | 位置 | 说明 |
|------|------|------|
| 请求日志组装入口 | `:92 (requestLogGen)` | 末尾调 `maskSensitiveCredentials` |
| 请求头独立字段采集 | `:121` 调；`:253 (reqReqHeaderInfoGen)` | 硬编码 `Referer`/`X-Forwarded-For`/`User-Agent` 等；**字段 55 `req_headers` 从未填充** |
| Authorization 显式丢弃（#1357） | `:296-298` | 字段 49 恒空；注释 "must never be written" |
| 响应头独立字段采集 | `:127` 调；`:349 (reqResponseInfoGen)` | `res == nil` 早退 `:351`；**字段 158 `res_headers` 从未填充** |
| 多值头惯例 | 各 `strings.Join(values, ",")` | 白名单采集沿用同一拼接惯例 |
| 凭证掩码兜底 | `credential_mask.go:41 (maskSensitiveCredentials)` | 对整消息递归反射扫描；注释明确未来新增字段自动覆盖——`req_headers`/`res_headers` **零改动纳入兜底** |

### 2.2 模块配置与热加载现状（`conf_mod_access_pb2.go` / `mod_access_pb3.go`）

| 事实 | 位置 | 说明 |
|------|------|------|
| 配置结构 `ConfModAccessPb2` | `conf_mod_access_pb2.go:29` | 仅 `[Log]` + `[BasicConf]` 两段 |
| 配置校验 `ConfModAccessPbCheck` | `:66` | 本期追加 `DataConfPath` 校验 |
| `Init` 加载配置、注册回调 | `mod_access_pb3.go:93` | `HandleRequestFinish`/`HandleFinish`；**无 `Reload`、无 WebHandleReload 注册**——本期按 mod_ai_route 模式补数据文件热加载 |
| 默认配置 | `conf/mod_access_pb3/mod_access_pb3.conf` | 本期追加 `DataConfPath` 一行 |

### 2.3 参照：mod_ai_route 动态数据文件五件套

| 环节 | 位置 |
|------|------|
| 静态配置登记路径（`RouteRulePath` + `ConfPathProc`） | `bfe_modules/mod_ai_route/conf_load.go:25-55` |
| 数据文件 JSON 加载 | `bfe_modules/mod_ai_route/data_load.go:23 (AiRouteDataLoad)` |
| Init 装载 + `WebHandleReload` 注册 | `mod_ai_route/mod_ai_route.go:63-79, 128, 143-148` |
| 锁外校验 + 锁内原子替换 | `mod_ai_route/route_table.go:45-63 (Update)`，读路径 `RLock` |
| 数据文件样例 | `conf/mod_ai_route/ai_route.data` |

**有意为之的两点差异**：①数据文件多段化（顶层按配置类别分 key，首版仅 `access_header` 段，未来动态配置以新段加入）；②Init 时文件缺失按全段缺省 + Warn（mod_ai_route 是启动失败）——保证存量部署不加文件直接升级行为不变。

### 2.4 下游契约（零改动，仅备查）

- proto：`bfe-access-pb/bfe_access.proto:184, 255, 740`（字段 55/158 与 `HttpHeader` 早已发布；本期不动 proto、不 regenerate、不升版本）；
- Doris 列：`ARRAY<STRUCT<key VARCHAR(128), value VARCHAR(2048)>>`（key 128 / value 2048，截断上限对齐此列宽）；
- log-reader JSON 契约已含 `req_headers`/`res_headers`（`ai-gateway-observability/api/depends_api/req_log.md:83, 104`）。

## 三、变更设计

### 3.1 静态配置（`conf_mod_access_pb2.go`）

`[BasicConf]` 段新增：

```go
type ConfModAccessPb2 struct {
    Log struct { ... }
    BasicConf struct {
        OpenDebug bool
        // DataConfPath is the path of mod_access_pb.data (dynamic, multi-section
        // config: header whitelist today, more sections later). Relative paths
        // are resolved against conf root (same rule as mod_ai_route.RouteRulePath).
        DataConfPath string
    }
}
```

`ConfModAccessPbCheck` 追加：`DataConfPath` **可选**——非空时经 `bfe_util.ConfPathProc` 解析（与 `ConfModAiRouteCheck` 同规则）；缺省不启用动态配置（等价全段缺省，不采集），且全部存量 `mod_access_pb3.conf`（发行包、各集成场景 testdata）无需修改即可继续加载（实现期决策：若设为必填，约 30 处存量 conf 需同步加行，违背"零配置升级"目标）。样例 `conf/mod_access_pb3/mod_access_pb3.conf`：

```ini
[BasicConf]
OpenDebug = false
DataConfPath = ../conf/mod_access_pb3/mod_access_pb.data
```

### 3.2 数据文件格式（`mod_access_pb.data`，多段式 JSON）

```json
{
    "Version": "20261115090000",
    "access_header": {
        "req_headers": ["business_type", "x-request-id"],
        "res_headers": ["x-trace-id"]
    }
}
```

- `Version`：版本标识（控制面导出时由数据签名生成；手工维护时由运维填写）；BFE 不校验内容，仅透传；
- `access_header`：本期唯一段；段缺失/null/空数组 = 空白名单，不采集；
- 未来段：新顶层 key，加载时**未知段忽略 + Warn**（前向兼容）。

### 3.3 加载、校验与热更新（新文件 `mod_access_pb_data.go`）

```go
type ModAccessPbConf struct {
    Version      string
    AccessHeader AccessHeaderConf // 未来段在此追加字段
}
type AccessHeaderConf struct {
    ReqHeaders []string // 已按 http.CanonicalHeaderKey 归一
    ResHeaders []string
}
type ModAccessPbConfHolder struct {
    lock sync.RWMutex
    conf *ModAccessPbConf // nil 等同全段缺省
}
// Update：锁内整份引用交换（校验在锁外完成）
// Snapshot：读路径取引用，无拷贝
```

`ModAccessPbDataLoad(path)`（Init 与 Reload 共用，锁外执行）：

1. `bfe_util.LoadJsonFile` 解析；文件不存在于 Init 路径 → 返回 nil-conf 由调用方按全段缺省处理（Warn）；
2. 逐已知段解析；未知顶层 key 收集后统一 Warn，不失败；
3. `access_header` 段校验（缺失则跳过）：
   - 名称非空、trim 后 ≤128 字节；RFC 7230 token 字符，非法报错；
   - **大小写归一**为 `CanonicalHeaderKey` 形式再入白名单（运行时直接 map 查找，零开销）；
   - **白名单内容不做敏感头限制**：`Authorization`/`Proxy-Authorization`/`Cookie` 等出现在数据文件时与其它头同等处理，**BFE 加载侧不拒绝**；敏感头管控不在数据面做（由上层配置管理承担），BFE 仅保留运行时 `maskSensitiveCredentials` 兜底；
   - 归一后去重，重复报错。

`Init`（`mod_access_pb3.go`）：

```go
if err := m.loadModAccessPbConf(nil); err != nil { // 文件缺失仅 Warn，不失败
    log.Logger.Warn(...)
}
reloadHandlers := map[string]interface{}{ m.name: m.loadModAccessPbConf } // handler 名 "mod_access_pb3"
web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, reloadHandlers)
```

Reload 语义（对齐 mod_ai_route `loadRouteRuleConf :63-79`，支持 `?path=` 覆盖）：

- 加载/校验失败 → 返回 error，**整份旧配置继续生效**（不允许半新半旧）；
- 成功 → 锁内整份原子替换；读路径要么见旧要么见新，无中间态。

### 3.4 采集逻辑（`request_log.go`）

`reqReqHeaderInfoGen` 末尾（`:296-298` 注释之后）新增：

```go
conf := m.dataHolder.Snapshot()
for _, name := range conf.AccessHeader.ReqHeaders {
    if values, found := req.HttpRequest.Header[name /* 已规范化 */]; found {
        reqLog.ReqHeaders = append(reqLog.ReqHeaders, &bfe_access_pb3.HttpHeader{
            Key:   proto.String(name),
            Value: proto.String(truncateHeaderValue(strings.Join(values, ","), 2048)),
        })
    }
}
```

- `req.HttpRequest.Header` 解析时键已规范化，白名单归一后可直接 map 查找；
- 与既有独立字段（如 `User-Agent`）允许重叠，两者都写，互不影响；
- 响应头同构挂 `reqResponseInfoGen`（数据源 `res.Header`，`res == nil` 由 `:351` 早退天然跳过）；
- value 截断 2048 字节对齐 Doris 列宽，截断不打日志；头存在但值为空照常写入 `{key, ""}`，头不存在不写条目。

### 3.5 安全设计（红线 #1357）

1. **运行时全消息兜底**：`maskSensitiveCredentials`（`credential_mask.go:41`）递归扫描覆盖新增字段，零改动（红线 #1357：凡字符串值含原始 API Key 一律替换/置空）；
2. **敏感头管控不在数据面做**：BFE 加载侧不限制白名单内容（`Authorization`/`Proxy-Authorization`/`Cookie` 均可配置、正常采集）；若需限制，由上层配置管理（OpenAPI 校验）承担；
3. 白名单默认空、按需开启。

## 四、兼容性

- 缺文件/`access_header` 缺失/空白名单 → proto 字段不序列化，日志体积零增长，**默认行为与旧版完全一致**；
- 字段 55/158 早已发布：旧 BFE 新链路、新 BFE 旧链路均无交叉影响；回滚 = 删数据文件或回滚二进制，Doris 无 DDL；
- `mod_access_pb3.conf` 本体（`[Log]`、`DataConfPath`）仅 Init 生效，热加载只针对数据文件（与 mod_ai_route 边界一致）。

`bfe-access-pb`（proto 与文档）与 `ai-gateway-observability` 本期零改动、零提交。

## 五、验证计划

- [x] 单元测试（`bfe_modules/mod_access_pb3/*_test.go`）：大小写不敏感（配置 `business-type` 命中 `Business-Type`）、多值头 `,` 拼接、value 2048 截断、白名单含任意头名（含 `Authorization`）均可正常加载采集（BFE 不限制白名单内容）、空白名单零采集零开销、未知段忽略 + Warn、Init 缺文件容忍、Reload 成功原子替换 / 失败保留旧值、`res == nil` 跳过；
- [x] 既有测试全绿：`go test ./bfe_modules/mod_access_pb3/... ./bfe_server/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_token_auth/...` 全过（全仓 `go build ./...` 通过）；
- [x] 联调：SC49 集成测试（`tests/integration/implementation/scenario-SC49-access-log-header-whitelist/`）8/8 通过——真实 BFE 进程 + mock 后端，覆盖白名单采集/大小写不敏感/默认零采集回归/热加载/失败保留旧配置/未知段与缺段/2048 截断/凭证掩码；Doris 侧按既有列契约零改动（log-reader → Doris 链路为存量设施，待端到端环境复验）；
- [x] 回归：不配数据文件升级，确认日志与升级前逐字节一致（体积不增）——SC49 TC-03 三组对照 + SC05/SC41/SC46/SC48 既有场景全过。

## 六、实施步骤（bfe 侧）

| 步骤 | 内容 | 涉及文件 |
|------|------|----------|
| 1 | `ConfModAccessPb2` 增加 `DataConfPath` + 校验 | `bfe_modules/mod_access_pb3/conf_mod_access_pb2.go` |
| 2 | 数据文件加载框架（多段解析/校验/原子替换） | `bfe_modules/mod_access_pb3/mod_access_pb_data.go`（新增） |
| 3 | Init 装载（缺失容忍）+ WebHandleReload 注册 | `bfe_modules/mod_access_pb3/mod_access_pb3.go` |
| 4 | 请求头白名单采集 | `bfe_modules/mod_access_pb3/request_log.go`（`reqReqHeaderInfoGen`） |
| 5 | 响应头白名单采集 + 截断 | `bfe_modules/mod_access_pb3/request_log.go`（`reqResponseInfoGen`） |
| 6 | 配置/数据文件样例 + 文档 | `conf/mod_access_pb3/mod_access_pb3.conf`、`conf/mod_access_pb3/mod_access_pb.data`（新增） |
| 7 | 单元测试（§五） | `bfe_modules/mod_access_pb3/*_test.go` |
