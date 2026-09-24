# BFE 姊妹卡（#1387 限流同型缺口）修复方案：限流「适用模型」按目标模型匹配

- 姊妹卡来源：[bfenetworks/bfe#1387](https://github.com/bfenetworks/bfe/issues/1387)「姊妹卡建议（限流同型缺口，不在本卡范围）」；独立 issue 号待创建后补充
- 缺陷：API-Key 限流策略（TPM / RPM / 最大并发）的「适用模型」按请求体原始模型 `meta.ClientModel` 匹配（`mod_ai_rate_limit.go:172,190`），与 #1387 的 `allow_models` 完全同型——限流规则按 provider 真实模型（如 `glm-5.2`）圈定、客户端发重定向别名（`glm-5.2-abc`）时策略匹配不上，限流形同虚设
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于含 #1387 修复的 HEAD，即 `reverseproxy.go` 已有 `ValidateTargetModel` 注入块 `:1577-1585`）
- 姊妹变更：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/`（本方案继承其「目标模型」定义；注入方式在其 `GetModule`+类型断言模式上**升级为框架回调点**，见 §2.0）

## 一、根因

### 根因链（代码级已核实）

1. **匹配对象**：`executeCheckLimitPolicy`（`bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go:155`）`:172` `clientModel := meta.ClientModel`，`:190` `matchModel(policy.Models, clientModel)` 决定策略是否适用；`:200-210` 三项检查（并发/RPM/TPM）把同一 `clientModel` 传入 `checkConcurrency/checkRPM/checkTPM`（`policy_limiter.go:268,320,360`），各规则项内部再 `matchModel(item.Models, model)` 过滤适用规则。**校验对象是请求体原始模型**，早于路由/重定向。
2. **检查时机**：`limitFoundProductHandler`（`mod_ai_rate_limit.go:112-121`）挂在 `HandleFoundProduct`（注册于 `:478`），执行点在 `reverseproxy.go:1194-1218`——**先于** `AiRouteResult` 检查（`:1220-1231`）与转发循环（`:1301`）。此时路由目标覆盖、StripPrefix、ModelMapping 均未应用，不存在目标模型。
3. **目标模型产生点**：`computeTargetModel`（`reverseproxy.go:1478-1509`）在 `doSingleAIForward` 内、每集群 attempt 重算（`:1567`），`:1570` 写入 `aiMeta.TargetModel`。`:1577-1585` 已为 #1387 注入 `ValidateTargetModel`——**本卡的注入区与目标模型来源与该块完全相同**。
4. **配置链路**：控制面（ai-gateway-api）限流策略 TPM/RPM 规则的 `models` 字段原样下发到 `PolicyConf.Rules.TPM[i].Models / RPM[i].Models`，无数据面转换；语义文档面向用户的描述是「适用模型」。

### 与 issue 原文的一处出入（按代码现状修正）

**「限流计数器按 model 分桶」表述不准确**：Redis 计数器键不含模型——`buildTpmRedisKey/buildRpmRedisKey` = `default_bfe_{policyId}_{instId}`（`policy_limiter.go:92-130`），`instId` 取自规则名或阈值参数拼接；并发键为 `default_bfe_{policyId}_con`（`:191`）。**模型只是规则项 applicability 的过滤维度，不是分桶维度**。由此得到两个对方案有利的结论：

- 切换匹配模型**零 Redis 键空间迁移**：无重定向流量 target==client，行为逐字节一致；有重定向流量共用同一规则计数器（语义上正是「按目标模型圈定」想要的）；
- 改动面比 issue 预估的小：复杂度不在计数器，而在**检查 relocation 后与 key 轮换 / 集群 fallback 的 429 交互**（见 §2.3，这是本方案的设计核心）。

## 二、修复步骤

核心思路：**限流策略检查后移到「目标模型已解析」之后**，匹配与规则过滤全部改用 `targetModel`；与 #1387 的「每 attempt 复校」不同，限流是**每请求仅检查一次**的预算语义，且本地 429 必须显式阻断 provider key 轮换与集群 fallback。

**注入方式（相对前版方案的修订）**：不采用 #1387 的 `srv.Modules.GetModule(...)` + 类型断言硬编码（`bfe_server` 反向依赖具体模块），而是**在 `bfe_module` 框架新增回调点** `HandleAfterAITargetModel`（目标模型解析后、转发执行前），由 `mod_ai_rate_limit` 以标准 `RequestFilter` 注册——模块间回到框架解耦，注册顺序复用既有模块序（token_auth 先于 rate_limit，`bfe_modules.go:149-160`），allow/block 与限流的相对顺序无需硬编码。

### 步骤 0（必改）：`bfe_module` 框架新增回调点 `HandleAfterAITargetModel`

框架现状：`bfe_module/bfe_callback.go:32-42` 为 iota 枚举回调点，`NewBfeCallbacks`（`:74-100`）为每个点建 HandlerList（本点用 `HandlersRequest`），`CallbackPointName`（`:44-67`）需补 case；调用方为 `srv.CallBacks.GetHandlerList(point).FilterRequest(basicReq)`，过滤器签名 `RequestFilter = func(req *bfe_basic.Request) (int, *bfe_http.Response)`（`bfe_handler_list.go:97-110`）——**无额外参数**：`targetModel` 由调用点在触发前写入 `aiMeta.TargetModel`（`:1570`），过滤器从 `req.GetAiBasicInfo().TargetModel` 读取，签名零改动。

改动内容：

```go
// bfe_module/bfe_callback.go（枚举追加在末尾，避免重排既有 iota 值）
const (
    // ... 既有回调点不变 ...
    // HandleAfterAITargetModel: AI 网关转发阶段回调——路由后、目标模型已解析
    // （aiMeta.TargetModel 已赋值）、转发执行前触发；按集群 attempt 触发
    //（key 轮换 / fallback 的每次 doSingleAIForward 均触发，注册方自行
    // 处理每请求一次语义）。
    HandleAfterAITargetModel
)
```

- `NewBfeCallbacks` 增 `bfeCallbacks.callbacks[HandleAfterAITargetModel] = NewHandlerList(HandlersRequest)`；
- `CallbackPointName` 增 case；
- `bfe_callback_test.go` 既有用例按新点数同步（如有总数断言）。

**命名与语义边界**：命名取 `HandleAfterAITargetModel`——构词沿用 `HandleAfterLocation` 的「After + 阶段名词」风格，「AI」字样标识这是 AI 网关专用回调点，与 `HandleAccept`/`HandleFoundProduct` 等通用点一眼区分；该点仅 `doSingleAIForward` 触发，非 AI 流（`EnableAiGateway=false`）不经过；与既有 `HandleForward`（`clusterInvoke` 内部、按后端触发，`reverseproxy.go:380-392`）区分——本点在选后端之前。

### 步骤 1（必改）：`mod_ai_rate_limit` 以 `RequestFilter` 注册到新回调点

`Init`（`mod_ai_rate_limit.go:446-501`）**变更注册**：注销 `HandleFoundProduct`（`:478-481`），改为：

```go
err = cbs.AddFilter(bfe_module.HandleAfterAITargetModel, m.targetModelCheckHandler)
```

新 handler（`func (m *ModuleAiRateLimit) targetModelCheckHandler(req *bfe_basic.Request) (int, *bfe_http.Response)`）实现要点（除模型来源与幂等守卫外，均为现状逻辑的平移）：

- **幂等守卫（每请求一次的关键）**：`if getPolicyLimiterContext(req) != nil { return bfe_module.BfeHandlerGoOn, nil }`——`PolicyLimiterContext`（req context 存储，`mod_ai_rate_limit.go:503-513`）兼作「已检查」标记；回调按 attempt 多次触发（key 轮换/fallback），守卫保证仅首个 attempt 真正检查；`HandleRequestFinish`（`:271-304`）本就以 ctx 存在为前提做 TPM 结算与并发释放，nil 时 no-op，**结算逻辑零改动**；
- `meta := req.GetAiBasicInfo()`，nil → GoOn；`targetModel := meta.TargetModel`；`req.InitAiRateLimitHitInfo()`（原 `:118`）移入；
- `runProductRules` / `executeCheckLimitPolicy` 主体平移：product 规则条件匹配、`meta.ClientApiKey` 为空放行（原 `:156-162`）、policyIds 为空放行（原 `:164-170`）、`policy.Enabled` 跳过——**唯一变化**：`:172` 的 `clientModel := meta.ClientModel` 改为 `targetModel`，`:190` `matchModel(policy.Models, targetModel)`，`:200-210` 三个 check 调用传 `targetModel`；
- `matchModel` 语义不动（`mod_ai_rate_limit.go:306-322`）：空 policy.Models 全放行、`*` 通配、空 targetModel 不命中（未配置 allow/block 且取不到模型时策略跳过，与现状 `matchModel(policy.Models, "")` 完全一致）；
- 拒绝路径 `executePolicyAction`（`:218-269`）平移，`req.ErrCode = bfe_basic.ErrAiRateLimit`（`:147` 语义保留），handler 返回 `(BfeHandlerFinish, resp)`——**`ErrCode` 是步骤 3 轮换/fallback 守卫的标记**；429 错误码映射已在 `bfe_basic/request_ai_basic.go:339-341`；
- **哨兵迁移（连带）**：`ErrAiRateLimit` 从 `mod_ai_rate_limit` 包内（`:54`）迁至 `bfe_basic/error_code.go` 全局定义——该文件是 `ErrBk*` 家族与 `ErrGslbBlackhole`（同为「模块拒绝」型 sentinel）的既有归属地；值 `"AI_RATE_LIMIT"` 不变，模块内引用（`:147`）随之更新。迁移后守卫比较 `basicReq.ErrCode == bfe_basic.ErrAiRateLimit`，`bfe_server` 不再为哨兵值 import 模块包；全仓引用仅 `:54`/`:147` 两处，无兼容别名必要；
- **保留** `HandleRequestFinish` 注册（`:483-486`）与全部 monitor/reload handler；
- 导出方法形态不再需要（不做 `bfe_server` 直接调用），原计划的 `CheckPolicyLimits` 降级为本 handler 的内部实现，单元测试同包直测不受影响。

### 步骤 2（必改）：`doSingleAIForward` 触发回调

`bfe_server/reverseproxy.go`，`aiMeta.TargetModel = targetModel`（`:1570`）之后、body 回写（`:1590`）之前：

```go
// record the final target model for this cluster attempt
aiMeta.TargetModel = targetModel                                     // :1570

// issue #1387 姊妹卡：AI 网关转发阶段回调（目标模型已解析、转发前）。
// 触发方只负责提供 aiMeta.TargetModel 与返回值映射，不感知具体模块。
hl = srv.CallBacks.GetHandlerList(bfe_module.HandleAfterAITargetModel)
if hl != nil {
    retVal, res = hl.FilterRequest(basicReq)
    basicReq.HttpResponse = res
    switch retVal {
    case bfe_module.BfeHandlerClose:
        return res, closeDirectly, nil, bodyModel
    case bfe_module.BfeHandlerFinish, bfe_module.BfeHandlerResponse:
        return res, closeAfterReply, nil, bodyModel
    }
}
```

与既有回调调用点（`:1194-1218`）的差异：转发阶段无 `goto send_response` 结构，直接以 `doSingleAIForward` 返回值表达（与 `:1531-1537`、`:1581` 的本地拒绝返回形态一致）；`BfeHandlerRedirect` 不支持（AI 流转发阶段无重定向语义，落入 switch 之外自然继续——实施时显式注释或记 warn）。

位置与顺序的核实结论：

- **顺序由注册序决定**：token_auth（`bfe_modules.go:150`）先于 rate_limit（`:160`）注册 → 若两者均注册本回调点，allow/block 先于限流执行——授权检查先于预算检查，避免为注定 400 `MODEL_NOT_ALLOWED` 的请求消耗 RPM/TPM 配额；与 ai-gateway-web §9.7「模型访问控制 → 限流 → 配额」的相对顺序一致，**无需在调用点硬编码模块依赖**；
- **覆盖无 provider key 路径**：`aiClusterInvoke` 早退分支（`:1671-1674`，`cluster.AIConf.Keys` 为空直接单次 `doSingleAIForward`）同样经过触发点；
- **`bfe_server` 反向依赖收敛**：本卡后 `bfe_server` 对 `mod_ai_rate_limit` 仅剩 session affinity 的 `RedisClient()` 获取（`:1698-1704`，存量问题，本卡不动）。

### 步骤 2+（连带，推荐）：#1387 的 `ValidateTargetModel` 迁移到同一回调点

#1387 以 `GetModule`+类型断言注入（`reverseproxy.go:1577-1585`），与本卡原方案同病。既然回调点已建，建议一并迁移（同一变更内完成，避免两种模式长期共存）：

- `mod_ai_token_auth`：新增 `RequestFilter` 包装 `ValidateTargetModel`（内部逻辑不变：读 `aiMeta.TargetModel`、无 context/无列表放行、空 target 400、`SetAiAuthInfo` + `ReqAuthFail` 计数、`details.Model` 填 targetModel），在 `Init` 注册到 `HandleAfterAITargetModel`；
- `reverseproxy.go`：删除 `:1577-1585` 的 `GetModule` 注入块，由步骤 2 的通用回调触发替代；
- **语义保持核实**：#1387 要求每 attempt 复校——回调按 attempt 触发天然满足；注册序保证其在限流前执行；400 拒绝返回值映射与 `:1581` 现状一致；
- **回归成本**：SC18 全场景需重跑（6 TC）。若评审决定拆分，本卡先只注册 rate_limit，#1387 迁移另立小卡——两种模式共存期间顺序仍正确（GetModule 块在回调点之前执行），但应设清理 TODO。

### 步骤 3（必改，本卡设计核心）：本地 429 阻断 key 轮换与集群 fallback

**问题**：429 ∈ `aiFallbackStatusCodes`（`:1802-1809`），且 key 轮换循环把 `statusCode == 429` 归类为「provider key 被限流」并轮换下一个 key（`:1763-1772`，含 session-affinity penalty）。本地策略 429 与上游 429 状态码相同——若不阻断：

- 轮换循环会把策略 429 误当 provider key 问题，白轮换全部 key 并附加 affinity penalty；若每轮换复校则计数膨胀（RPM `Check(1)` 每次检查计数），若不复校则换 key 后放行 → **限流被绕过**；
- fallback 循环对 429 默认触发（`shouldTriggerFallback` `:1829`），若后续 attempt 跳过已检查标记 → **限流被绕过**。

**方案**：沿用 `req.ErrCode` 标记（`bfe_basic` 中表达「BFE 内部拒绝原因」的既有载体，`findProduct`/模块拒绝均在用），在两个决策点加守卫。sentinel 按步骤 1 连带项迁移为全局定义 `bfe_basic.ErrAiRateLimit`：

1. **key 轮换循环**（`:1749` 成功检查之后、`:1762` switch 之前）：

```go
// #1387 姊妹卡：本地限流 429 不是 provider key 问题，停止轮换直接返回
if basicReq.ErrCode == bfe_basic.ErrAiRateLimit {
    return res, action, cluster, nil, newBodyModel
}
```

上游 429 不设 `ErrCode`（仅为 upstream 响应体），不受影响，轮换行为不变。

2. **集群 fallback 循环**（`:1328` `shouldTriggerFallback` 调用之前）：

```go
// #1387 姊妹卡：本地限流 429 不触发集群 fallback
if basicReq.ErrCode == bfe_basic.ErrAiRateLimit {
    break
}
```

时序核实：`resetRequestForRetry`（`:1842-1863`）在 attempt i>0 开头清空 `ErrCode`（`:1860`），而本 attempt 返回时标记尚在（守卫在 reset 之前执行）；本地 429 命中守卫即 return/break，不会再进入下一次 attempt 被清掉。实施时需复核 `ErrCode` 在该路径上无其他提前清空点（现有清空点仅 `:1860` 与 `findProduct` 等请求早期阶段，不在转发循环内）。

**备选（不采纳，记录备查）**：本地 429 响应加私有头（如 `X-Bfe-Local-Rate-Limit: 1`），`shouldTriggerFallback` 与轮换循环检查响应头、发送前剥离。更抗 `ErrCode` 被意外清空，但要改 `shouldTriggerFallback` 签名/响应头处理链路，改动面明显更大；`ErrCode` 是本仓库表达「BFE 内部拒绝原因」的既有载体（`findProduct`/模块拒绝均在用），取一致性。

### 步骤 4（文档同步，另仓）

- `ai-gateway-web`：限流「适用模型」语义从「客户端请求模型」改为「转发后目标模型」（与 #1387 的 allow/block 表述同型并列）；§9.7 运行时顺序重述为「限流/配额在鉴权阶段、模型访问控制与限流匹配对象均在转发阶段按目标模型」——准确表述见该仓文档章节结构；
- `ai-gateway-api`：限流策略 `models` 字段的 API 文档描述同步（适用模型 = 转发后模型名）；
- **配置语义切换提示（与 #1387 同型，需版本公告）**：现有「适用模型填客户端模型名」的限流配置仅在「无路由目标 Model 覆盖 + 无 StripPrefix + 无 ModelMapping」时与改动前等价；一旦启用重定向/裁剪/目标覆盖，圈定对象切换为转发后模型。

## 三、回归测试

### 单元测试（`testing` + `testify`，置于被测代码旁）

1. `bfe_module/bfe_callback_test.go`：新回调点注册/获取/名称映射用例；
2. `mod_ai_rate_limit_test.go` 迁移与新增：
   - 既有 `limitFoundProductHandler` / `executeCheckLimitPolicy` 用例迁移至 `targetModelCheckHandler` 直测（从 `req.AiBasicInfo.TargetModel` 读取，同包可直赋）；
   - 新增**幂等**：同一 req 二次调用（模拟 fallback 第二 attempt）返回 GoOn 且 limiter `Check` 不重复计数（fake limiter/agent 计调用次数）；
   - 新增**目标模型匹配**：policy.Models=[`glm-5.2`] + TargetModel=`glm-5.2` 命中；TargetModel=`glm-5.2-abc`（client 别名）不命中（修复前行为的镜像断言）；规则项级 `item.Models` 过滤同样按 targetModel；
   - 新增**拒绝标记**：429 响应返回 BfeHandlerFinish、`req.ErrCode == bfe_basic.ErrAiRateLimit`、hitInfo limit_type 正确（RPM/TPM/并发/redis-error 四分支）；
   - 新增 action 映射：Close → BfeHandlerClose，Pass → GoOn；
3. `policy_limiter_test.go`：check 函数 `model` 参数语义不变（纯过滤维度），既有用例不回退；
4. `bfe_server`：轮换守卫用例——`ErrCode == bfe_basic.ErrAiRateLimit` 时 `statusCode==429` 不进入轮换 case（usedSet 不变、无 penalty 调用）；fallback 守卫同理。若 `doSingleAIForward`/`aiClusterInvoke` 级 harness 成本过高（与 #1387 实施记录同因），由集成场景端到端覆盖，单元层只测守卫纯逻辑；
5. 若执行步骤 2+：`mod_ai_token_auth` 的 `RequestFilter` 包装直测（9 例从 SC18/模型检查用例平移，断言不变）。

### 集成测试（`bfe/tests/integration`，新场景 SC19，参照 SC07/SC18 结构）

- **TC-01 目标模型圈定生效**：provider 模型 `glm-5.2` + 集群 `ModelMapping glm-5.2-abc→glm-5.2` + 限流策略「适用模型=[`glm-5.2`]，RPM=1」+ 请求 `model=glm-5.2-abc` × 2 → 第 1 次 200，第 2 次 429 且 `limit_type=RPM`（修复前两次均 200，策略形同虚设）；
- **TC-02 本地 429 不轮换 provider key**：同 TC-01 配置 + 集群多 provider key → 第 2 次请求 429 且**所有 provider key 后端 Hits 均为 0**（未轮换、未转发）；无 affinity penalty key 生成；
- **TC-03 本地 429 不集群 fallback**：主目标策略 429 + fallback 集群可用 → 仍 429，fallback 集群后端 Hits=0；
- **TC-04 无重定向行为不退化**：无 ModelMapping/无目标覆盖 + 策略适用模型=[`glm-5.2`] + 请求 `model=glm-5.2` × 2 → 200 后 429（与改动前一致）；404（无路由）与鉴权失败请求**不计数**（重复请求仍 200——行为变化 §5.3 的显式断言）；
- **TC-05 并发上限单请求单槽位**：MaxConcurrency=1 + 主目标 5xx 触发 fallback → 并发计数不重复占用（第二请求不被误拒）；
- **TC-06 上游 429 语义不变**：模拟后端返回 429 → 仍触发 provider key 轮换（区别于本地 429）。

回归：SC07（Redis 键）、SC02（key 轮换）、SC08（fallback 重算）、SC18（#1387 语义；若执行步骤 2+ 则为其全量回归）、SC03（配额结算不受影响）。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确链路（issue 场景）

1. 客户端 `model=glm-5.2-abc`，限流策略「适用模型=[`glm-5.2`]，RPM=1」，Key 无 allow/block 列表；
2. `HandleFoundProduct`：token 鉴权通过；**限流不再在此检查**；
3. 路由命中 → `doSingleAIForward`：`targetModel = computeTargetModel(...)` → `glm-5.2`（`:1567`），`aiMeta.TargetModel = "glm-5.2"`（`:1570`）；
4. 触发 `HandleAfterAITargetModel` 回调（`:1570` 后）：token_auth 的 allow/block 校验（无列表放行）→ rate_limit 的 `targetModelCheckHandler`：`matchModel([glm-5.2], glm-5.2)` 命中 → RPM `Check(1)` → 首次放行 / 第二次 429（返回 BfeHandlerFinish + 429，`ErrCode=bfe_basic.ErrAiRateLimit`）；
5. 429 路径：`doSingleAIForward` 返回 429 → key 轮换守卫直接返回；fallback 守卫 break；客户端收到 429，provider key 零命中、无轮换、无 penalty。

## 五、行为变化清单

| 项 | 改动前 | 改动后 |
| --- | --- | --- |
| 匹配对象 | 请求体原始模型 `meta.ClientModel`（`mod_ai_rate_limit.go:172`） | `computeTargetModel` 输出（`reverseproxy.go:1567`，经 `aiMeta.TargetModel` 传入回调） |
| 检查时机 | `HandleFoundProduct`（路由前，每请求一次） | `HandleAfterAITargetModel` 回调（路由后、转发前，每请求一次——回调按 attempt 触发，幂等守卫保证单次） |
| 注入方式 | —（模块自注册 `HandleFoundProduct`） | 框架回调点；`bfe_server` 不反向依赖限流模块（session affinity 的 RedisClient 获取为存量遗留） |
| 早期拒绝请求的计数 | 无路由 404 / 鉴权失败 / 配额不足 / 协议不匹配（`:1530`）的请求**已计** RPM/预占 TPM | 不再计（预算只约束真正进入转发的请求；语义修正，需公告） |
| 本地策略 429 | 整请求拒绝（鉴权期短路，天然无轮换/fallback） | 整请求拒绝（步骤 3 显式阻断轮换与 fallback），响应/错误码/limit_type 不变 |
| 上游 provider 429 | 轮换 + fallback（`:1763-1772`、`:1808`） | 不变（`ErrCode` 区分本地/上游） |
| fallback 不同 target | 不感知（检查在路由前，无 target 概念） | 以首次 attempt 的 targetModel 判定，后续 attempt 不复校（预算语义，见 §7 非目标） |
| 计数器 / Redis 键 / Prometheus / hitInfo | — | 键空间与指标标签不变（`policy_limiter.go:92-130`）；hitInfo 生成点移至检查点 |
| 访问日志 | 限流拒绝时 `ai_target_model`=请求模型 | = 目标模型（#1387 后所有转发阶段拒绝均如此） |

## 六、验收标准对照（姊妹卡口径）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | 重定向 + 限流适用模型=[目标模型] → 按目标模型圈定，超限 429 `limit_type` 正确 | 步骤 1-2 + SC19 TC-01 |
| 2 | 本地 429 不触发 provider key 轮换、不触发集群 fallback、无 affinity penalty | 步骤 3 + SC19 TC-02/TC-03 |
| 3 | 上游 429 轮换/fallback 行为不变 | `ErrCode` 标记区分 + SC19 TC-06 |
| 4 | 无重定向/无覆盖行为不退化；计数不膨胀（fallback/轮换单次计数） | 步骤 1 幂等守卫 + SC19 TC-04/TC-05 |
| 5 | 早期拒绝（404/鉴权失败/协议不匹配）不计限流 | 检查点后移天然达成 + SC19 TC-04 |
| 6 | Redis 键空间、配置格式、控制面导出不改 | §1 核实结论（键不含模型） |
| 7 | 单元/集成测试覆盖 bfe_module 回调点、mod_ai_rate_limit、bfe_server 守卫 | 第三节全部 |
| 8 | 架构：模块经框架回调点注册，`bfe_server` 不硬编码限流模块调用 | 步骤 0-2 + 代码评审 |
| 9 | §9.7 等文档表述与新行为一致 | 步骤 4 |

## 七、边界与非目标

- **非 AI 流**：`EnableAiGateway=false` 的传统流不经过 `ServeHTTPForAI`/`doSingleAIForward`，新回调点永不触发，不受影响。
- **非目标：每 attempt 按各自 target 复校**。这是与 #1387 的刻意差异——allow/block 是授权语义（不同 target 权限不同，须每 attempt 复校），限流是预算语义（每请求一次判定；重复检查导致计数膨胀，跳过检查导致绕过）。fallback 到不同目标模型时策略不再重新匹配；若产品需要「按 fallback 目标分别限流」，另立需求评估。
- **不改**：Redis 键结构、限流配置 schema、控制面导出、TPM 预占-结算机制（`predictTokenUsage` + `HandleRequestFinish` delta）、配额（RMB）扣减链。
- **空 targetModel**：与现状 `matchModel(policy.Models, "")` 语义一致（策略跳过）；配置 allow/block 列表的请求会先被 allow/block 校验以 400 拦在限流之前，无双标。
- **Redis 故障 `isRejectOnRedisError` 语义**：平移不变（步骤 1 原样搬移）。
- **多实例部署**：计数器本就在共享 Redis，检查点后移不改变一致性模型。


## 八、实施记录（2026-09-23）

已按上述方案实施（含评审修订：回调点注入、AI 字样命名、全局 sentinel、步骤 2+ 一并迁移）。

| 文件 | 改动 |
| --- | --- |
| `bfe_module/bfe_callback.go` | 新增回调点 `HandleAfterAITargetModel`（枚举末尾追加、`NewBfeCallbacks` 注册 `HandlersRequest`、`CallbackPointName` case） |
| `bfe_module/bfe_callback_test.go` | 名称映射与回调点总数（9→10）同步 |
| `bfe_basic/error_code.go` | `ErrAiRateLimit` 全局 sentinel 迁入（值 `AI_RATE_LIMIT` 不变；`ErrGslbBlackhole` 同址同型） |
| `bfe_modules/mod_ai_rate_limit/mod_ai_rate_limit.go` | `limitFoundProductHandler` → `targetModelCheckHandler`：幂等守卫（`PolicyLimiterContext` 存在即放行，每请求一次）+ 从 `aiMeta.TargetModel` 取匹配模型；`executeCheckLimitPolicy` 改收 `targetModel`；`Init` 注销 `HandleFoundProduct`、注册新回调点；删除包内 `ErrAiRateLimit` 与 `errors` import |
| `bfe_modules/mod_ai_token_auth/model_check.go` | 新增 `targetModelCheckFilter`（`RequestFilter` 包装 `ValidateTargetModel`，每 attempt 复校语义保持） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `Init` 注册新回调点（先于 rate_limit，allow/block 恒在限流前执行） |
| `bfe_server/reverseproxy.go` | `doSingleAIForward` 在 `aiMeta.TargetModel` 赋值后触发通用回调（返回值映射 Close/Finish/Response），删除 `GetModule`+类型断言注入块与 `mod_ai_token_auth` import；key 轮换循环与集群 fallback 循环各加 `ErrCode == bfe_basic.ErrAiRateLimit` 守卫 |
| 单元测试 | `mod_ai_rate_limit_test.go`：6 个既有用例迁移至新 handler + 新增幂等/按 targetModel 不匹配跳过/匹配全贯通 3 例；`model_check_test.go` 新增 filter 包装 2 例 |
| 集成测试 | 新场景 `scenario-SC19-rate-limit-models-target-model`（TC-01 目标模型圈定 / TC-02 本地 429 不轮换 key / TC-03 本地 429 不 fallback / TC-04 无重定向不退化+404 不计数 / TC-05 并发单槽位 / TC-06 上游 429 轮换不变），设计文档 6 个 TC 并登记 `测试场景总体说明.md`；`common/redis_server.go` 追加 `Keys()`（TC-02 penalty key 断言用，additive） |

验证：

- `go test -cover ./...` + `go vet ./...`：全绿（**唯一例外** `scenario-SC13-tls-conf-reload-path` 两 TC 失败，经 `git stash` 干净树复现确认为 Windows 环境既有问题——`/reload/tls_conf` 响应把 Windows 绝对路径未转义拼进 JSON（`\U` 非法转义），与本次变更零文件交叠；此前未暴露是因 go test 结果缓存，本次 `tests/integration/common` 变更使缓存失效后真实执行才显现。建议另开 issue 修复，勿并入本卡）；
- SC19 全 6 TC 通过（无 t.Skip；TC-05/TC-06 借助 harness 既有 `MockBackend.ResponseFunc` 实现）；回归 SC18（18.2s，#1387 迁移无回归）、SC07、SC02、SC08、SC03 全绿；
- 与计划的偏离：无功能性偏离；TC-02 的「所有 provider key 后端 Hits=0」落地为「单后端 Hits=1 + distinct Authorization=1 + Redis 无 `:penalty:` key」（多 key 共享后端时以 auth header 多样性判定是否轮换）。

文档同步（步骤 4，另仓）：`ai-gateway-web` `10-api-key.md`（TPM/RPM 适用模型行、版本切换提示、§10.7 限流条目）与 `09-entity.md`（同型 3 处）；`ai-gateway-api` `OpenAPI接口定义/00-common.md`（TPM/RPM model 字段 + RateLimitPolicy 匹配语义与切换提示）与 `InnerAPI接口定义/rate-limit-policy.md`（导出 models 语义一句，appendix 无语义描述未改）。

遗留（另卡）：`09-entity.md` §9.5/§9.6 模型访问控制仍写「请求模型」，#1387 漏改 Entity 侧文档，与 10-api-key.md 新语义不一致。
