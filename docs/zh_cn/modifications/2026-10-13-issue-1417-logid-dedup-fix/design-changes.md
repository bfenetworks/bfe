# Issue #1417 BFE 侧修改说明：pb `logid` 恒 0（`uk_dedup` 判别列失效）

- Issue: [bfenetworks/bfe#1417](https://github.com/bfenetworks/bfe/issues/1417)（open；数据管道侧缺陷，同轮 SC3201 计费审计发现）
- 缺陷：`bfe_ai_request_log` 的幂等唯一键 `uk_dedup (hostid, log_time, ai_apikey_id, ai_requested_model)` 中 `log_time` 为秒级，写入侧以 `ON DUPLICATE KEY UPDATE` **全列覆盖**写入，同秒同 Key 同模型并发请求互相覆盖，n 个请求只留 1 行（生产实测丢率恰为 (n−1)/n）。设计上用于区分请求的逃逸列 `logid`（pb `BfeLog.logid`）**恒为 0**，判别能力从未兑现。
- 代码库：`bfe/`，HEAD `ea704614`（2026-10-13，"issue #1420"）。**本文行号均按该 HEAD 逐条读码核实**，非引用 issue 原文行号。
- 本文件范围：**只负责 BFE 侧**（让 pb `logid` 成为真正非 0、逐请求唯一的标识）。DDL 加列在 `ai-gateway-api`（MySQL）与 `ai-gateway-observability`（Doris 同缺陷），写入侧与文档在 `log-reader`；四份仓库方案共同的上游是总方案 `document-ai-gateway/迭代系统设计/v0.8/bfe_ai_request_log秒级覆盖问题/bfe_ai_request_log秒级覆盖问题修复方案.md`（下称"总方案"）。
- 关联：#1409/#1410/#1414（同轮计费审计缺陷家族，根因各异，勿合并）、#1357（访问日志凭据脱敏，同为"日志字段契约与实现不符"类）。
- 决策状态（2026-10-13 评审）：取值策略（§四.1，解析优先、为 0/失败随机重生）与编码映射（§四.2，**B1：`genLogId()` 改产十进制串**）均已定。A（解析侧截断）/B2（十六进制定长 16）/C（pb 改 string）的否决理由见 §四.2 评审记录。

## 一、口径定义（Expected 的来源）

| 条款 | 内容 |
| --- | --- |
| `bfe-access-pb/bfe_access_pb/bfe_access.proto:42-45` | `logid` 字段注释："For request log, each http request is assigned **one unique log id** by go-bfe / if there is already `BFE_LOGID` in header, **just use it**"；`required uint64 logid = 3` |
| `log-reader/doc/modules/mod_log_mysql/output-columns.md:33` | `logid` = "BFE 请求唯一标识" |
| `log-reader/doc/modules/mod_log_mysql/mod_log_mysql.md:53` | "消费侧以 `log_time` + `logid` 为准" |
| `log-reader/doc/sys_design/mod_log_mysql.md:73-74` | 幂等键四元组"与 Doris UNIQUE KEY 完全一致"，设计承诺的覆盖粒度是**分钟**级 |

三条断言：

- **A-01**：pb `BfeLog.logid` 必须是非 0 的请求级唯一标识，能区分同一秒内同 Key 同模型的不同请求；
- **A-02**（本方案以取值策略满足，不依赖 `mod_logid` 行为变更）：无论 `mod_logid` 是否开启、上游 header 形态如何、`genLogId` 格式新旧，**写出的 `bfeLog.Logid` 必须非 0**；
- **A-03**：logid 取不到/解析失败时**不得静默落 0**——必须有可观测信号。

诚实边界：产品条款规定了"唯一 log id"的**意图**，未规定字符串与 `uint64` 之间的编码映射，也未规定解析失败时的兜底形态。本文 §四 提出的 B1 映射、随机重生与计数器属**实现选型**，不构成对产品的额外要求。本文亦**不主张**修改访问日志 schema（`bfe-access-pb`）——总方案 §4.2 方案 C 已否决。

## 二、现状核实：issue 断言 vs HEAD 事实

| issue 断言 | HEAD 事实 | 判定 |
| --- | --- | --- |
| "`request_log.go:51` 以十进制解析 hex logid，错误被吞掉恒落 0" | `request_log.go:51` `logId, _ := strconv.ParseUint(req.LogId, 10, 64)`；`bfe_modules/mod_logid/mod_logid.go:113-119` `genLogId()` 返回 `hex.EncodeToString(16 random bytes)` = 32 位 hex | **成立** |
| "mod_logid 若未启用则 `req.LogId` 为空、同样落 0" | `conf/bfe.conf:72` `Modules = mod_logid`（出厂默认已启用）；`bfe_modules/bfe_modules.go:69` 注册，`modulesAll` 顺序保证其早于 `mod_access`(`:146`)/`mod_access_pb3`(`:215`)。故**这不是当前生产形态**，但取值策略必须覆盖该通路 | **部分成立** |
| "解析错误必须显式处理（宁可不写也不写 0，当前 `_` 吞错是缺陷的一部分）" | 成立，且 HEAD 无任何计数器覆盖该失败路径 | **成立** |
| （issue 未提及）**信任上游已带 header 的请求 `req.LogId` 保持空串** | `mod_logid.go:90-95`：`TrustSource()` 且 header 非空 → **直接返回且不回填 `req.LogId`**；`mod_logid_test.go:149-163` 明确断言 `req.LogId == ""` | **新发现的第二条恒 0 通路（D1b）** |
| （issue 未提及）**"改 base-16 解析"对完整 32 字符串不可行** | `genLogId()` 产出 128 位，pb 字段为 `uint64`（`bfe_access.proto:45`）——`ParseUint(s, 16, 64)` 必溢出报错 | **issue 建议需补编码决策** |
| （issue 未提及）同一解析缺陷还影响 `session_id` | `request_log.go:139`（`RequestLog.session_id`）与 `session_log.go:46`（会话日志 `bfeLog.Logid`）同为十进制解析 `session.SessionId`（`mod_logid.go:85` = `genLogId()`，hex）；`session_id` 列同理恒 0 | **成立，同源第 3 处** |
| "LR03 集成测试把覆盖行为断言为预期设计，缺陷被测试钉死" | 该断言针对 log-reader；BFE 侧对应事实是**根本没有测试覆盖**：`mod_access_pb3` 的单测 fixture 用十进制串（`request_log_test.go:40` `SessionId="100"`、`:81` `LogId="12345"`）绕过缺陷，`request_log_test.go:108-127`/`session_log_test.go:55-71` **均未断言 `bfeLog.Logid`** | **成立（形态为"未测"而非"钉死"）** |

## 三、根因链（BFE 侧）

### 3.1 三处十进制解析十六进制串（D1a / D1c）

```go
// bfe_modules/mod_access_pb3/request_log.go:50-52
// log id
logId, _ := strconv.ParseUint(req.LogId, 10, 64)   // req.LogId 是 32 位 hex → 恒失败
bfeLog.Logid = proto.Uint64(logId)                  // → 恒 0；err 被 _ 丢弃
```

```go
// bfe_modules/mod_access_pb3/request_log.go:138-140
sessionId, _ := strconv.ParseUint(req.Session.SessionId, 10, 64)  // 同为 hex → 恒 0
reqLog.SessionId = proto.Uint64(sessionId)
```

```go
// bfe_modules/mod_access_pb3/session_log.go:46-47
sessionId, _ := strconv.ParseUint(session.SessionId, 10, 64)      // 同为 hex → 恒 0
bfeLog.Logid = proto.Uint64(sessionId)
```

数据来源均为 `mod_logid`：`session.SessionId = genLogId()`（`mod_logid.go:85`）、`req.LogId = genLogId()`（`:103`）。

### 3.2 信任上游已带 logid 时拒绝回填（D1b）

```go
// bfe_modules/mod_logid/mod_logid.go:90-106
func (m *ModuleLogId) requestIdHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
	// check if request comes from trusted ip
	if req.Session.TrustSource() {
		logId := req.HttpRequest.Header.Get(bfe_basic.HeaderBfeLogId)
		if logId != "" {
			return bfe_module.BfeHandlerGoOn, nil   // ← 未回填 req.LogId
		} else {
			// trust ip, should has a logid
			m.state.NoLogidFromUpperBfe.Inc(1)
		}
	}
	// generate a new log id
	req.LogId = genLogId()
	req.HttpRequest.Header.Set(bfe_basic.HeaderBfeLogId, req.LogId)
	return bfe_module.BfeHandlerGoOn, nil
}
```

信任源（上游 BFE）已分配 logid 时，本 BFE 直接复用 header 而**不回填 `req.LogId`**，该请求的 `req.LogId` 保持 `""`。这是与 D1a 独立的第二条恒 0 通路。**注意**：取值策略（§四.1）已使该通路不再产出 0，`mod_logid` 是否回填 `req.LogId` 降级为可选增强（§四.3），不在本 issue 修改范围内。

### 3.3 三种失败通路叠加的取值矩阵（修复后）

| `req.LogId` 来源 | 现状 pb logid | B1 修复后 |
| --- | --- | --- |
| 本 BFE 生成（`genLogId()`，新十进制串） | 0 | 解析成功 → 非 0，**逐值恒等**（`logid_str = FormatUint(logid,10)`） |
| 信任上游 + header 已带（D1b） | 0 | 解析失败 → 随机重生（非 0、无关联、计数 +1） |
| mod_logid 未启用 | 0 | 随机重生（非 0、无关联、计数 +1） |
| 非法/异形 header | 0 | 随机重生（非 0、无关联、计数 +1） |
| 混编旧版 32-hex 头（滚动升级期） | 0 | 解析失败 → 随机重生（非 0、无关联、计数 +1，**属预期**） |

### 3.4 改动为何仍收敛在两个包

- pb `logid` 的整条读取/写入链只在这两处（`request_log.go`、`session_log.go`）发生数值化；`b2log` 只做整体 marshal，不引用 `Logid`（`bfe-access-pb/b2log/`）。
- `req.LogId` 是**不透明字符串**，除上述三处外无任何生产代码假设长度或字符集（`mod_access/request_log.go:218`、`mod_header/action_header_var.go:423`、`mod_traffic_mirror/mirror_task.go:132-133`、`mod_unified_waf/waf_client.go` 全按字符串透传）。唯一假设 32-hex 的是 `mod_logid` 自己的测试助手——B1 下随 `genLogId` 一并改写。
- `genLogId()` 全仓库仅两个调用点（`:85` session、`:103` request），均为字符串赋值——**`session_id` 列与 D1c 随 `genLogId` 十进制化一并修好**。

## 四、修改方案

### 4.1 取值策略（已定）：解析优先，为 0 / 失败则随机重生

`request_log.go:50-52` 改为：

```go
// log id
logId, ok := logIdToUint64(req.LogId)   // B1：ParseUint(s,10,64) + 值域校验（genLogId 已改产十进制，见 §4.2）
if !ok || logId == 0 {
	logId = randUint64NonZero()         // crypto/rand 生成 [1, 2^63-1]（值域约束见 §4.2）
	m.state.LogidRegenerated.Inc(1)     // 该行 logid 与 req.LogId 无关联，必须可见（A-03）
}
bfeLog.Logid = proto.Uint64(logId)
```

该策略的性质：

- **恒非 0、逐请求唯一，且不依赖 `mod_logid` 是否开启**（本决策的核心收益）。mod_logid 被裁剪、信任上游未回填（D1b）、上游 header 异形、滚动升级混编旧头等**所有**通路，都收敛为"唯一但可能与字符串 logid 无关联"，而不是"恒 0、无判别能力"。
- **解析成功时**（`req.LogId` 为 B1 十进制串），pb logid 与字符串 logid **逐值恒等**——`logid_str = FormatUint(logid, 10)`，可把日志里的值直接抄进 SQL（`WHERE logid = ...`）。
- **重放幂等不受影响**：随机值只在日志生成时产生一次，随后**固化在 pb 日志行中**；log-reader 重发 / `-b` 补读 / 重启重放读到的仍是同一值，`uk_dedup` 命中覆盖，不产生重复行。
- **随机重生必须计数**（`LogidRegenerated`），否则各 fallback 通路又退化为无信号。**此兜底不随编码映射改变而省略**。
- **同一策略应用于另外两处同源字段**：`request_log.go:139` 的 `session_id` 列与 `session_log.go:46` 的会话日志 logid（复用同一 `logIdToUint64`）。注意：`session_id` 随机兜底会使这些行无法按会话聚合——此前全 0 同样无法聚合，严格更优，可接受。

### 4.2 编码映射（已定，**B1**）：`genLogId()` 改产十进制串，逐值恒等

核心矛盾：`genLogId()` 产出 **128 位** hex，pb 字段是 **uint64**，放不进。**B1 从生成侧解决**：`genLogId()` 改产 `FormatUint(rand63(), 10)`（≤19 位十进制串），解析侧的现有 `ParseUint(s,10,64)` **直接正确**——原代码的十进制解析意图回归正确，无需任何截断。

**评审记录（2026-10-13，已定 B1）**：

| 方案 | 结论 | 理由 |
| --- | --- | --- |
| A. 解析侧取高 64 位（不动 `genLogId`） | 未选 | pb logid 是前缀派生值，关联靠规则且受值域掩码弱化（首字符 8-f 的行互推不一致）；保留为零对外格式变更时的备选 |
| **B1. `genLogId()` 改产十进制串** | **选定** | 逐值恒等、原代码意图回归正确、SQL 可直接抄值、改动面小 |
| B2. 十六进制定长 16 | 未选 | 解析须改基数 + 定长校验；纯数字串会被"解错不报错"（`"12345"`→`0x12345`）且无症状；与历史 32-hex 形似易混、外部定长 32 正则静默失配；SQL 不友好 |
| C. pb 字段改 `string` | 否决 | 需改 `bfe-access-pb` + 报表库列类型 + log-reader 字段类型，超出本卡范围，键长增加 |

`genLogId()` 新实现（`mod_logid.go:113-119`，7 行 → 约 10 行）：

```go
func genLogId() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	// issue #1417 值域约束：MySQL/Doris/StarRocks 的 BIGINT 均为有符号，
	// 超 2^63-1 会在 Doris 非严格模式被静默截断为边界值（高位行互撞）、
	// 在 MySQL 严格模式整批报错（Error 1264）。生成时即掩到 63 位。
	return strconv.FormatUint(binary.LittleEndian.Uint64(b)&0x7FFFFFFFFFFFFFFF, 10)
}
```

解析助手（新增于 `request_log.go`，供三处调用）：

```go
// logIdToUint64 将 BFE 请求 logid 字符串转换为 pb 的 uint64 logid（issue #1417，B1）。
// B1 下 genLogId 产出 ≤19 位十进制串，转换即完整 ParseUint、无截断：
//   logid_str == FormatUint(logid, 10)（逐值恒等）。
// 校验：纯十进制且 ≤ 2^63-1（值域约束，防御手工设置的大数 header）。
// 解析失败（空串 / 含非数字 / 超界 / 混编期旧版 32-hex 头）→ 调用方随机重生并计数，
// 不得像修改前那样静默吞掉写 0。
func logIdToUint64(logId string) (uint64, bool) {
	if logId == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(logId, 10, 64)
	if err != nil || v > math.MaxInt64 {
		return 0, false
	}
	return v, true
}
```

随机重生助手（新增于 `request_log.go`）：

```go
// randUint64NonZero 生成 [1, 2^63-1] 的加密随机数，作为 logid 解析失败时的兜底值。
// 值域上限 2^63-1 原因见 genLogId 的值域约束注释。
// crypto/rand 失败时循环重试：宁可阻塞也不写 0（A-03 的取值面）。
func randUint64NonZero() uint64 {
	for {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			continue
		}
		if v := binary.LittleEndian.Uint64(b[:]) & 0x7FFFFFFFFFFFFFFF; v != 0 {
			return v
		}
	}
}
```

> 实现注：`mod_logid.genLogId()` 是小写未导出函数，`mod_access_pb3` 无法复用，故在包内实现 `randUint64NonZero`（8 字节 crypto/rand + 掩码，无第三方依赖）。`math/rand` 不可用作兜底——全局随机源与可预测性都不适合"请求唯一标识"用途。

**值域约束（B1 在生成时即满足，已据 Doris/CH 实现核实）**：最终 uint64 必须落在 `[0, 2^63−1]`（`math.MaxInt64 = 9223372036854775807`）——MySQL/Doris/StarRocks 的 `BIGINT` 均为有符号，超出的后果：

| 后端 | 超出 `2^63−1` 的行为 | 后果 |
| --- | --- | --- |
| MySQL（默认 STRICT 模式） | 整批 INSERT 报 Error 1264 | log-reader 写批失败 → 退避重试 → `markDown`，数据积压 |
| Doris（非严格模式） | **静默截断为边界值** `9223372036854775807` | 所有高位行拿到**同一个** logid → 在新 uk 下互相覆盖，比丢行更隐蔽 |

B1 在 `genLogId()` 生成时掩到 63 位，解析侧再校验 `≤ math.MaxInt64`（防御手工 header），随机重生同样 `[1, 2^63−1]`，链路全段满足。ClickHouse 侧 `logid UInt64` 虽能存满 64 位，仍同域以保持跨后端一致。现成例证：`ai-gateway-observability/clickhouse/sqls/bfe_ai_request_log.sql` 头部注释记录"样例/线上取值（如 10602749765076101032）超出 Int64 上限，Doris 非严格模式截断为边界值存储"。

**B1 的对外影响与配套约定（已逐条核实）**：

- **格式变更**：`X-Bfe-Log-Id` header、`mod_access` 文本日志 `log_id`、biglog 平台关联、跨 BFE 串联的字符串形态从 32 位 hex 变为 ≤19 位十进制；文本日志历史数据的 32-hex 与新数据不可直接 join（pb logid 列历史上恒 0，无值可谈兼容）；
- **混编安全**：滚动升级期间旧版 BFE 产出的 32-hex 头被新版信任复用时，`ParseUint` 十进制解析失败 → §4.1 随机重生兜底（仍非 0、仅失去关联、计数可见），不会回退恒 0；
- **配套约定（交付项）**：纯十进制 19 位串在日志平台/前端链路容易被自动类型化为数字，JS `Number` 超 `2^53` 会精度丢失（`10602749765076101032` → `10602749765076100000`）；需在日志平台侧约定 logid 按字符串/keyword 处理（hex 形态免疫此问题，此为 B1 相对 B2 的唯一代价）。

### 4.3 `mod_logid` 信任上游回填（可选，不在本 issue 范围内）

取值策略落地后，D1b 不再产生 0。可选增强：信任源且 header 已带 logid 时回填 `req.LogId = logId`。**这是行为变更**（`mod_access` 文本日志 `log_id`、`%bfe_log_id`、mod_unified_waf、mod_traffic_mirror 的取值从空变为上游 logid）——评估为改善，但默认不做，如做需单独评审。本 issue 不修改 `mod_logid.go:90-106` 的控制流。

### 4.4 逐文件改动清单

| 文件 | 改动 | 类型 |
| --- | --- | --- |
| `bfe_modules/mod_logid/mod_logid.go` | `:113-119` `genLogId()` 改产十进制（8 字节 crypto/rand & `0x7FFF...` → `FormatUint`）；imports 增 `encoding/binary`、`strconv` | 代码 |
| `bfe_modules/mod_access_pb3/request_log.go` | 新增 `logIdToUint64()`、`randUint64NonZero()`；`:50-52` 改取值策略；`:138-140` 同构（session_id 失败计同一 `LogidRegenerated`，或独立 `SessionIdRegenerated`） | 代码 |
| `bfe_modules/mod_access_pb3/session_log.go` | `:46-47` 同构 | 代码 |
| `bfe_modules/mod_access_pb3/mod_access_pb3.go` | `ModuleAccessLogState` 增计数器（`LogidRegenerated`，可选 `SessionIdRegenerated`） | 代码 |
| `bfe_modules/mod_logid/mod_logid_test.go` | `isHexLogId` 改为十进制校验助手；`TestGenLogId`/`TestSessionIdHandler`/`TestRequestIdHandler` 的 32-hex 断言改为"纯十进制、≤ `MaxInt64`、非 0、互不相同" | 测试 |
| `bfe_modules/mod_access_pb3/request_log_test.go` | fixture `LogId`/`SessionId` 由 `"12345"`/`"100"` 改 B1 十进制串；新增 `bfeLog.Logid` 恒等断言；新增空串/非法串/32-hex 旧头 → 随机重生 + 计数断言 | 测试 |
| `bfe_modules/mod_access_pb3/session_log_test.go` | 同上（会话日志 logid 与 session_id） | 测试 |
| `bfe/docs/en_us/modules/mod_logid/mod_logid.md` | 删除失效行 `LOGID_CONVERT_FAILED`（该计数器不存在于代码，`mod_logid` 现只有 `NoLogidFromUpperBfe`） | 文档 |
| `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md` | 补 `logid`/`session_id` 字段的 B1 口径：十进制串 ↔ uint64 逐值恒等、值域 ≤ `2^63−1`、解析失败随机重生（若该文档已列这两列） | 文档 |

> 命名说明：不复用 `LOGID_CONVERT_FAILED` 到 `mod_access_pb3`——转换发生在 pb3，不属于 logid 生成职责；且该名在 en 文档里已被错误登记在 mod_logid 名下，一并清理，避免第三个"文档承诺但代码没有"的坑。

## 五、监控与告警

| 指标 | 位置 | 语义 | 建议 |
| --- | --- | --- | --- |
| `ACCESS_PB3.LogidRegenerated` | bfe monitor | logid 解析失败、随机重生的次数（请求日志 + 会话日志） | 稳态应趋 0；持续 >0 告警。**B1 滚动升级混编期旧 32-hex 头会抬高该值，属预期**，升级完成后应回落——按部署窗口设临时阈值 |
| `ACCESS_PB3.SessionIdRegenerated`（可选） | bfe monitor | `session_id` 列解析重生的次数 | 同上 |
| `LOGID.NoLogidFromUpperBfe` | 既有 | 信任源未带 logid（由本 BFE 生成） | 信息性，无需告警 |

**告警基线变更**：新增计数器属告警基线新增项，需同步既有看板/阈值清单。

## 六、测试计划

### 6.1 单元测试（`make test`）

| # | 用例 | 断言 |
| --- | --- | --- |
| U1 | `logIdToUint64` 表驱动 | `"1234567890123456789"`（19 位）→ ok 且恒等；`"9223372036854775807"`（MaxInt64）→ ok；`"9223372036854775808"` / `"18446744073709551615"` → 超界 fail；`""` / `"0f1e2d…"`（32-hex）/ `"12a45"` / 负数形态 → fail |
| U2 | `randUint64NonZero` 多次采样 | 恒 ≠ 0、恒 ≤ `MaxInt64`、互不相同 |
| U3 | `TestGenLogId`（改） | 产物为纯十进制串、`ParseUint` 成功、值 ∈ `(0, MaxInt64]`、连续采样互不相同 |
| U4 | `TestRequestLogGen`（改） | fixture `LogId` 改 B1 十进制串（如 `"123456789012345"`）；断言 `*bfeLog.Logid != 0`、`FormatUint(*bfeLog.Logid,10) == req.LogId`（恒等）；`LogidRegenerated` 不增 |
| U5 | `TestReqBasicInfoGen`（改） | fixture `SessionId` 改 B1 十进制串；断言 `*reqLog.SessionId` 恒等于解析值 |
| U6 | `TestSessionLogGen`（改） | 断言 `*bfeLog.Logid != 0` 且与 fixture 恒等 |
| U7 | 重生路径 | `LogId = ""`、`"zzz…"`、旧版 32-hex 头 → `Logid != 0`、`LogidRegenerated` +1；同测试内两个失败请求的 logid 不同 |
| U8 | 幂等性回归 | 同一 fixture 生成两次请求日志：解析路径 logid 相同（确定性），重生路径不同——验证重放读 pb 行而非重生成 |

### 6.2 集成 / 端到端（跨仓库）

| # | 用例 | 说明 |
| --- | --- | --- |
| E1 | pb3 解码抽查 | 生产/预发抽样 pb3 日志：解码后 `logid` 列非 0 比例 = 100%；`FormatUint(logid,10)` 与 `X-Bfe-Log-Id` header **逐值一致**（B1 恒等） |
| E2 | 同秒并发对账 | 按 issue 复现步骤：同秒以同 Key 同模型并发 n（2/3/4）个可区分请求，比对 pb3 行数与 `bfe_ai_request_log` 行数，**丢率应为 0**——依赖 `ai-gateway-api` DDL 加列（`logid` 进 `uk_dedup`）同时就位 |
| E3 | 重放幂等 | log-reader LR03 `TestLR03_IdempotentReplay` 保持通过 |
| E4 | 混编回归（灰度） | 滚动升级窗口内：新版 BFE 接收旧版上游 32-hex 头 → 该行 logid 非 0（随机重生）、`LogidRegenerated` 计数上升；升级完成后计数回落、恒等率恢复 100% |

**回归重点**：`mod_access_pb3` 其余断言（AI 字段、脱敏、时间列）不受影响；`session_log.go` 的 `SessionLog` 字段不受影响；`mod_logid` 控制流不变（仅 `genLogId` 实现与测试），`TestRequestIdHandler` 信任 + header 分支的既有断言（`req.LogId == ""`）保持原样。

## 七、兼容性与回滚

- **pb 兼容性**：`logid` 字段类型与 wire 格式不变（仍 `varint` uint64），仅取值由 0 变为真实值。**无需**改动 `bfe-access-pb`、Doris/MySQL 列类型、log-reader。
- **字符串格式变更（B1 的既定代价）**：`X-Bfe-Log-Id` 与文本日志 `log_id` 由 32 位 hex 变为 ≤19 位十进制；`req.LogId` 在所有消费方（文本日志、WAF、镜像、header 变量）均为不透明字符串透传，功能不受影响，但外部若按 32-hex 定长解析/关联会失配（见 §4.2 配套约定）。
- **配置**：无新增配置项、无新增 pb 字段，`conf/bfe.conf` 无需变更。
- **回滚**：回滚 bfe 二进制即可。回滚后新写日志的 `logid` 恢复为 0；若下游 uk 已加列，行为退化为现状（0 互撞覆盖），**不产生重复行、不损坏数据**——随机重生值只存在于 pb 日志行内，无其他持久化副作用。
- **上线顺序**：与 `ai-gateway-api` / `ai-gateway-observability` 的 DDL 改动**顺序不敏感**，单独上线都不劣于现状；推荐本修改先行（详见总方案 §6）。
- **性能**：每请求日志新增一次短串十进制解析 + 分支；失败路径一次 8 字节 crypto/rand。`genLogId` 由 16 字节随机降为 8 字节，成本不增。无锁、无额外分配热点。

## 八、风险与开放问题

| # | 项 | 说明 | 需要谁裁定 |
| --- | --- | --- | --- |
| 1 | **B1 的对外影响（已定，交付时落地）** | 全局 logid 字符串格式变更：混编期旧 32-hex 头走随机重生兜底（计数可见、属预期）；JS 数字链路精度风险以"日志平台侧按字符串/keyword 处理"约定缓解（§4.2 配套约定，属交付配套项） | 已裁定（B1）；配套约定需同步平台侧 |
| 2 | 63 位随机唯一性 | 63 位随机 id 全局碰撞概率在 10^9 请求量级约 10⁻²·10⁻⁹ 量级，且 §4.1 兜底后映射碰撞不再造成丢行（即便发生，退化为该秒该行覆盖，等同现状） | 接受 |
| 3 | `randUint64NonZero` 的 crypto/rand 失败循环 | crypto/rand 持续失败意味着 OS 随机源损坏，此时宁可循环也不写 0（A-03 的取值面）；代价是极端故障下日志路径阻塞——与原实现 `genLogId` 返回 `""` 的"降级继续"风格不同，属有意取舍 | 接受 |
| 4 | 随机重生行失去与字符串 logid 的关联 | 这类行无法与 `X-Bfe-Log-Id`/`mod_access` 文本日志对账；靠 `LogidRegenerated` 计数识别，占比应趋 0（`conf/bfe.conf:72` 默认启用 mod_logid；B1 混编期旧头亦会触发，升级完成后趋 0）。**判别能力（修复目标）不受影响** | 接受 |
| 5 | `session_id` 随机兜底打破按会话聚合 | 仅发生在解析失败场景（此前全 0 也无法聚合，严格更优）；若未来需要会话分析，应修 `mod_logid` 回填（§4.3）而非依赖随机兜底 | 接受 |
| 6 | 与 `mod_access` 文本日志的 logid 关系 | B1 下两者**逐值恒等**（同一十进制串）；该口径与值域约束必须写入 `ai_access_log_fields.md`（§4.4 清单），否则会成为下一个"文档与实现不符"的来源 | 本修改交付 |

## 九、验收口径（BFE 侧）

| # | 项 | 判据 |
| --- | --- | --- |
| 1 | pb `logid` 恒非 0 且逐请求唯一，不依赖 mod_logid | U4/U6/U7 通过；E1 生产抽查非 0 比例 100% |
| 2 | 解析成功行与字符串 logid **逐值恒等**（B1） | U4/U5 断言 `FormatUint(logid,10) == req.LogId`；E1 反格式化后与 `X-Bfe-Log-Id` header 一致；U3 断言 `genLogId()` 产物为 ≤19 位纯十进制且 ≤ `MaxInt64` |
| 3 | 解析失败时不静默（A-03） | U7 通过（含 32-hex 旧头用例）；monitor 出现 `LogidRegenerated` |
| 4 | 重放幂等 | U8 + E3 |
| 5 | 既有回归 | `make test`（`go test -cover ./...` + `go vet ./...`）全绿；`mod_access_pb3`、`mod_logid` 包覆盖不下降 |
| 6 | 不引入新契约 | `git diff` 不触及 `bfe-access-pb`、不新增配置项；`X-Bfe-Log-Id` 取值形态变更是 B1 的既定决策（§4.2），已写入对外影响 |

## 十、参考

- 缺陷：[bfenetworks/bfe#1417](https://github.com/bfenetworks/bfe/issues/1417)；总方案：`document-ai-gateway/迭代系统设计/v0.8/bfe_ai_request_log秒级覆盖问题/bfe_ai_request_log秒级覆盖问题修复方案.md`
- 同行修复先例（文档结构参照）：`docs/zh_cn/modifications/2026-10-13-issue-1414-billing-bypass-observability-gap/design-changes.md`
- 相关代码：`bfe_modules/mod_access_pb3/request_log.go`、`session_log.go`、`mod_access_pb3.go`、`bfe_modules/mod_logid/mod_logid.go`
- pb 定义：`bfe-access-pb/bfe_access_pb/bfe_access.proto:42-45,109`
- 下游消费契约：`log-reader/doc/modules/mod_log_mysql/output-columns.md:33,186`、`doc/sys_design/mod_log_mysql.md:73-74`
- 值域约束例证：`ai-gateway-observability/clickhouse/sqls/bfe_ai_request_log.sql` 头部注释

## 十一、实现校准记录（2026-10-13 落地）

实现与本文设计的偏差与补充（均已随代码落地）：

| # | 项 | 设计 | 实现 | 说明 |
|---|----|------|------|------|
| 1 | 计数器拆分 | §4.4 曾写"session_id 失败计同一 `LogidRegenerated`，或独立 `SessionIdRegenerated`" | **拆成两个**：`LogidRegenerated`（请求日志 logid）+ `SessionIdRegenerated`（`session_id` 列与会话日志 logid） | 两条写入路径的失败原因不同，分离后告警可定位（会话 id 由 `HandleAccept` 生成，请求 id 由 `HandleBeforeLocation` 生成） |
| 2 | 函数改为方法 | 未提及 | `sessionLogGen`、`reqBasicInfoGen` 由包级函数改为 `*ModuleAccessPb3` 方法（调用点与单测同步更新） | 两者是新计数器的写入点，原为包级函数拿不到 `m.state`；`requestLogGen` 本就是方法，改后风格统一 |
| 3 | 值域上限常量 | §4.2 写"解析取 `ParseUint(s[:16],16,64) & 0x7FFF...`"（方案 A 表述） | B1 落地：`genLogId()` 生成时掩 63 位（`& 0x7FFFFFFFFFFFFFFF`），`mod_access_pb3` 侧新增 `const maxLogid` 用于解析上限校验与随机重生上限 | B1 无需解析侧截断，掩码只在生成侧；`maxLogid` 同时被 `logIdToUint64`/`randUint64NonZero`/单测引用 |
| 4 | 测试文件数 | §4.4 列 4 源 + 3 测试 | 4 源 + **4** 测试（新增 `bfe_modules/mod_access_pb3/logid_test.go`：`logIdToUint64` 表驱动、`randUint64NonZero` 采样、解析恒等、重生路径与计数、两会话 id 路径计数分离） | 新用例集中成文件，避免改动既有测试文件的行尾结构 |
| 5 | 文档同步范围 | §4.4 列 en 文档清理 + zh sys_design 补充 | 另同步 zh/en `docs/*/modules/mod_logid/mod_logid.md`（logid 格式章节 + 监控项修正）、`CHANGELOG.md`（Changed ×2 / Fixed ×1） | 技能 Phase 3 要求的 sys_design 更新落在 `ai_access_log_fields.md` §5.7 |

**Phase 5/6 已落地（集成测试，2026-10-13）**——未新建 scenario，扩展现有 `scenario-SC05-AI访问日志字段校验`（已有 21 个 TC、含 `mod_access_pb3` testdata，是最贴近的落点）：

| # | 交付物 | 说明 |
|---|--------|------|
| 6 | TC 文档 `tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/TC-22-logid非0且逐请求唯一.md` | 按 skill 模板（目的/前置/请求构造/后端响应/执行步骤/预期/清理/边界） |
| 7 | 用例 `TestTC22_LogidNonZeroAndUnique`（`sc05_access_log_ai_fields_test.go`） | **两臂**：臂 1 默认 testdata（**未加载 `mod_logid`**，走本地重生兜底）；臂 2 追加 `Modules = mod_logid` 后重启（生产默认形态）。各连续 3 次同秒同 Key 同模型请求，断言 `logid` 非 0、三行互不相同、≤ `2^63-1` |
| 8 | 公共解码辅助 `ParseBfeLogs` / `ParseBfeLogsAfterStop`（`tests/integration/common/access_log_parser.go`） | 原 `ParseAccessLog*` 丢弃日志级字段（`logid` 在 `BfeLog` 顶层）故无法断言；`ParseAccessLog` 改为委托新函数，行为不变（已由 SC05/SC22/SC29 回归验证） |
| 9 | 文档同步 | `场景说明.md` 增 TC-22 行；`测试场景总体说明.md` 场景清单 TC 数 21→22 并补 TC-22 条目 |

**边界（写入 TC 文档）**：「`logid` 与 `X-Bfe-Log-Id` 字符串逐值恒等」仅由单元测试断言（`TestRequestLogGenLogidParsed` / `TestGenLogId`）——BFE 只在**请求 header** 设置该值，非信任源客户端读不到，集成层无法构造比对；报表库侧「同秒 n 请求 → n 行」由 log-reader LR03 承担。

**实测证据（TC-22 两臂日志）**：两臂各 3 行，`timestamp` 同秒、`ai_apikey_id`/`ai_requested_model` 相同（即报表库唯一键的碰撞前提），`logid` 三行各异且非 0——修复前这 3 行在唯一键下只会剩 1 行。
