# expr 条件表达式引擎（第一期：条件引擎 + 旧原语兼容层）

日期：2026-10-30
系统设计详见 [sys_design/expr_condition_engine.md](../../sys_design/expr_condition_engine.md)

## 背景与目标

BFE 现有的条件表达式体系（`bfe_basic/condition`，goyacc 解析 + 60+ 固定搭配原语）
能力边界明显：仅有 `&&`/`||`/`!` 与原语布尔组合，无算术/比较/集合/正则谓词等
通用表达式能力，每新增一个匹配维度都要改 Go 代码并遵循命名约定。

go-bfe-pro 仓库已基于 `expr-lang/expr`（v1.17.8）实现了新一代表达式引擎
（`bfe_expr/` 包，约 4700 行），采用"双轨并存"策略：旧原语继续可用，新风格
表达式（取数函数 + expr 运算）与之混写，并通过 `legacyPatcher` 将旧原语名重写为
Env 方法实现统一求值。本改动将该能力**第一期**范围移植到 bfe：

- **条件表达式引擎**：`Cond` 字段支持 expr 风格表达式，编译期类型检查
  （结果必须 bool），运行期每请求构建一次 Env。
- **三模式引擎开关**：`LegacyFirst`（默认，旧引擎优先，失败回退新引擎）/
  `ExprCompatible`（新引擎 + 旧原语兼容）/ `ExprOnly`（纯新表达式）。
- **旧原语兼容层**：全部旧 condition 原语实现为 `Env` 方法（LegacyEnv），
  新旧可混写；常量参数经 `ValidateLegacyArgs` 保持"加载期拒绝非法配置"语义。

**不在本期范围**（第二期）：动作参数表达式（`=`/`==` 前缀，Param + action
钩子）、动态数据源（RedisEnv/HttpEnv/GlbEnv/ConfEnv，`bfe.data` Expr 段）。
本期代码结构上为第二期预留 `Param` 与扩展 Env 的接入点，但不实现。

## 总体设计

### 引擎选择（condition.Build 三模式分派）

`condition.Build` 是全仓库唯一条件构建入口（路由表 + 约 20 个模块的规则
配置均经此加载）。改造后按进程级 `EngineMode` 分派：

| 模式 | 行为 | 适用场景 |
|------|------|----------|
| `LegacyFirst`（默认） | 旧引擎优先：能按旧语法完整解析则走旧引擎（语义与旧版完全一致）；旧引擎编译失败才用新引擎编译，新引擎也失败则配置报错 | 存量集群、迁移期 |
| `ExprCompatible` | 新引擎 + `legacyPatcher` 重写旧原语名（新旧混写）；编译失败**不回退**，直接报错 | 已评估存量配置、灰度新引擎 |
| `ExprOnly` | 纯新表达式：旧原语名视为未知函数；编译失败不回退 | 验证新表达式独立覆盖 |

模式由 `bfe.conf` 新增 `ConditionEngineMode` 配置（支持字符串与 0/1/2 数字），
启动时经 `bfe_server` 初始化注入 `bfe_basic/condition` 包级状态。默认
`LegacyFirst` 保证存量配置行为 100% 不变。

### Env 设计（struct + 导出方法，PascalCase）

env 采用 struct，函数实现为各子 env 的**导出方法**，按域匿名嵌入：

```go
type Env struct {
    BaseEnv   // Req*/Res*/Ses*/Route*/BackendAddr 等取数
    TimeEnv   // Now / InTimeRange / InPeriodicRange
    HashEnv   // Hash / IpIn / IpRange / Rand / RandInt / Srand
    VarEnv    // UsrVarStr/Int/Float/Bool + UsrVarSetXxx + ReqContext
    InnerEnv  // %var 内置变量（BfeClientIp/BfeLogid 等，本期一并移植，主要为二期动作参数铺路）
    LegacyEnv // 旧原语函数化兼容（legacyNameMap 全量原语）
}
```

- 编译期 `expr.Env(&Env{})` + `expr.AsBool()` 强类型检查；
- 运行期每请求 `NewEnv(req)` 一次，同一请求生命周期内复用（用户变量、随机源
  的"每请求一致"语义由此保证）；
- **命名即配置接口**：方法名是面向配置的 API，发布前冻结，不随意改名。

### 求值约定

- 表达式结果必须是 bool，编译期强制；
- 取数函数数据缺失返回零值（`""`/`0`/`[]`/`false`），不视为错误；
- 运行期求值出错：该条规则条件按 `false` 处理 + warn 日志（带 logid 与表达式
  串），不影响其他规则与请求处理；
- 条件表达式允许受控副作用（`UsrVarSetXxx` + `; false` 序列表达式写法），
  用于跨规则传递中间结果（种子规则模式）。

## 主要改动点

### 新增：`bfe_expr/` 包（移植自 go-bfe-pro，按 bfe 适配）

| 文件 | 职责 |
|------|------|
| `env.go` | `Env` 组合结构、`NewEnv(req)`、每请求 Env 缓存（经 `req.Context`） |
| `base_env.go` | `BaseEnv`：Req*（method/host/path/query/header/cookie/body/bodyJson/cip/sip/vip…）、Res*、Route*、Ses*、ReqTags |
| `time_env.go` | `TimeEnv`：`Now()`（尊重 `X-Bfe-Debug-Time`）、`InTimeRange`、`InPeriodicRange` |
| `hash_env.go` | `HashEnv`：`Hash`（murmur3 桶 10000，空输入每请求稳定随机桶）、`IpIn`（支持 CIDR）、`IpRange`、`Rand`/`RandInt`/`Srand` |
| `var_env.go` | `VarEnv`：`UsrVar*`/`UsrVarSet*`（与 legacy `USR_VAR_SET` string 存储互操作）、`ReqContext` |
| `inner_env.go` | `InnerEnv`：`%var` 内置变量函数化（BfeClientIp/BfeLogid/BfeVip/BfeSslVersion…） |
| `legacy_env.go` | `LegacyEnv`：旧原语同名方法实现，语义逐项对照 `primitive.go` |
| `legacy_patcher.go` | `expr.Patch`：编译期把旧原语名重写为 Env 方法名；`legacyNameMap` 即兼容清单；`ValidateLegacyArgs` 加载期常量校验 |
| `program.go` | `Condition`：加载期 `expr.Compile`，运行期 `Match(req)` 求值，错误按 false + warn |

### 修改：`bfe_basic/condition/build.go`

新增包级 `EngineMode` 与 `SetEngineMode`；`Build` 按模式三分支（见上表）。
LegacyFirst 下先走现有 parser，失败再 `bfe_expr` 编译；ExprCompatible/ExprOnly
直接经 `bfe_expr`（含 patcher）。

### 修改：`bfe_config/bfe_conf/conf_basic.go`

`ConfigBasic`（`[server]` 段）新增 `ConditionEngineMode string`：空/缺省 =
`LegacyFirst`；兼容 `"0"/"1"/"2"` 数字写法；非法值启动报错。`conf/bfe.conf`
样例同步补充注释。

### 修改：`bfe_server` 初始化

启动路径（`bfe_server_init.go` 附近）在配置加载后调用
`condition.SetEngineMode(...)`，再做模块/路由加载——保证所有 `condition.Build`
调用点拿到正确模式。

### 移植适配点（bfe 与 go-bfe-pro 的 bfe_basic 差异核对清单）

- `bfe_basic.Request` 字段/`Context` 缓存机制、`GetAiBasicInfo`（AI 原语
  `apikey_in`/`model_in` 依赖）、tag 结构、`BfeCluster` 命名等逐项核对；
- `ReqBodyJson` 依赖 gjson，随本期引入依赖；
- 旧原语集合以 bfe 当前 `primitive.go` 为准做对拍（两仓库原语清单可能存在
  差异，如 bfe 的 `req_ai_intent_in`、`time_weekday_in`），`legacyNameMap`
  按 bfe 实际原语生成，缺失的原语在 ExprCompatible/ExprOnly 下明确报错。

### 测试

- **单测**：`bfe_expr/*_test.go` 移植（env/param 基础用 httptest 构造请求）；
  `bfe_basic/condition/build_expr_test.go`（三模式下返回类型/分派行为）；
  `legacy_compare_test.go`（**新旧引擎对拍**，旧引擎为 oracle，按 bfe 原语
  清单覆盖，重点固化转义语义差异）。
- **集成测试**：`tests/integration` 新增 scenario（ExprOnly 路由、
  ExprCompatible 新旧混写、LegacyFirst 回退），按 bfe-rd-workflow Phase 5/6
  先写测试设计文档。e2e 模板参考 go-bfe-pro `test/e2e/scenarios/route_expr_test.go`
  的场景设计，但用 bfe 集成测试框架落地。
- **回归**：全部既有单测 + 集成测试在默认 LegacyFirst 下必须通过（零回退）。

## 配置影响

- `bfe.conf` `[server]` 段新增 `ConditionEngineMode`（string，选填，默认
  `LegacyFirst`）。仅在新模式下 `Cond` 字段的语法约束变化；其余配置文件
  （路由、各模块规则）格式不变。
- 中英文 configuration 文档（`docs/{zh_cn,en_us}/configuration/`）同步补充
  该字段说明。
- 用户文档：`docs/zh_cn/condition/` 新增 expr 表达式章节（语法、内置函数、
  新旧对照、模式说明、转义规则），英文 `docs/en_us/condition/` 同步。

## 兼容性说明

- **默认 LegacyFirst 零回退**：存量配置全部继续走旧引擎，行为与旧版一致；
  只有旧引擎解析失败（新风格/混写表达式）才进入新引擎。
- **字符串转义差异（有意决策）**：expr 对所有字面量统一按 Go 规则解码（双/单
  引号），反引号不解码；旧引擎各原语转义处理三分支不一致（regmatch/gjson path
  透传派 vs `*_in` Unquote 派）。同一条件串在 `ExprCompatible`/`ExprOnly` 下
  语义可能与旧引擎不同（典型：正则 `\.` 在新引擎编译失败），迁移前需按
  转义对照表改写（`\\.` / 反引号 / `[.]`）。此差异由对拍测试固化。
- **加载期校验对齐**：正则/哈希区间/时间格式/IP 区间/变量名等常量参数经
  `ValidateLegacyArgs` 编译期校验，保持旧实现"加载期拒绝非法配置"的行为。
- **已知差异（记录不修复）**：
  - expr 检查器盲区：bool 形参位置的未知标识符（如大写 `FALSE`）编译期不报错、
    运行期按 false；旧引擎加载期报错（配置笔误场景）；
  - 旧原语方法的哈希空输入保持"每次调用新随机桶"旧语义，新式 `Hash()` 为
    每请求稳定随机桶，两者并存各有文档；
  - `res_code_in` 旧原语方法保持字符串比较语义，新式 `ResCode()` 为 int 比较。
- **AI 原语**：`apikey_in`/`model_in`/`req_ai_intent_in` 等作为旧原语在
  LegacyEnv 中兼容（读 `req.GetAiBasicInfo()` 等既有接口），新式 AI 函数本期
  不注册（沿用 go-bfe-pro 决策），需要时由 AI 模块按需扩展。
- 模块侧（路由 + 各 mod_*）**零改动**：全部经 `condition.Build` 统一分派。

## 第二期范围预告

动作参数表达式（`Param`、`=`/`==` 前缀、action 钩子注入 `bfe_basic/action`、
mod_header/mod_redirect/mod_cookie 接入）与动态数据源（RedisEnv/HttpEnv/GlbEnv/
ConfEnv、`bfe.data` Expr 段与 `ApplyExprConf` 热更）另行立项，不在本期。

## 修订记录

### 2026-10-30：初稿

第一期范围确立：条件引擎三模式 + 旧原语函数化兼容层 + 配置开关；含
Env 组合结构设计、求值约定、转义差异与已知差异清单、测试与文档计划。
