# BFE Issue #1441 修复方案（第二轮）：网关自生成 AI 错误响应补充请求关联标识

- Issue: https://github.com/bfenetworks/bfe/issues/1441
- 上游文档：[2026-10-21-issue-1441-key-exhaustion-error-contract](./../2026-10-21-issue-1441-key-exhaustion-error-contract/design-changes.md)（第一轮修复，commit `f4feae5b`）
- 状态：**已实施（2026-10-23）**。裁定取第三节建议的选项 A（双写）；本节原始分析保留备查。

## 一、复验结果（2026-10-23，run `20261023-143025`，SC2101-TC067）

复验环境：数据面 `http://172.19.1.222:8421`，版本 `1.8.9-preview.1`（`start_at` 2026-10-23 12:41:04，期间未重启），源码基线 `source/bfe@5d581d0d`（含第一轮修复 `f4feae5b`），verified commit `b1070d49`。

**已通过（第一轮主诉求，A-01..A-06a 全 PASSED）**：

- 无可用 Key 不再返回空 body 500，改为合成 429 + JSON 错误 envelope；
- `error.code` / `error.type` / `error.message` 非空（`NO_AVAILABLE_KEY` / `rate_limit_error`）；
- `Retry-After` 与 `details.retry_after_seconds` 同时存在；
- `X-Bfe-Gw-Error` 存在，响应可识别为网关自生成；零转发（不落到上游）；
- 配置回退集群时返回 200，命中 `mock-success-chat-a`（本轮首次真实通过）。

**仍未通过（A-06b 单点）**：

- 响应缺少 `X-Request-ID`（为空），客户端无法把错误响应关联到访问日志与 Key 选择/penalty 轨迹；
- 同一失败指纹 `d26c99e6…` 在 12:43、13:48、14:30 三次独立复验重复出现，TC067 判 `FAILED_EVIDENCE`（非 `FAILED_PRODUCT`——`ai_error_codes.md` §3.2 未定义关联字段，子断言暂无冻结条款支撑）。

## 二、根因（源码复核，本地方）

1. `bfe_server/reverseproxy.go:1938-1947`：Key 耗尽分支用 `NewAiErrorWithDetails(CodeNoAvailableKey, …)` 构造合成 429，`:1944` 补 `Retry-After` 后返回。
2. `bfe_basic/request_ai_basic.go:799-808`：`AiError.CreateErrorResponse` 只写 `Server`(:803)、`Content-Type`(:804)、`X-Bfe-Gw-Error`(:805)，**无任何请求关联字段**。
3. **共性放大**：`CreateErrorResponse` 是网关自生成 AI 错误的统一出口，全部 9 个调用点同受此缺欠——鉴权失败（`mod_ai_token_auth/mod_ai_token_auth.go:660`、`model_check.go:114`）、本地限流（`mod_ai_rate_limit/mod_ai_rate_limit.go:280`、`batch_check.go:95`）、batch 错误（`mod_ai_batch/mod_ai_batch_handlers.go:295/305/315`）、findProduct 失败（`reverseproxy.go:1195`）、`BACKEND_UNAVAILABLE`(:1402)、协议不匹配(:1667)、Key 耗尽 429(:1943)。修复必须落在公共构造点，而非各调用点（与第一轮「网关标记加在构造点」的裁定同理）。
4. **排除部署遗漏**：基线全部非测试 Go 代码无任何 `X-Request-ID` 响应写入；仓库唯一关联标识是 `X-Bfe-Log-Id`（`bfe_basic/common.go:28`），由 `mod_logid.requestIdHandler`（`bfe_modules/mod_logid/mod_logid.go:91-107`）在 `HandleBeforeLocation` 阶段从请求头透传或生成，写入 `req.LogId` 与请求头，访问日志 `BfeLog.logid` 同值（`genLogId` 注释，`mod_logid.go:114-127`）——但**从不回写响应头**。即：关联键网关门内全程存在，只差「写回响应」一步。

## 三、裁定点（待产品 owner 确认，阻塞实现）

| 选项 | 字段名 | 生成规则 | 分析 |
| --- | --- | --- | --- |
| A（建议） | `X-Bfe-Log-Id` + 回显 `X-Request-ID`（双写） | 恒写 `req.LogId`；客户端带 `X-Request-ID` 则原样回显 | 访问日志关联键即 `logid`，1:1 直达访问日志与 penalty 轨迹；同时兼容 OpenAI 生态惯例与 TC 现状断言（`X-Request-ID` 非空）。头数 +1，均为加性 |
| B | `X-Bfe-Log-Id`（单头） | 恒写 `req.LogId` | 关联最直接、头最少；但 TC 断言名需同步改为 `X-Bfe-Log-Id` |
| C | `X-Request-ID`（单头） | 回显客户端传入值，缺省网关生成 UUID | 生态惯例；但回显语义下访问日志未记录该值，「关联到访问日志」诉求落空，需另加日志字段 |

取值回退链（三选项通用）：`req.LogId` → 请求头既有 `X-Bfe-Log-Id`（多级 BFE 可信链路透传）→ 现场生成。注意 `genLogId` 位于 `mod_logid` 且未导出，而依赖方向为 `mod_logid → bfe_basic`，故生成逻辑需下沉至 `bfe_basic`/`bfe_util`（沿用 63-bit 十进制格式，与访问日志 BIGINT 兼容，见 `mod_logid.go:114-127` 注释）。

裁定后：`ai_error_codes.md` §3.2 增补「响应关联字段」条款（字段名、生成规则、必填性），TC067 A-06b 子断言相应升格为规范断言。

## 四、修复方案（按选项 A/B 实现；选项 C 差异仅一处取值）

1. **`bfe_basic/request_ai_basic.go` `CreateErrorResponse`（唯一改动点）**：在 `:805` 附近写入关联头——`X-Bfe-Log-Id: <req.LogId 回退链>`；选项 A 下若 `request.HttpRequest.Header.Get("X-Request-ID")` 非空则原样回显。9 个调用点零改动即全部覆盖，天然满足验证方「不是 429 单点补丁」的要求。
2. **不在范围**：`rewriteResponseWithUnified`（`bfe_server/reverseproxy_ai_error.go:153-166`）是上游错误归一的镜像构造点，改写的是上游响应（客户端可经上游自身标识关联），且该函数无 `Request` 入参；若要求归一响应也带网关关联头，另开卡。客户端中断空体 500（`isAIClientAbort` 路径）保持空体不加头（响应已不可达）。
3. 行为不变项：`NO_AVAILABLE_KEY`/`rate_limit_error` 语义、`Retry-After`、`X-Bfe-Gw-Error`、归一器跳过逻辑、零转发、cluster fallback 均不受影响（纯加性响应头）。

## 五、改动文件（实现阶段）

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `CreateErrorResponse` 写关联头（回退链取值；`X-Bfe-Log-Id` 引用 `common.go` 既有常量，不新增字面量） |
| `bfe_basic/common.go` 或 `bfe_util` | 若保留「现场生成」回退：新增/下沉 63-bit 十进制 ID 生成函数（从 `mod_logid.genLogId` 逻辑移植，`mod_logid` 改为引用之） |
| `bfe_server/reverseproxy_ai_test.go` | 扩展 `TestAiClusterInvokeExhaustionReturnsSynthetic429`、`TestBackendUnavailableErrorResponseContract`：断言关联头存在、回显字节相等；新增鉴权失败路径（`mod_ai_token_auth`）构造响应的关联头断言 |
| `bfe_basic`（新测试文件） | `CreateErrorResponse` 单测：回退链三分支（LogId 非空 / 仅请求头 / 现场生成） |
| `tests/integration/测试设计文档/scenario-SC02-多API-Key轮换与重试/TC-09-首次选Key失败返回合成429并降级.md` 臂 B | 增关联头断言：`X-Bfe-Log-Id` 非空且与访问日志 `logid` 一致；请求携带 `X-Request-ID` 时回显字节相等（字段名按裁定） |
| `tests/integration/测试设计文档/scenario-SC27-上游错误体归一/TC-09-网关自生成错误跳过.md` | 增同一关联头断言（鉴权失败 `MODEL_NOT_ALLOWED` 代表例；同时锁定加头后归一跳过行为不变） |
| `tests/integration/测试设计文档/scenario-SC02-多API-Key轮换与重试/` 新增 TC（网关侧失败合成 502） | 补第一轮遗留的 502 端到端用例：后端不可达（连接失败）→ 合成 502 `BACKEND_UNAVAILABLE` + `X-Bfe-Gw-Error: 1` + 关联头断言（「上游不可用」代表例）；`场景说明.md` 与 `测试场景总体说明.md` 同步 |
| `docs/zh_cn/sys_design/ai_error_codes.md` | §3.2 增补响应关联字段条款（裁定后） |

**测试设计文档为何本轮必做**：验证方复验要求「同一关联头在其它自生成错误（至少鉴权失败、上游不可用各一例）同样出现，确认不是 429 单点补丁」，外部 TC（SC2101-TC067）的三条证据在仓库设计文档中需各有落点（与第一轮 #1441 扩展 SC02/TC-09 臂 B 断言的做法一致）。其中网关侧合成 502（`ServeHTTPForAI` 出口，`reverseproxy.go:1195/:1402`）在现有设计文档中**零覆盖**——文档中既有的 502 全部是上游返回体或归一映射（SC03/SC05/SC27），且第一轮已把 502 端到端用例列为遗留增强；本轮验证使其成为硬性要求，须新增 TC 收口。

## 六、验证方案

1. 单测 + `go build` / `go vet`（bfe_server、bfe_basic）通过后重跑 **TC067**：耗尽 429 关联头非空（回显语义下与请求头逐字节相等），envelope 字段与 `Retry-After` 不变；
2. 其它自生成错误各抽一例（鉴权失败——SC27/TC-09；上游不可用——SC02 新增 502 TC）确认关联头同现，对应 TC067 的「非 429 单点补丁」证据要求；
3. 不回归：`NO_AVAILABLE_KEY`/`rate_limit_error` 语义、零转发、回退臂 200；SC02 全量（含 TC-09 臂 B 与新增 502 TC）、SC27（归一跳过，TC-09 关联头扩展后）回归。

## 七、遗留事项

- 字段名与生成规则的产品裁定（第三节）为唯一阻塞项；裁定前 TC067 A-06b 维持 `FAILED_EVIDENCE`；
- 上游归一响应（`rewriteResponseWithUnified`）与非 AI `ServeHTTP` 本地错误的关联标识另开卡；
- 多级 BFE 链路下 `X-Bfe-Log-Id` 透传与访问日志 `logid` 的一致性已由 `mod_logid` 既有语义保证，无需本卡处理。

## 八、实施记录（2026-10-23，裁定取选项 A 双写后执行）

**裁定**：选项 A——`X-Bfe-Log-Id` 恒写（回退链取值），客户端携带 `X-Request-ID` 时原样回显；`ai_error_codes.md` 新增 §3.3 条款与 §5 交叉引用，TC067 A-06b 子断言的规范依据以此为准。

**实际改动**（相对第五节计划的差异已标注）：

| 文件 | 实际落地 |
| --- | --- |
| `bfe_basic/request_ai_basic.go` | `CreateErrorResponse` 调 `setCorrelationHeaders`（同一文件）：`X-Bfe-Log-Id` 按回退链取值（`req.LogId` → 请求头 → `GenLogId()`），后两种取值回写 `req.LogId` 与请求头（保证访问日志同源）；`X-Request-ID` 非空则原样回显；`HttpRequest` 为 nil 时安全跳过 |
| `bfe_basic/logid.go`（新；计划为 common.go 或 bfe_util） | 导出 `GenLogId()`，生成逻辑从 `mod_logid` 下沉（含 #1417 63-bit 十进制格式注释一并迁移）；`mod_logid` 两处调用改为引用，本地 `genLogId` 删除 |
| `bfe_basic/common.go` | 新增 `HeaderXRequestId = "X-Request-ID"` 常量 |
| `bfe_basic/request_ai_basic_test.go`（新） | `TestCreateErrorResponseCorrelationHeaders` 7 个子用例：回退链三分支、回显字节相等、未携带不生成、nil 防护、`X-Bfe-Gw-Error`/body 不变项 |
| `bfe_server/reverseproxy_ai_test.go` | `TestAiClusterInvokeExhaustionReturnsSynthetic429`、`TestBackendUnavailableErrorResponseContract` 各增关联头 + 回显断言 |
| `bfe_modules/mod_logid/mod_logid_test.go` | `TestGenLogId` 改为对 `bfe_basic.GenLogId()` 的一致性测试（语义不变） |
| `docs/zh_cn/sys_design/ai_error_codes.md` | §3.3「响应关联头」+ §5 交叉引用 |
| `tests/integration/测试设计文档/...` | SC02/TC-09 臂 B 扩展（+臂 A 反例边界）；新增 SC02/TC-10（502 代表例）；SC27/TC-09 扩展（鉴权代表例 + 响应-日志 logid 恒等）；SC02 场景说明、测试场景总体说明同步；SC05/TC-22 边界说明补交叉引用 |
| `tests/integration/implementation/...` | SC02：testdata 三件套（gslb/host_rule/ai_route）增 `cluster_dead_backend` 链路、`clusterSubName` 新 case、TC-09 扩展、新增 `TestTC10`；SC27：`sendRawHeaders` + TC-09 扩展（含 `ParseBfeLogsAfterStop` 恒等断言） |

**实施中发现**：

1. `mod_access_pb3` 在 `req.LogId` 为空时本就走本地重生回退（`randUint64NonZero`，`request_log.go:103-105`）——若 `CreateErrorResponse` 只写响应头不回写 `req.LogId`，响应头与访问日志 `logid` 会分叉；回写恰好使两侧同源，该恒等由 SC27/TC-09 集成断言固化。
2. SC02 共享 testdata 的 `gslb.data` 声明 `cluster_dead_backend` 后，**所有** SC02 用例的 env 都必须为其生成 cluster_table 条目（`newTestEnv` 统一加 `StaticBackends: 127.0.0.1:1`），否则 BFE 启动报 `no backend conf`——TC-09 首跑因此失败，已修复。

**验证结果（2026-10-23 本机实跑）**：

| 范围 | 结果 |
| --- | --- |
| `go vet`：bfe_basic、mod_logid、bfe_server、tests/integration/common、SC02、SC27 | 通过 |
| 单测：bfe_basic（含新增）、bfe_server 全量、mod_logid、mod_access_pb3、mod_ai_token_auth、mod_ai_rate_limit、mod_ai_batch、mod_ai_route | 全过 |
| SC02 全量（10 TC：TC-09 扩展两臂 + 新 TC-10） | 通过（42s） |
| SC27 全量（10 TC：TC-09 扩展） | 通过（34s） |
| SC05 全量回归 | 通过（28s） |

**遗留**：① 外部复验——按验证方要求重跑 SC2101-TC067（需其环境），预期 A-06b 转 PASSED；② 上游归一响应与非 AI `ServeHTTP` 的关联标识另开卡（范围不变）；③ 改动未提交 Git，按流程待用户确认后再 commit/push。
