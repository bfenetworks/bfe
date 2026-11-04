# BFE Issue #1476 修复方案：越过配额边界的请求只记到配额上限

- Issue: https://github.com/bfenetworks/bfe/issues/1476
- 缺陷：API Key 额度快用完时，最后一次请求的实际成本超过剩余额度，请求照常完成，但 Redis 余额被截断在 0，超出部分静默丢弃。开启"额度不足时放行"（`PassNoQuota`）后，余额一直钉在 0，下游控制台已用额度恒等于配额上限，看不出实际消耗。
- 代码库：`bfe/`（行号基于 HEAD `963438a5`，2026-10-30，逐条核实）
- 配套改动：ai-gateway-api 侧有 3 处"负值钳零"的 used 派生逻辑需同步调整（`fillQuotaBalance` ×2、`adjustQuota`），方案见该仓 `design-docs/modifications/2026-10-30-issue-1476-quota-overage-used-derivation/`。本文档只覆盖 bfe 侧；两侧通过同一个 Redis key 耦合但前后向兼容，可分开发布。

## 一、根因

### 1.1 扣减 Lua 脚本截断式丢弃超额

`QuotaPlan.Deduct`（`bfe_modules/mod_ai_token_auth/token.go:73-91`）按单位分派到 `deductToken`（`:93-115`）与 `deductRMB`（`:117-146`），两个脚本同构：

```lua
-- deductToken（:94-102）
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local amount = tonumber(ARGV[1])
local deduct = math.min(current, amount)      -- ← 扣减量被截断到当前余额
if deduct > 0 then
    redis.call('DECRBY', KEYS[1], deduct)
end
return math.max(0, current - deduct)          -- ← 返回值钳 0
```

`deduct = math.min(current, amount)` 使超出剩余额度的部分**根本不进入 Redis**：余额值域被钉在 `[0, quota]`，overrun 数据在扣减那一刻永久丢失。`deductRMB` 的首访初始化分支（key 不存在时 `SET` 为配额总量）不影响这一结论。

### 1.2 放行模式只是同一根因的放大

"额度不足时放行"（`PassNoQuota`）的语义仅是**鉴权期**跳过余额检查（`token_rule_table.go:149`）；收尾扣减循环（`mod_ai_token_auth.go:576-599`）对**所有**非 Unlimited 计划照常执行 `Deduct`，于是被同一个 `math.min` 截断——放行后余额钉 0、后续所有请求的实际消耗全部丢失，正是 issue 描述的"已用额度一直停在额度上限"。

### 1.3 为什么必须在 bfe 侧修

Redis 中 `QUOTA_<owner>`（key 由 ai-gateway-api 在导出 bfe 配置时生成，`ai-gateway-api/model/imods/exporter.go:603`）是余额唯一真实来源，**只有剩余量、没有独立的用量累计**。下游 ai-gateway-api 的"已用额度"全部由 `quota − remaining` 派生（`api_key.go:330`、`entity_manager.go:609`、`quota_plan_manager.go:416`），remaining 被截断后派生值恒等于 quota。因此超额信息的丢失发生在 bfe 扣减瞬间，下游任何改动都无法找回，必须在扣减源头改为全额记录。

## 二、修复方案（仅 bfe）

设计决策：**余额可为负（方案A），不引入独立 used 计数器**。Redis 数据模型不变（仍是单 key 存 remaining 定点整数），无迁移；`HasBalance` 的 `current > 0` 与 Batch 预检 `balance − reserved < units` 对负余额天然判负，鉴权语义零改动；`DECRBY` 单命令原子性不变。

### 步骤 1（必改）：`deductToken` 全额扣减

文件：`bfe_modules/mod_ai_token_auth/token.go:93-115`。去掉 `math.min` 截断与返回值的 `math.max(0, ...)`：

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local amount = tonumber(ARGV[1])
redis.call('DECRBY', KEYS[1], amount)   -- 全额扣减，余额可为负
return current - amount
```

`amount <= 0` 的守卫在 Go 侧 `Deduct`（`:74-76`）已有；amount 恒为正整数（RMB 为 1e-8 定点整数），`DECRBY` 无精度问题。

### 步骤 2（必改）：`deductRMB` 全额扣减

同文件 `:117-146`。首访初始化分支（key 不存在时 `SET` 为配额总量）保留，同样去掉截断：

```lua
local raw = redis.call('GET', KEYS[1])
local current
if raw == false then
    current = tonumber(ARGV[2])
    redis.call('SET', KEYS[1], current)   -- 首访初始化不变
else
    current = tonumber(raw)
end
local cost = tonumber(ARGV[1])
redis.call('DECRBY', KEYS[1], cost)       -- 全额扣减，余额可为负
return current - cost
```

首次请求即超额的场景下 remaining 直接为负，语义正确。

### 步骤 3（建议）：超额可观测信号

扣减返回 remaining < 0 表示发生欠费（放行模式下会持续累计）。在 ModuleState（`mod_ai_token_auth.go:115` 一带，参考 `DeductFailed`/`BillingZeroCost` 注册方式）新增计数器 `QuotaOverage`，触发时记一行 WARN（plan id、amount、扣后 remaining）。不阻断流程，纯信号，便于发现放行模式被滥用导致的欠费累计。

### 不需要改动的点（已核实）

| 位置 | 原因 |
|---|---|
| `token.go:148-168` `HasBalance` | 判定 `current > 0`：负余额 → false → 429，天然正确 |
| `token_rule_table.go:149-167` 鉴权期检查 | 同上；`PassNoQuota` 跳过余额检查的语义不变（放行只影响鉴权，不影响记账） |
| `mod_ai_token_auth.go:576-599` 收尾扣减循环 | 只调 `Deduct`；修复后放行模式下也能全额记账，正是 issue 需求 |
| `batch_quota.go:107-140` `BatchPreCheck` | `balance − reserved < units`：负 balance 自动拒绝 |
| `batch_quota.go:306-313` Batch 结算扣减 | 走同一个 `Deduct`，Lua 改后自动全额结算 |
| Unlimited 分支 | 不触 Redis，不变 |

## 三、与既有语义的兼容性（安全边界）

1. **Redis 数据格式零变化**：key、定点整数 value 均不变，**无迁移**。
2. **新 bfe 读老 key**：兼容。
3. **老 bfe 读负余额 key（混部/回滚）**：`min(neg, amount) = neg ≤ 0` → `if deduct > 0` 不成立 → 不扣减、返回 `max(0,…)` = 0。不损坏数据，但该请求超额丢失（与修复前现状一致）。可接受。
4. **新 bfe + 老 ai-gateway-api（混部）**：老 api 展示层把负 remaining 钳 0，控制台已用额度暂显 quota，数据无损，api 升级后自动正确。可接受。
5. **值域变化**：`QUOTA_<owner>` 从 `[0, quota]` 扩为 `(-∞, quota]`。所有读方（bfe 自身、`ai-gateway-api/model/quotacache`、报表）已逐一核实。
6. **已知局限**：升级前已被钉在 0 的存量 key，其历史超额已丢失，无法找回；这些 key 的 used 从 quota 起继续累计，略低于真实总消耗。接受。

## 四、回归测试

### 单元测试（`mod_ai_token_auth_test.go`）

- 更新 `TestQuotaPlanDeduct`（`:1849-1908`）现有用例预期：超额场景 remaining 返回负数；
- 新增用例：
  - `deductToken` 超额：余额 100、扣 200 → 返回 −100，Redis 值为 −100；
  - `deductRMB` 首访即超额：quota 100、扣 200 → 返回 −100；
  - `deductRMB` 欠费后再扣：余额 −50、扣 30 → 返回 −80；
  - `HasBalance` 负余额 → false。

### 集成测试（integration-test 仓，SC03 新增 TC-32/33/34）

实现于 `tests/integration/implementation/scenario-SC03-rmb-quota/sc03_quota_overage_test.go`（miniredis 真实执行 Lua 脚本），设计文档见 `tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/TC-32`/`TC-33`/`TC-34`：

1. **TC-32 RMB 超额全额记录**（pass=否）：余额 10000、单次成本 20000 → 请求 200 后 Redis remaining = **−10000**，`QuotaOverage` 计数 ≥1，下一次请求 429。
2. **TC-33 放行模式欠费累计**（pass=是）：连续 3 次请求均 200，remaining 累计至 **−50000**，`QuotaOverage` 计数 ≥3。
3. **TC-34 total_token 超额**：usage 150 token 对余额 100 全额扣减，remaining = **−50**。

回归：TC-01（正常扣减）/TC-02（余额 0 拒绝）等存量 SC03 用例全量通过；Batch 预检/结算语义经 `mod_ai_batch` 单测与 SC28 覆盖。

## 五、修复后正确行为（目标链路）

- 放行=否：超额请求完成，remaining 变负；后续请求鉴权拒绝；已用额度如实等于实际消耗（可超过配额）。
- 放行=是：每次请求全额扣减，remaining 持续变负，已用额度持续累计——控制台可见真实消耗。
- 管理员在欠费状态调大配额：`ai-gateway-api` 的 `adjustQuota` 按 `newQuota − used` 计算目标剩余（可为负），负值表示新配额仍不足以覆盖欠费，鉴权继续拒绝，不凭空赠送额度。

## 六、实施记录（2026-10-30）

已按本方案完成 bfe 侧实现，单测全绿（`go test ./bfe_modules/mod_ai_token_auth/`、`./bfe_modules/mod_ai_batch/`，`go vet` 通过）：

- `token.go`：`deductToken`/`deductRMB` 两个 Lua 脚本去掉 `math.min` 截断与 `math.max(0,…)` 钳制，改为 `DECRBY` 全额扣减、返回可为负的 remaining（`deductRMB` 首访 SET 初始化分支保留）；`Deduct` 统一捕获 remaining < 0 时调用 `noteQuotaOverage`（欠费计数 + WARN，经 `defaultTokenAuthModule` 空值守卫，单测裸调 `QuotaPlan.Deduct` 安全）。
- `mod_ai_token_auth.go`：`ModuleAITokenAuthState` 新增 `QuotaOverage *metrics.Counter`（`Metrics.Init` 反射初始化，无需额外注册）。
- `mod_ai_token_auth_test.go`：mock Redis 脚本仿真同步为全额扣减语义；`TestQuotaPlanDeduct` 的 "rmb deduct insufficient" 改为 "rmb deduct overage"（预期 −100），新增 token 超额、RMB 首访即超额、欠费后再扣三个用例；`TestQuotaPlanHasBalance` 新增负余额判无用例。

遗留：ai-gateway-api 侧配套改动（`fillQuotaBalance` ×2、`adjustQuota` 去钳制）未实施，按该仓 `design-docs/modifications/2026-10-30-issue-1476-quota-overage-used-derivation/` 另行安排；bfe 侧集成用例 TC-32/33/34 已落地并通过（见上节），跨两侧端到端验收（控制台已用额度口径）待 api 侧合入后重跑 issue 原测试用例。
