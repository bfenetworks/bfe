# expr 条件表达式引擎设计

日期：2026-10-30（第二期更新：2026-10-31）
关联：
- [modifications/2026-10-30-expr-condition-engine](../modifications/2026-10-30-expr-condition-engine/design-changes.md)（第一期）
- [modifications/2026-10-31-action-expr-dynamic-env](../modifications/2026-10-31-action-expr-dynamic-env/design-changes.md)（第二期）

## 1. 背景与目标

BFE 既有条件体系（`bfe_basic/condition`：goyacc 解析 + 60+ 固定搭配原语）只有
`&&`/`||`/`!` 布尔组合能力，新增匹配维度必须修改 Go 代码。本设计引入基于
[expr-lang/expr](https://expr-lang.org/) 的通用表达式引擎作为**第二引擎**，与
旧引擎双轨并存：

- 旧原语继续可用，语义不变（默认模式下行为与旧版完全一致）；
- 新风格表达式（取数函数 + expr 运算/比较/集合/正则谓词）可表达旧原语无法表达
  的逻辑（算术、数值区间、多值谓词、跨规则中间结果传递等）；
- 全部条件消费方（路由表 + 各 mod_* 模块规则）经 `condition.Build` 统一分派，
  模块侧零改动。

本设计分两期落地：

- **第一期**：条件引擎 + 旧原语函数化兼容层 + 三模式开关（§1～§7）；
- **第二期**：动作参数内嵌表达式（§8）与动态数据源（§9：`RedisEnv` 最小实现，
  `HttpEnv`/`GlbEnv`/`ConfEnv` 接口 + stub）。

## 2. 方案选型

### 2.1 为什么选 expr

- 编译期强类型检查（`expr.Env(&Env{})` + `expr.AsBool()`）：类型错误、未知
  函数、结果非 bool 在配置加载期暴露，而非运行期；
- 无副作用、无循环：表达式求值有确定上界，配置错误不会拖死 worker；
- 内置函数丰富（字符串/集合/谓词/类型转换），避免自研求值器的长尾维护成本；
- RE2 正则（`matches`），无回溯爆炸风险。

### 2.2 双轨而非替换

存量配置规模大且旧引擎字符串转义语义三分支不一致（见 §6），一刀切替换风险
不可控。采用进程级三模式开关渐进迁移：

| 模式 | 行为 |
|------|------|
| `LegacyFirst`（默认） | 旧引擎优先；旧引擎解析失败（新风格/混写）才用 expr 引擎；expr 编译失败则配置报错 |
| `ExprCompatible` | expr 引擎 + `legacyPatcher` 重写旧原语名（新旧混写）；编译失败直接报错，不回退 |
| `ExprOnly` | 纯 expr；旧原语名视为未知函数 |

旧原语不做"AST 自动翻译"，而是**函数化兼容**：每个旧原语实现为 `Env` 方法
（LegacyEnv），patcher 在类型检查前把原语名重写为方法名。无代码生成层、无
字符串层改写，规避转义二次处理问题。

## 3. 架构与数据流

### 3.1 组件

```
┌─────────────────────────────────────────────────────────┐
│ 配置加载期（每文件一次）                                   │
│  route_rule.data / mod_*.data ──► condition.Build(condStr)│
│                                   │ 按 EngineMode 分派     │
│     ┌─────────────────────────────┼──────────────────┐    │
│     ▼ LegacyFirst                 ▼ ExprCompatible/  │    │
│  旧 parser (goyacc)            ExprOnly                │    │
│  成功→Legacy Cond              bfe_expr.Compile:       │    │
│  失败▼                         expr.Env(&Env{})+AsBool │
│  bfe_expr.Compile ◄────────  legacy_patcher 重写旧名    │    │
│     │ 成功→*bfe_expr.Condition   ValidateLegacyArgs    │    │
│     │ 失败→配置加载报错          常量参数校验            │    │
└─────┼──────────────────────────────────────────────────┘
      ▼
运行期（每请求）
  NewEnv(req) 一次（缓存于 req.Context）──► cond.Match(req)
                                              │ expr.Run(prog, env)
                                              ▼
                                     出错→按 false + warn(带 logid)
```

动作参数表达式不经过 `condition.Build`：加载期对每个 `=` 参数单独编译为
`bfe_expr.Param`，运行期在动作执行处求值，详见 §8。

### 3.2 Env 设计（struct + 导出方法）

```go
type Env struct {
    BaseEnv   // Req*/Res*/Ses*/Route*/BackendAddr 取数
    TimeEnv   // Now / InTimeRange / InPeriodicRange
    HashEnv   // Hash / IpIn(CIDR) / IpRange / Rand / RandInt / Srand
    VarEnv    // UsrVar*/UsrVarSet*/ReqContext
    RedisEnv  // GetRedisClient* / RedisClient.Get|GetInt|HGetAll（第二期，见 §9）
    HttpEnv   // 接口 + stub（第二期，见 §9）
    GlbEnv    // 接口 + stub（第二期，见 §9）
    ConfEnv   // 接口 + stub（第二期，见 §9）
    InnerEnv  // %var 内置变量函数化（BfeClientIp 等）
    LegacyEnv // 旧原语同名方法（legacyNameMap 全量）
}
```

- 编译期以 `&Env{}` 为类型环境做完整类型检查；运行期 `expr.Run` 传 `*Env`
  （指针接收者方法集）；
- 每请求 `NewEnv(req)` 一次并缓存于 `req.Context`：同一请求内所有表达式
  求值共享同一上下文，用户变量读写与随机源语义由此保证"每请求一致"；
- 随机源随 `NewEnv` 按请求隔离（`math/rand/v2` 风格按时间播种），`Srand(seed)`
  只重播种本请求；
- 动作参数表达式与条件表达式复用同一 `Env`（同一函数集、同一请求级实例），
  区别仅在编译期要求结果为 string（见 §8）；
- **命名即配置接口**：方法名面向配置发布，冻结不改。

### 3.3 求值约定

- 结果必须 bool（编译期 `expr.AsBool()` 强制）；
- 取数函数数据缺失返回零值（`""`/`0`/`[]`/`false`），不视为错误；
- 运行期求值错误：该条规则条件按 `false` + warn 日志（logid + 表达式串），
  不影响其他规则与请求；
- 条件表达式允许受控副作用（`UsrVarSetXxx(...); false` 序列表达式）：规则按
  配置顺序求值、每条规则 Cond 一次请求至多求值一次，因此"前排种子规则写、
  后续规则读"是可靠的跨规则传递机制；写入值跨模块可见（存于请求级 Context）。

### 3.4 状态存储

无持久状态。引擎级状态仅：

| 状态 | 位置 | 生命周期 |
|------|------|----------|
| EngineMode | `bfe_basic/condition` 包级变量 | 进程级，启动时由 bfe.conf `ConditionEngineMode` 注入 |
| 编译产物 `*vm.Program` | 各规则结构体内（随配置热更替换） | 配置版本生命周期 |
| 每请求 Env / 用户变量 | `req.Context` | 请求生命周期 |
| 动作参数编译产物 `*Param` | 各 action 结构体内（随配置热更替换） | 配置版本生命周期 |
| Redis client 单例表 | `bfe_expr` 包级 map（key = 地址 + 参数组合） | 进程级，随进程退出回收 |

## 4. 模式注入与配置

`bfe.conf [server]` 段 `ConditionEngineMode`（默认 `LegacyFirst`，兼容
`0`/`1`/`2`）。`bfe_server` 启动时在加载路由/模块配置**之前**调用
`condition.SetEngineMode(...)`，保证所有 `condition.Build` 调用点行为一致。
配置热更不影响模式（模式变更需重启）。

该开关只作用于**条件表达式**；动作参数表达式是独立编译路径，不受模式影响，
任何模式下 `=` 前缀参数都按 expr 编译（见 §8）。

## 5. 边界情况与优化

- **性能**：表达式编译全部发生在加载期，运行期仅 `Run`；`IpIn` 网段列表
  运行期每次调用解析（对齐 go-bfe-pro 现状，热路径优化留待 benchmark）；
  `ReqBody()`/`ReqBodyJson()` 有 IO 成本，文档标注谨慎用于热路径；
  Env 复用经 `req.Context` 缓存，避免每规则重复构造。
- **求值顺序**：expr 不保证子表达式求值顺序，同一表达式内"写后读同一变量"
  须用 `UsrVarSetInt("k", v) > 100` 写判一体形式。
- **expr 检查器盲区**：bool 形参位置的未知标识符（如大写 `FALSE`）编译期
  不拒绝、运行期按 false（旧引擎加载期报错）——配置笔误场景，记录不修复。
- **AI 原语**：`apikey_in`/`model_in`/`req_ai_intent_in` 等经 LegacyEnv 兼容，
  读既有 `req.GetAiBasicInfo()` 等接口；新式 AI 函数本期不注册。
- **动作参数求值**：`=` 参数在加载期逐个预编译，运行期与条件表达式同源求值；
  单参数失败置 `""` + warn（带 logid），不阻断同一动作的其他参数与后续动作。
- **Redis 访问**：表达式内 Redis 读是同步网络 IO，会占用处理该请求的
  goroutine；默认超时 conn 100ms / read 300ms / write 300ms，超时或错误按
  零值返回（不报错、不重试），连接池单例见 §9。

## 6. 新旧引擎已知差异（有意决策，对拍测试固化）

1. **字符串转义**：expr 对双/单引号字面量统一按 Go 规则解码、反引号不解码；
   旧引擎三分支不一致（`*_in` 系 `strconv.Unquote` 解码；regmatch/gjson path
   原样透传；scanner 白名单只校验不解码）。同一条件串在 `ExprCompatible`/
   `ExprOnly` 下语义可能不同（典型：正则 `\.` 新引擎编译失败，须改写为
   `\\.` / 反引号 / `[.]`）。默认 `LegacyFirst` 100% 复现旧行为。
2. **加载期校验**：`ValidateLegacyArgs` 保持旧实现"加载期拒绝非法配置"语义
   （正则/哈希区间/时间格式/IP 区间/变量名等常量参数）。
3. **哈希空输入**：旧原语方法保持"每次调用新随机桶"旧语义；新式 `Hash()`
   为空输入提供"每请求稳定随机桶"，两者并存各有文档。
4. **`res_code_in`**：旧原语方法保持字符串比较；新式 `ResCode()` 为 int 比较。

## 7. 测试策略

- `legacy_compare_test.go`：新旧引擎对拍，旧引擎为 oracle，按 bfe 实际原语
  清单覆盖，重点固化 §6 差异；
- `build_expr_test.go`：三模式下分派与返回类型；
- `bfe_expr/*_test.go`：各子 Env 行为；
- `param_test.go` / `action_hook_test.go`：`=`/`==`/字面量三态、返回值类型
  检查失败、求值失败置空继续、注入与未注入两种行为；
- `redis_env_test.go`：用 `miniredis`（go.mod 已依赖）验证
  `Get`/`GetInt`/`HGetAll`、miss/错误零值、单例复用；
- 动作参数接入用例：`bfe_basic/action`、`mod_redirect`、`mod_header`
  的 `=` 参数；
- 集成测试（`tests/integration`）：ExprOnly 路由、ExprCompatible 混写、
  LegacyFirst 回退三场景；
- 回归：默认 LegacyFirst 下全部既有单测与集成测试零回退。

## 8. 动作参数表达式（第二期）

### 8.1 语法与编译

部分模块的动作参数支持以 `=` 前缀内嵌表达式，参数写法分三态：

| 写法 | 语义 | 处理 |
| ---- | ---- | ---- |
| `=expr` | expr 表达式，结果必须为 string | 加载期编译 + 类型检查；失败即配置加载失败，不回退字面量 |
| `==lit` | 转义字面量，首个 `=` 转义：`==foo` → `=foo`，`==` → `=` | 加载期直接转义 |
| 其他 | 普通字面量，原样使用 | 不编译，零开销 |

编译使用 `expr.Compile(code, expr.Env(&Env{}), expr.AsKind(reflect.String))`，
因此可用函数集与条件表达式完全一致（取数函数、内置变量函数、用户变量、
Redis 等），差别仅在于**结果类型必须是 string**。

### 8.2 运行期约定

- 编译产物为 `bfe_expr.Param`（`*vm.Program` + 字面量），随所属配置结构体
  一起被热更替换；
- 每个参数运行期经 `GetEnv(req)` 取请求级 `Env` 求值，与同一请求的条件
  表达式共享上下文（用户变量、随机源等）；
- 单个参数求值失败（结果非 string、数据源异常等）→ 该参数按 `""` 处理 +
  warn 日志（含 logid 与原始配置串），同一动作的其他参数与后续动作继续执行；
- 数据缺失按 Env 约定返回零值（`""`），不是错误；
- 与 `ConditionEngineMode` 解耦：任何模式下 `=` 前缀都按 expr 编译。

### 8.3 消费点与支持的参数位

| 消费点 | 动作 / 参数位 |
| ------ | ------------- |
| `bfe_basic/action`（`mod_rewrite`、`mod_prison`） | `HOST_SET`、`PATH_SET`、`PATH_PREFIX_ADD`、`PATH_PREFIX_TRIM` 的 `Params[0]`；`REQ_HEADER_ADD/SET`、`QUERY_ADD` 的 `Params[1]` |
| `mod_redirect` | `URL_SET`、`URL_PREFIX_ADD` 的 `Params[0]`；`SCHEME_SET`、`URL_FROM_QUERY` 保持字面量 |
| `mod_header` | `REQ/RSP_HEADER_SET/ADD` 的值参数 `Params[1]`（cookie 类动作不在此列） |

补充：

- `mod_rewrite` 的 `allowActions` 只含 host/path/query 类动作，`REQ_HEADER_*`
  由 `mod_header` 承载；
- `bfe_basic/action` 的 `REQ_HEADER_ADD/SET` 仍要求 header 名以 `X-Bfe-`
  开头（与是否使用表达式无关）；
- 语法与示例见各模块配置文档及
  [configuration/00-common.md 第 10 节](../configuration/00-common.md#10-动作参数表达式actionparam)。

### 8.4 与既有 `%变量` 的关系

`mod_header` 原本已有 `%变量` 格式串替换。两者的判定与执行互斥：

- 值参数以 `=` 开头 → 走表达式路径，整串编译，不再做 `%变量` 切分；
- 其他值 → 保持原 `%变量` 路径，行为不变。

`bfe_basic/action`（host/path/query 等）与 `mod_redirect` 现状没有 `%变量`，
第二期也**不新增**：本期目标只是增加 `=` 表达式能力，不与 go-bfe-pro 的
旧机制做能力对齐。

### 8.5 注入机制与依赖环

`bfe_expr/inner_env.go` 依赖 `bfe_basic/action` 的 `VariableHandlers`，因此
`bfe_basic/action` 不能反向 import `bfe_expr`。解决办法是钩子注入：

- `bfe_basic/action` 暴露包级钩子 `SetExprParamCompiler`，入参为参数列表与
  「可作为表达式的下标集合」，返回运行期求值闭包与加载期错误；
- `bfe_expr/action_hook.go` 在 `init()` 中注入实现（内部调用 `CompileParam`
  与 `EvalParams`）；
- 未注入时（例如单独编译/单测 `bfe_basic/action`）表达式参数按字面量处理，
  保持旧行为；
- `mod_redirect`/`mod_header` 是自有 action 类型，直接调用
  `bfe_expr.CompileParam`，不经过钩子。

### 8.6 兼容性

- 存量参数无 `=` 前缀，行为 100% 不变（不进入编译路径）；
- 若存量参数值本身以 `=` 开头，第二期起会被判为表达式并在加载期编译，
  需改写为 `==` 转义；此类配置在实践中极罕见；
- 求值失败容错（置空 + warn 继续）与条件表达式「出错按 false + warn」一致。

## 9. 动态数据源（第二期）

### 9.1 RedisEnv（真实可用，最小实现）

表达式内通过 `GetRedisClient` 获取进程级单例 client，再链式读取：

| 函数 / 方法 | 说明 |
| ----------- | ---- |
| `GetRedisClient(addr)` | 按地址返回单例 client；默认超时 conn 100ms / read 300ms / write 300ms，连接池 MaxIdle 10 / MaxActive 100 |
| `GetRedisClientWithOpts(addr, password, connTimeoutMs, readTimeoutMs, writeTimeoutMs)` | 自定义密码与超时；单例 key 由全部参数组成 |
| `RedisClient.Get(key)` | string 读；miss/错误/类型不符返回 `""` |
| `RedisClient.GetInt(key)` | 整型读（string 存储解析）；miss/错误/解析失败返回 `0` |
| `RedisClient.HGetAll(key)` | Hash 读；miss/错误返回空 map |

约定与边界：

- 单例表在 `bfe_expr` 包级，key = 地址 + 参数组合，同一组合进程内只创建一次
  连接池，后续调用（任意表达式、任意请求）复用；相同 key 传入不同参数时以
  首次创建的实例为准；
- client 不可用（地址非法/创建失败）时返回不可用句柄：方法仍 nil 安全，
  调用返回零值并记 warn，不中断规则求值；
- 本期只提供读操作（`Get`/`GetInt`/`HGetAll`），写操作（`SetEx`/`Incr` 等）
  待有明确需求再开放；
- bfe 的 `redis_client.Client` 无 `Close` 且无 cluster 模式客户端，故本期
  只有 proxy 模式（`NewRedisBnsClient`）单例，不做连接生命周期管理；
- 本期**不新增数据源配置项**：Redis 地址/密码/超时直接写在表达式内。

示例：

```
=GetRedisClient("10.0.0.1:6379").Get("acl:" + ReqCip())
=GetRedisClient("10.0.0.1:6379").HGetAll("app:gray")["uid"]
```

### 9.2 HttpEnv / GlbEnv / ConfEnv（接口 + stub）

- 方法签名沿用 go-bfe-pro 的冻结接口：`HttpGet`/`HttpPost`/`GetHttpClient`/
  `HttpService`、`GlbVarXxx`、`StrTable`/`MapTable`；
- 本期实现为 stub：调用记 warn（未实现）并返回零值，不改变规则执行结果；
- 对应的服务注册与配置热更 API（`RegisterHttpService`/`ApplyExprConf` 等）
  本期不暴露，留待后续接入 `bfe.data` 的 Expr 配置段；
- 之所以保留接口而非直接删除，是为了让配置误用在**运行期**产生明确 warn，
  而不是以「未知函数」在加载期编译失败，避免后续实现时产生语法破坏。

### 9.3 安全与性能

- 表达式可发起同步网络 IO（本期为 Redis 读），会阻塞当前请求的处理 goroutine；
  默认超时 300ms，容量规划需评估上游 QPS 与 Redis 扇出；
- 超时/错误一律降级为零值，不重试，避免故障放大；
- Redis 地址写在表达式内意味着**规则配置的作者需被信任**；后续接入
  `bfe.data` 命名服务后可收拢地址、收窄信任边界；
- `HttpEnv` 若后续放开，需额外评估 SSRF（回调内网地址）与响应体大小上限。
