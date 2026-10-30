# BFE Issue #1479 修复方案：缺价的"拒绝"选项、形态化日志与可观测性

- Issue: https://github.com/bfenetworks/bfe/issues/1479
- 缺陷：模型缺少某种接口（mode）的定价时，该接口的请求照常返回结果但不扣费，控制台无任何提示；修改建议为"控制台提示 + 提供未定价则拒绝的选项"。
- 现状（v1.9.0-dev，commit `2e23595b`）：issue #1408 已实现**语义回退**（非 chat mode 缺价回落 chat 价计费）与 `ai_price_miss` 标记，**不再静默免费**；本卡补齐 issue 的两条修改建议（拒绝选项、提示/信号），并把缺价信号从"单个布尔"细化为"形态 + 结构化日志 + 三态计数器"。
- 关联：#1408（回退 + `ai_price_miss`，已修）、#1382（原"缺价按 0"设计）、#1414（零成本信号链路）、bfe-access-pb `v0.3.17`（本卡新增 `ai_price_warn` 字段 766）。
- 跨仓配套：ai-gateway-api 增加集群配置字段 `price_miss_action`（snake_case，OpenAPI/存储）/ `PriceMissAction`（导出 `cluster_conf.data` 的 `AIConf`）；控制台新建集群显式写入 `fallback_chat`。bfe 侧不感知 API 命名。

## 一、现状盘点与缺口

### 1.1 现状（`calcCostUnits`，`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:1019-1050`）

| 缺价形态 | 计费行为 | pb3 日志 | 模块计数器 | 日志 |
| --- | --- | --- | --- | --- |
| 非 chat mode 缺价、有 chat 基线 | 按 chat 价计费 | `ai_price_miss=true`、`ai_cost_value>0` | `PriceFallbackUsed` +1 | WARN `model price fell back to chat price`（每请求一条） |
| 缺价且无 chat 基线 | 按 0 计费 | `ai_price_miss=true`、`ai_cost_value=0`、`ai_zero_cost_reason=price_miss` | `PriceLookupMiss` +1 | WARN `model price not found`（每请求一条） |
| ModelTable 整体缺失 | 按 0 计费 | `ai_zero_cost_reason=model_table_missing`，无 `ai_price_miss` | — | WARN `model table not found` |

`ai_price_miss`（`bfe-access-pb` field 763）为 optional bool，只表达"发生过缺价"，**不区分上表前两行**；`ai_mode`（716）可定位接口类型。

### 1.2 缺口

1. **无退出机制**：回退写死；responses 等接口与 chat 计价维度不同（cache/reasoning 子项），回退可能系统性多收或少收，无法阻止。
2. **无"未定价则拒绝"选项**（issue 建议之一）：现有缺价判定全在收尾计费，转发前无法拦截。
3. **pb 信号不可直读**：两种形态需靠 `ai_cost_value`/`ai_zero_cost_reason` 组合推断。
4. **WARN 每请求一条**：热门模型缺价会刷日志（与 #1414 abort 分支"故意不 warn"同一教训）。
5. **回退子项价格缺失不可见**：回退借 chat 条目计价，若该条目缺 `cache_read_input_token_cost` 等子项键，按 0 计且无信号（少收残余）。
6. **无消费方**：`ai_price_miss` 未被报表/控制台消费。
7. **计数器命名误导**：`PriceLookupMiss` 实际语义是"缺价且无回退基线、最终按 0"，且同时被 batch settle 使用（`batch_quota.go:260`）。

**不做的事（已确认）**：**不增加配置期覆盖度校验**（api 侧 lint / bfe reload 自检均不做）。原因：价格表由独立流程提供、导入顺序不可控，且 provider 无"支持哪些接口"声明，任何"应该有什么价"的判定都会误杀；缺价的可见性由本卡的日志/计数器/pb 字段承担。

## 二、修复方案

### 步骤 1（必改）：集群级 `PriceMissAction` 三态

文件：`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`。

```go
// AIConf 新增字段（PascalCase，与导出一致）
// PriceMissAction controls what happens when the (model, mode) price entry
// is absent: "fallback_chat" (default, bill at the model's chat price),
// "bill_zero" (bill 0, issue #1382 behavior) or "reject" (reject before
// forwarding, no upstream call).
PriceMissAction string

const (
    PriceMissActionFallbackChat = "fallback_chat"
    PriceMissActionBillZero     = "bill_zero"
    PriceMissActionReject       = "reject"
)
```

- `AIConfCheck`（`:1269`）增加枚举校验：空值 → 归一为 `fallback_chat`；非三值之一 → 加载失败（fail-fast）；
- **缺省值已确认保持 `fallback_chat`**（旧 conf 零改动，与当前行为一致；`bill_zero` 为显式"回到 v0.7.0.1 行为"开关）。

### 步骤 2（必改）：`calcCostUnits` 按动作分流

```go
entry := cluster_conf.LookupModelPrice(cluster.AIConf.ModelTable, targetModel, mode)
if entry == nil {
    switch cluster.AIConf.PriceMissAction {
    case cluster_conf.PriceMissActionBillZero:
        // 按 0 计费 + ai_price_miss（type=no_entry） + PriceMissZeroBilled
    case cluster_conf.PriceMissActionReject:
        // 正常流量到不了这里（请求阶段已拒绝）；兜底按 0 计费并计数
    default: // fallback_chat
        if mode != bfe_basic.ModeChat {
            if chatEntry := LookupModelPrice(..., ModeChat); chatEntry != nil {
                entry = chatEntry; priceMissed = true
            }
        }
        if entry == nil {
            // 无 chat 基线：按 0（type=no_baseline）+ PriceMissZeroBilled
        }
    }
}
```

- 新增计数 `PriceRejected`（请求阶段拒绝次数），与 `PriceMissZeroBilled`（收尾按 0 次数）区分；
- **动作覆盖边界（已确认）**：`PriceMissAction` 只作用于**第一层缺价**（`(model, mode)` 主条目缺失 / `ModelTable` 整体缺失）；**子项价格键缺失不与 reject 耦合**（见步骤 5）。

### 步骤 3（必改）：`reject` 的前置预检

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go`，`tokenFoundProductHandler`（`HandleFoundProduct`，`:709`）——鉴权相位、尚未转发，此时 `aiMeta.Mode`（`http_conn.go:570` 由路径检测）与 `TargetModel`（路由阶段写入）均就绪：

1. 跳过条件（与 `calcCostUnits` 排除一致）：`/count_tokens` 端点、`ModeFile`/`ModeBatch`、`TargetModel == ""`、`AiCacheHit`；
2. 取 cluster `AIConf`：`ctx.serverConf`（缺失时回落 `req.SvrDataConf`）→ `ClusterTableLookup(req.Route.ClusterName)`；
3. 动作非 `reject` → 直接放行（零开销）；是 `reject` → `LookupModelPrice(ModelTable, targetModel, mode)`；
4. 缺失（**含 `ModelTable` 整体缺失 / `AIConf == nil`**，已确认同等按"未定价"处理）→ 返回 AiError：

   ```go
   aiErr := bfe_basic.NewAiErrorWithDetails(
       bfe_basic.CodePriceNotConfigured, bfe_basic.TypeInvalidRequestError,
       "model price not configured for the requested interface.",
       &bfe_basic.AiErrorDetail{Model: aiMeta.ClientModel, ...})
   ```

   - 状态码 **503**（已确认：服务端配置缺失，非调用方错误；语义区别于上游 5xx 透传）；
   - `details` 带 `model`，message 中带 `mode`；返回 `BfeHandlerResponse`（自定长 503 body、连接可复用，issue #1416 口径）；`ClusterTableLookup` 本身失败（路由异常）**不拒绝**：fail-open + 限流 WARN；
5. 置 `aiMeta.MarkPriceMiss()` + `MarkPriceWarn("reject")`，计 `PriceRejected`；**不置** `ai_zero_cost_reason`（该字段是"绑定 RMB 计划且计费为 0 的 2xx"归因，reject 行非 2xx、未进入计费）；
6. **fallback 链交互**：按"当前尝试的 cluster"判定；fallback 到另一集群后按新集群的 `PriceMissAction`/`ModelTable` 重新判定。

### 步骤 4（必改）：pb 可见性（已确认方案 A）+ 新字段 `ai_price_warn`

文件：`bfe_basic/request_ai_basic.go`、`bfe_modules/mod_access_pb3/request_log.go`。

1. 新增 AiError 码 `CodePriceNotConfigured = "PRICE_NOT_CONFIGURED"`；`ErrorCodeToStatusCode`（`:650`）→ 503；`ErrorCodeToErrorType`（`:707`）→ `TypeInternalError`；
2. **方案 A**：`AiPriceMiss` 的写出目前在 `usage != nil && UsageDisposition() != UsageDispositionNone` 块内（`request_log.go:506-570`），reject 走 `UsageDispositionNone` 则写不出。把 `if aiInfo.IsPriceMiss() { reqLog.AiPriceMiss = ... }` **移出 usage 块**（与 `AiMode`/`AiProvider` 同级）——`ai_price_miss` 表达"缺价事实"，与 usage/计费无关；`ai_zero_cost_reason` 语义不变（仅 2xx 零计费检查点写）。对账影响：非 2xx 行自此也会带 `ai_price_miss`，筛选时需叠加 `err_code`；
3. **新字段 `ai_price_warn`（bfe-access-pb field 766，`v0.3.17` 已发布）**：`optional string`，取值与步骤 6 的 `PRICE_MISS` type 一一对齐（`no_entry`/`no_baseline`/`fallback`/`subitem_missing`/`no_model_table`/`reject`），同请求多原因写**最具体**者（`subitem_missing` > `fallback` > `no_entry|no_baseline|no_model_table`；`reject` 互斥）；
   - 承载：`AiBasicInfo` 新增内部字段 `priceWarn string` + `MarkPriceWarn(value)`（setter 内实现"最具体优先"合并）；
   - 写出：与 `AiPriceMiss` 同一请求级位置；`go.mod` 升级 `bfe-access-pb v0.3.16 → v0.3.17`。

### 步骤 5（必改）：回退子项价格缺失计数（已确认区分"键缺失"与"显式 0"）

回退分支（`priceMissed == true`）计价前，对**实际产生用量**的子项维度检查价格键是否存在：

- 用 `ModelPrice.HasPrice(tier, key)` 判定（`cluster_conf_load.go:1623`，内部即 `lookupPrice` 的 ok 位，先查 tier 再查默认表；与 #1420 "键存在=显式选择"约定同源）；**键存在（含显式 0）视为已定价、不计数**；键缺失 → `PriceFallbackSubItemMiss` +1；
- 仅检查有量的维度：`usage.CacheReadTokens > 0` 才查 `cache_read_input_token_cost`，`usage.CacheWriteTokens > 0` 才查 `cache_creation_input_token_cost` / `cache_creation_input_token_cost_1h`；未用到的维度不计数；
- 每请求最多计 1 次，限流 WARN 携带 `missing_keys=`；
- **与 reject 不耦合（已确认）**：缺键维度有量时**只计信号不拒单**，金额仍按 0 计（"少收残余"）。原因：子项用量只有响应后可知、流式响应头已发无法改判，转发前拒绝只能凭配置一刀切（等于把 `fallback_chat` 退化为 `reject`）；且少收仅涉 cache 读写单价、需三条件同时成立，比整单按 0 低一个量级；运维闭环为"`PriceFallbackSubItemMiss>0` → 补价 → 计数回落"。
- 可选增强（默认关，另行排期）：`PriceFallbackSubItemAction: allow|reject`，`reject` 仅对**请求侧可预判用缓存**的协议生效（Anthropic `cache_control`、OpenAI `prompt_cache_key`）；明确不做"凭配置全量拒绝"。

### 步骤 6（必改）：结构化 `PRICE_MISS` 日志 + 限流

全链路缺价日志统一格式（WARN，行首固定标签，便于 grep/日志系统抽取）：

```
PRICE_MISS type=<no_entry|no_baseline|fallback|subitem_missing|no_model_table|reject> cluster=<c> model=<m> mode=<mode> action=<fallback_chat|bill_zero|reject> missing_keys=<k1,k2>
```

| type | 触发点 | 语义 |
| --- | --- | --- |
| `no_entry` | `calcCostUnits` 条目缺失且动作 `bill_zero` | 按 0 计费 |
| `no_baseline` | 条目缺失且无 chat 基线（默认动作） | 按 0 计费 |
| `fallback` | 回退到 chat 条目计费 | 按 chat 价计费 |
| `subitem_missing` | 回退条目缺子项键且该维度有量 | 该子项按 0 计（残余） |
| `no_model_table` | cluster `ModelTable` 缺失 | 按 0 计费 |
| `reject` | 请求阶段拒绝（步骤 3） | 503，未转发 |

- 限流：每 `(cluster, model, mode, type)` 每 60s 最多一条，首条即时打印（进程内 map + 原子时间戳，无需 Redis）；
- 结构化字段值为空输出 `-`；`mode`/`action` 打印实际生效值；`missing_keys` 仅 `subitem_missing` 出现；
- **告警口径**：主信号是计数器（`PriceMissZeroBilled`/`PriceFallbackUsed`/`PriceRejected`/`PriceFallbackSubItemMiss`），日志用于定位；运维闭环"计数>0 → grep `PRICE_MISS` → 补价 → 计数回落"。

### 步骤 7（必改）：计数器改名 `PriceLookupMiss` → `PriceMissZeroBilled`

已确认该指标**未接入生产监控/看板/告警**，不设双写过渡期，随本卡一步改名（纯可读性，零功能影响）：

| 位置 | 改动 |
| --- | --- |
| `mod_ai_token_auth.go` 状态字段（`:63`） | `PriceLookupMiss` → `PriceMissZeroBilled`（反射初始化，字段名即指标名） |
| `calcCostUnits` 递增点（`:1039`） | 改名 |
| `batch_quota.go:260`（batch settle 的 batch mode 缺价，按模型组逐次递增） | 改名，语义写入 monitor 文档 |
| 单测（`TestCalcCostUnits_PriceLookupMissCounter` 及断言） | 用例与断言同步改名 |

发布说明单列指标名变更；历史 modification 文档（`2026-09-22-issue-1382-*`）不改写。

## 三、兼容性与发布顺序

- **旧 conf 零改动**：缺省空串 → `fallback_chat`，与 v1.9.0-dev 一致；`bill_zero` 需显式配置；
- **pb 兼容**：新增 optional 字段 `ai_price_warn`(766)——旧 log-reader/报表忽略未知字段；`ai_price_miss` 写出时机改变（非 2xx 也会写出，步骤 4 方案 A），按"仅 2xx 行有该字段"筛选的旧消费方需同步调整；
- **无 Redis/存储结构变化**；无模块顺序调整；
- **发布顺序**：`bfe-access-pb v0.3.17`（已发布）→ **bfe**（本卡）→ log-reader 映射 → ai-gateway-api 报表列/入库/查询 → ai-gateway-observability 三套 schema → web 展示（可后置）；每步独立可回滚；
- **回滚**：revert 可按步骤 1/3（reject）、4（pb）、5/6（信号）、7（改名）分块。

## 四、回归测试

### 单元测试

1. `cluster_conf`：`AIConfCheck` 三值合法/非法/空值归一；旧 conf 保持默认行为；
2. `calcCostUnits` 矩阵：三动作 × {非 chat 有基线、非 chat 无基线、chat 缺价} 的金额/pb 标记/计数；子项缺失计数（键缺失计数、显式 0 不计数、无用量维度不计数）；
3. `tokenFoundProductHandler`：`reject` 命中（缺价 → 503、上游零调用、`PriceRejected`）、有价放行、跳过条件、fallback 集群切换后按新集群重判、`ClusterTableLookup` 失败 fail-open；
4. `PRICE_MISS` 日志：五种 type + reject 的字段可 parse、`missing_keys` 仅子项场景出现、限流窗口内 1 条；
5. pb 写出：`UsageDispositionNone + IsPriceMiss` ⇒ `ai_price_miss` 仍写出；`ai_price_warn` 六种取值 + 优先级合并 + reject 行（非 2xx）也写出；
6. 改名：`PriceMissZeroBilled` 在按 0 分支递增、`PriceRejected` 在拒绝分支递增、互不串台。

### 集成测试

7. 新场景 SC42（`scenario-SC42-price-miss-action`，已落地 4 个 TC）：缺价 ModelTable + mock 后端
   - `reject` 臂：转发前拒绝（backend hits=0）、`error.code=PRICE_NOT_CONFIGURED`（503）、pb `err_code` 正确且 `ai_price_miss=true` + `ai_price_warn=reject`、余额不变；`ModelTable` 整体缺失子臂同断言；`ClusterTableLookup` 失败对照臂放行；
   - `fallback_chat` 臂：responses 请求按 chat 价计费、`ai_price_miss=true`、`ai_price_warn=fallback`、`ai_cost_value>0`；
   - `bill_zero` 臂：按 0 计费、`ai_price_warn=no_entry`、`ai_zero_cost_reason=price_miss`；
   - 限流臂：同模型连发 N 条，wf.log 仅 1 条 `PRICE_MISS`；
8. 回归：SC03（RMB 扣减）、SC05（日志字段，补 `ai_price_warn` 断言）、SC25（长度分档/缓存 1h 价）、SC27（错误归一）、SC29（计费快照）——确认默认值不改变既有行为。

验证命令：`cd bfe && go test ./bfe_modules/mod_ai_token_auth/ ./bfe_modules/mod_access_pb3/ ./bfe_config/bfe_cluster_conf/cluster_conf/` 与 `go test ./tests/integration/implementation/scenario-SC03-rmb-quota/ ./tests/integration/implementation/scenario-SC05-access-log-ai-fields/`；提交前 `make test`。

## 五、边界与残余风险

- **`reject` 是"未转发即拒绝"**：被拒请求不调用上游、不计费、不进 fallback（fallback 到另一集群时按新集群重新判定是否可放行）；`/count_tokens`、file/batch、缓存命中不参与判定；
- **残余少收（已知并接受）**：`fallback_chat` 下"子项键缺失 + 该维度有量"按 0 计那部分单价；**无配置期校验兜底**（已确认不做），依赖 `PriceFallbackSubItemMiss` + `PRICE_MISS` 日志的运维闭环；该残余在 v1.9.0-dev 已存在，本卡让其可观测；
- **显式 0 的子项价**视为"已定价"，真实用量下不会告警（配置纪律问题）；
- **`ai_price_miss` 覆盖范围扩大**（非 2xx 行也写出）：对账/报表筛选需叠加 `err_code`；
- **指标名变更**（`PriceLookupMiss` → `PriceMissZeroBilled`）：发布说明单列，看板/告警按新名配置。

## 六、部署与配置说明

- 生效方式：bfe 升级即生效，无配置迁移；集群可选配置 `AIConf.PriceMissAction`（缺省 `fallback_chat`）；
- ai-gateway-api 侧：OpenAPI `llm_config.price_miss_action`（snake_case，枚举校验）→ 导出 `AIConf.PriceMissAction`；控制台新建集群显式写入 `fallback_chat`（api 层不对 `nil` 强制填充，存量集群保持 `nil`，对 bfe 行为一致）；
- 文档同步：本目录 design-changes.md + `docs/zh_cn/sys_design/rmb_quota.md`（价格口径、三态动作、对账查询模板、`PRICE_MISS` 格式与告警闭环）+ `docs/zh_cn/monitor/`（新计数器与指标改名）+ `cluster_conf.data` 中英文配置说明（`PriceMissAction` 字段）+ `bfe-access-pb` 的 `docs/protobuf.md`（766 已在 v0.3.17 记录）。

## 七、实施记录（2026-10-30）

已按本方案实施，无方案偏差。

| 文件 | 改动 |
| --- | --- |
| `go.mod` / `go.sum` | `bfe-access-pb` 升级 `v0.3.16 → v0.3.17`（新增 `AiPriceWarn` field 766） |
| `bfe_basic/request_ai_basic.go` | 新增 `PriceWarn*` 六常量 + `priceWarnRank` 优先级表 + `priceWarn` 字段 + `MarkPriceWarn`/`PriceWarn`（最具体优先合并）；新增 `CodePriceNotConfigured`，`ErrorCodeToStatusCode → 503`、`ErrorCodeToErrorType → TypeInternalError` |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIConf.PriceMissAction` + 三常量；`AIConfCheck` 枚举校验并把空串归一为 `fallback_chat` |
| `bfe_modules/mod_access_pb3/request_log.go` | `AiPriceMiss` 写出移出 usage 块（请求级，方案 A），同级新增 `AiPriceWarn` |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | 状态计数器：`PriceLookupMiss → PriceMissZeroBilled`、新增 `PriceRejected`/`PriceFallbackSubItemMiss`；`calcCostUnits` 按 `PriceMissAction` 分流（含 ModelTable 缺失标记 `no_model_table`）；`tokenFoundProductHandler` 新增 `rejectIfPriceMissing` 前置预检（503、跳过条件、ModelTable 缺失同拒、cluster 查询失败 fail-open、fallback 链按当前集群判定） |
| `bfe_modules/mod_ai_token_auth/price_miss.go`（新增） | `PRICE_MISS` 结构化日志 + 进程内 60s 限流器；`missingSubitemKeys`（`HasPrice` 判存在、仅实际用量维度）；`priceMissActionOf`（空值防御性默认） |
| `bfe_modules/mod_ai_token_auth/batch_quota.go` | batch settle 缺价：计数器改名 + `MarkPriceWarn(no_entry)` + 统一 `PRICE_MISS` 日志 |
| 单测 | 新增 `mod_ai_token_auth_price_miss_test.go`（8 组：AIConfCheck、bill_zero/reject 分流、子项缺失、`missingSubitemKeys` 键缺失 vs 显式 0、reject 预检 6 臂、限流器、`MarkPriceWarn` 优先级）；`mod_access_pb3/request_log_test.go` 新增 `TestReqAiInfoGenPriceMissVisibility`（非 2xx reject 行仍写出 `ai_price_miss`/`ai_price_warn`，usage 字段仍抑制）；旧用例改名适配 |
| 集成测试 | 新增 **SC42 `scenario-SC42-price-miss-action`（4 个 TC：fallback_chat / bill_zero / reject / subitem_missing）**，含静态 AIConf 四集群、monitor 计数器与 pb 断言；SC03（TC26 等）与 SC28（batch 缺价）按新指标名 `PriceMissZeroBilled` 更新断言；`测试设计文档/scenario-SC42-缺价动作与形态信号/` 与 `测试场景总体说明.md` 同步 |
| 文档 | `rmb_quota.md`（三态动作、四层信号、对账模板、子项残余口径、测试建议）、`modules/mod_ai_token_auth`（指标表 + 改名说明）、`cluster_conf.data` 中英文（`PriceMissAction` 字段/示例） |

验证结果（2026-10-30）：

| 范围 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 全量单元测试（`go list ./... \| grep -v tests/integration`） | 全部通过 |
| 模块单测（`mod_ai_token_auth` / `mod_access_pb3` / `bfe_basic` / `cluster_conf`） | 全部通过 |
| 集成回归 | SC03（RMB 扣减 + 零成本信号）、SC05（日志字段）、SC09（分档价）、SC15（长度档/1h cache）、SC27（错误归一）、SC28（batch）、SC29（计费快照）全部通过 |
| `gofmt` | 变更文件全部干净（`action_host.go` 等 HEAD 既有违规未触碰） |

实施中修正两处（已回写方案正文）：① `reject` 判定钩子从 `HandleFoundProduct` 改为 `HandleAfterAITargetModel`（前者 `Route.ClusterName` 未就绪，SC42 TC03 实测未生效）；② `rejectIfPriceMissing` 的 `serverConf` 取 `ctx.serverConf`、缺失时回落 `req.SvrDataConf`。

实施注意：`gofmt -w <目录>` 会顺带重排既有未格式化文件（`b2log.go`/`conf_mod_ai_token_auth.go` 等），已按"只格式化本次变更文件"原则还原，避免无关 churn。

验证补充：SC42 4/4 通过（fallback 计费 75000 单位、bill_zero 零扣费、reject 503 + 后端零调用 + 非 2xx 行带 `ai_price_miss`/`ai_price_warn=reject`、subitem 按 chat 价计费 + `PriceFallbackSubItemMiss=1`）；SC03/SC05/SC09/SC15/SC27/SC28/SC29 回归通过。