# 动作参数表达式 + 动态数据源（第二期）

日期：2026-10-31
前置依赖：[2026-10-30 expr 条件表达式引擎（第一期）](../2026-10-30-expr-condition-engine/design-changes.md)
系统设计详见 [sys_design/expr_condition_engine.md](../../sys_design/expr_condition_engine.md)

## 背景与目标

第一期已完成条件表达式引擎（`bfe_expr/`、`condition.Build` 三模式分派、
旧原语兼容层），并在 `Env` 中预留了 `Param` 与扩展 Env 的接入点。第二期补齐
两块拼图：

- **动作参数内嵌表达式**：`mod_rewrite`/`mod_redirect`/`mod_header` 等模块的
  动作参数（header 值、目标 URL、host/path/query 值等）支持以 `=` 前缀内嵌
  expr 表达式，加载期预编译 + 强类型检查，运行期经请求级 `Env` 求值。
  变量来源 = 第一期 action 变量（`InnerEnv`/`VarEnv`）+ 本期动态数据源
  （`RedisEnv`）。
- **动态数据源**：`Env` 新增 `RedisEnv`（真实可用：GET/Hash 读）与
  `HttpEnv`/`GlbEnv`/`ConfEnv`（接口 + stub，调用返回明确错误），使条件
  表达式与动作参数表达式都能读取进程外数据。

设计原型为 go-bfe-pro 仓库 `bfe_expr/param.go`、`action_hook.go`、
`redis_env.go` 等，按 bfe 实际代码结构适配（bfe 与 go-bfe-pro 的 action
体系差异较大，见「消费点」）。

**本期边界（Phase 0 已确认）**：

- **不拉齐 go-bfe-pro 的旧机制**：本期只给 bfe 的动作参数**新增 expr 能力**，
  不把 go-bfe-pro 已有的 `%变量` 替换机制补到 bfe 尚未支持的位置（见下节
  「与 go-bfe-pro 的差异」）。目的不是能力对齐，而是增加新能力。
- 不做数据源配置项：Redis 地址/超时/TTL 等**不进** `bfe.conf`，也不新增
  `bfe.data` Expr 配置段；Redis 地址以字面量（或变量）直接写在表达式内。
- `HttpEnv`/`GlbEnv`/`ConfEnv` 仅接口 + stub；命名服务注册
  （`RegisterRedisService`/`ApplyExprConf` 热更）不实现。
- 验收 = 编译 + 单测 + 文档；不做集成测试场景。

### 与 go-bfe-pro 的差异：不拉齐 `%变量`

go-bfe-pro 在动作参数上已有一套 `%变量` 替换机制（全局 `EvalParams` +
格式串切分）；bfe 现状是**部分位置有、部分位置没有**：

| 消费点 | bfe 现状 `%变量` | 本期处理 |
|--------|------------------|----------|
| `bfe_basic/action`（`REQ_HEADER_ADD/SET`、`HOST_SET`、`PATH_SET` 等，用于 `mod_rewrite`/`mod_prison`） | **无**，参数按字面量使用 | **只新增 `=expr`**，不补 `%变量` |
| `mod_redirect`（`URL_SET`/`URL_PREFIX_ADD` 等） | **无**，参数按字面量使用 | **只新增 `=expr`**，不补 `%变量` |
| `mod_header`（header/cookie 值） | **有** `%var` 格式串（`VariableHandlers`） | 保留既有 `%var`，另加互斥的 `=expr` 整体表达式 |

即：**有 `%var` 的地方原样保留，没有 `%var` 的地方不新增**；`=` 前缀在三处
语义一致（整串表达式）。这样本期改动可视为纯增量语法，不与既有机制交织。

注：go-bfe-pro 的 `USR_VARIABLE_SET` 在 bfe 仓库中并不存在（仅有相关 helper
定义），本期不新增该动作，也不新增 `%var` 机制。

## 总体设计

### 动作参数表达式语法

沿用 go-bfe-pro 的三态约定（`docs/design/动作参数表达式.md`）：

| 参数形态 | 语义 |
|----------|------|
| `=expr` | expr 表达式，编译期 `expr.AsKind(reflect.String)` 强类型检查（返回值必须 string），**加载期编译失败即配置报错，不回退字面量** |
| `==lit` | 转义字面量：首个 `=` 转义，`"==foo"` → 字面量 `"=foo"`；`"=="` → `"="` |
| 其他 | 普通字符串，原样使用（零开销，不经过表达式编译） |

运行期约定：

- 加载期对每个可表达式参数预编译为 `*Param`（`prog *vm.Program`），运行期
  `Eval(req)` 经 `bfe_expr.GetEnv(req)` 取请求级 Env 求值；
- 单个参数求值失败 → 置空 `""` + warn 日志（带 logid 与原始配置串），
  **继续执行**其余动作，不中断请求处理；
- 数据缺失按 Env 约定返回零值（`""`），不视为错误。

与条件引擎三模式（`ConditionEngineMode`）**解耦**：动作参数表达式是独立的
编译路径，任何模式下只要参数以 `=` 开头即按表达式编译。

### 消费点（按 bfe 实际结构）

bfe 的 action 体系与 go-bfe-pro 不同，需逐点接入：

| 消费点 | 现状 | 本期改动 |
|--------|------|----------|
| `bfe_basic/action`（`Action.Do`，用于 `mod_rewrite`、`mod_prison`） | 参数原样使用；**无 `%var`** | 只增加 `=expr`：表达式参数编译（加载期，`UnmarshalJSON` 内）与求值（`Do` 内）；可表达式参数下标表：`REQ_HEADER_ADD/SET{1}`、`HOST_SET{0}`、`PATH_SET{0}`、`PATH_PREFIX_ADD/TRIM{0}`、`QUERY_ADD{1}`。**不新增 `%var`** |
| `mod_redirect`（自有 `Action` 类型） | `URL_SET`/`URL_PREFIX_ADD`/`SCHEME_SET`/`URL_FROM_QUERY` 参数原样使用；**无 `%var`** | 只增加 `=expr`：`URL_SET`/`URL_PREFIX_ADD` 的 `Params[0]`（目标 URL）支持表达式；`SCHEME_SET`（有 http/https 字面量校验）与 `URL_FROM_QUERY`（query key）保持字面量。**不新增 `%var`** |
| `mod_header`（自有 action 实现） | header/cookie 值已有 `%var` 格式串（`preProcessParams`/`getHeaderValue`，`VariableHandlers`） | 保留 `%var`；`REQ/RSP_HEADER_SET/ADD` 的值参数（`Params[1]`）整体以 `=` 开头时按表达式编译求值，与 `%var` 路径互斥；`=`/`==` 判定规则同上 |

**注入方式（避免依赖环）**：`bfe_basic/action` 不能反向依赖 `bfe_expr`
（`bfe_expr/inner_env.go` 依赖 `action.VariableHandlers`），故由
`bfe_expr/action_hook.go` 在 `init()` 中调用 `action.SetExprParamCompiler(...)`
注入编译器；未注入时（如 `bfe_basic/action` 包内单测）表达式参数按字面量处理。
`mod_redirect`/`mod_header` 为自有类型，直接调用 `bfe_expr.CompileParam`
（不经过注入钩子）。

### Env 扩展：动态数据源

`Env` 组合结构新增四个子 env：

```go
type Env struct {
    BaseEnv
    TimeEnv
    HashEnv
    VarEnv
    InnerEnv
    LegacyEnv
    RedisEnv  // GetRedisClient / GetRedisClientWithOpts，见 redis_env.go
    HttpEnv   // stub：本期未实现，调用返回零值 + warn
    GlbEnv    // stub：本期未实现，调用返回零值 + warn
    ConfEnv   // stub：本期未实现，调用返回零值 + warn
}
```

#### RedisEnv（真实可用，最小实现）

- `GetRedisClient(addr string) *RedisClient`：proxy 模式（`bfe_util/redis_client`
  BNS 客户端）单例连接池，默认超时 conn 100ms / read 300ms / write 300ms，
  MaxIdle 10 / MaxActive 100；同一 addr+参数组合进程级只建一次；
- `GetRedisClientWithOpts(addr, password string, connTimeoutMs, readTimeoutMs, writeTimeoutMs int)`
  支持自定义超时/密码；
- `RedisClient` 句柄（nil 安全，错误/miss/类型不符返回零值）：
  - `Get(key) string` — string 读；
  - `GetInt(key) int` — 整型读（string 存储解析）；
  - `HGetAll(key) map[string]string` — Hash 读（miss 返回空 map）；
  - 写操作（SetEx/Incr 等）本期不提供，待有明确需求再开放；
- 表达式示例：
  `=GetRedisClient("10.0.0.1:6379").Get("acl:" + ReqClientIp())`、
  `=GetRedisClient("10.0.0.1:6379").HGetAll("app:gray")["uid"]`。
- bfe 的 `redis_client.Client` 接口无 `Close`，且无 cluster 模式客户端，故
  本期只有 proxy 模式单例，不做服务生命周期管理。

#### HttpEnv / GlbEnv / ConfEnv（接口 + stub）

- 三个子 env 的方法签名按 go-bfe-pro 冻结（`HttpGet`/`HttpPost`/`GetHttpClient`/
  `HttpService`、`GlbVarXxx`、`StrTable`/`MapTable`），编译期可见、接口面稳定；
- 本期实现为 stub：调用记 warn（"not implemented in phase 2"）并返回零值，
  不改变规则执行；Go 层的 `RegisterXxxService`/`ApplyExprConf` API 不暴露；
- 这样配置里若误用会在运行期明确暴露（warn 日志），而不是以未知函数编译失败。

### 安全说明

- 表达式内可发起同步网络 IO（本期为 Redis 读），会阻塞请求处理：默认超时
  300ms，请评估后端 Redis 容量与扇出；
- 动作参数表达式与条件表达式一样可被规则配置使用，Redis 地址写在表达式内
  意味着规则作者需可信任；后续接 `bfe.data` 配置段时可收拢为命名服务。

## 主要改动点

### 新增：`bfe_expr/param.go`

`Param`（`=`/`==`/字面量三态）、`CompileParam`/`CompileParams`（加载期，
`expr.AsKind(reflect.String)`）、`EvalParams`（运行期，失败置空 + warn）。
移植自 go-bfe-pro，日志改用 bfe 的 `bfe_util/log`。

### 新增：`bfe_expr/action_hook.go`

`init()` 注入 `action.SetExprParamCompiler`；编译器按 `=` 前缀批量编译指定
下标参数，返回运行期求值闭包。

### 新增：`bfe_expr/redis_env.go` + `bfe_expr/redis_client.go`

`RedisEnv`（`GetRedisClient`/`GetRedisClientWithOpts` + 单例表）、
`RedisClient` 句柄（`Get`/`GetInt`/`HGetAll`，nil 安全零值返回）。
底层用 `bfe_util/redis_client.NewRedisBnsClient`。

### 新增：`bfe_expr/http_env.go` / `glb_env.go` / `conf_env.go`（stub）

仅含 Env 方法与零值实现 + warn 日志；不含 client 实现与注册 API。

### 修改：`bfe_expr/env.go`

`Env` 嵌入 `RedisEnv`/`HttpEnv`/`GlbEnv`/`ConfEnv`；`NewEnv` 初始化
`RedisEnv{req: req}`。

### 修改：`bfe_basic/action/action.go`

- 新增 `exprParamCompiler` 包级变量 + `SetExprParamCompiler`；
  `exprParamIndexes` 表；`UnmarshalJSON` 末尾编译表达式参数（失败返回错误，
  配置加载失败）；`Do` 开头对命中 cmd 取编译闭包求值后的参数。
- bfe 仓库不存在 `USR_VARIABLE_SET` 动作（go-bfe-pro 专有），本期跳过该
  消费点，不新增 `%var` 机制。

### 修改：`mod_redirect` / `mod_header`

- `mod_redirect`：`URL_SET`/`URL_PREFIX_ADD` 的 `Params[0]` 加载期
  `CompileParam`，`redirectExclusiveActionDo` 运行期 `Eval`；
- `mod_header`：值参数整体 `=` 前缀时加载期 `CompileParam`，
  `getHeaderValue` 运行期 `Eval`，与 `%var` 格式串互斥。

### 测试（单测，本期验收标准之一）

- `bfe_expr/param_test.go`：三态编译、`==` 转义、类型检查失败、Eval 错误传播；
- `bfe_expr/action_hook_test.go`：注入编译、下标过滤、求值失败置空继续；
- `bfe_expr/redis_env_test.go`：`miniredis`（已是 go.mod 依赖）验证
  Get/GetInt/HGetAll、miss/错误零值、单例复用；
- `bfe_basic/action`：表达式参数（编译器注入契约）用例；
- `mod_redirect`/`mod_header`：`=expr` 参数用例；
- 回归：`make test`（全量单测 + vet）。

## 配置影响

- **无新增配置字段**：`bfe.conf`、`bfe.data` 均不变。
- `mod_rewrite`/`mod_redirect`/`mod_header` 的规则文件中动作参数语法扩展：
  支持 `=` 前缀表达式与 `==` 转义。语法统一定义在
  `docs/{zh_cn,en_us}/configuration/00-common.md` 新增的「动作参数表达式
  （ActionParam）」一节，各模块配置文档列出本模块支持表达式的动作与参数位，
  并补充示例。
- 表达式可用函数清单补充 `GetRedisClient*`（在 Phase 3 一并写入
  `docs/zh_cn/sys_design/expr_condition_engine.md`）。

## 兼容性说明

- **纯增量语法**：存量参数无 `=` 前缀，行为 100% 不变（不经过表达式编译）；
- **唯一注意点**：存量配置中若存在本身以 `=` 开头的参数值（如
  `QUERY_ADD` 的 value 为 `=foo`），本期起会被判为表达式并在加载期编译，
  需改写为 `==foo` 转义。此类配置在实践中极罕见，迁移时留意即可；
- 求值失败的容错语义（置空 + warn 继续）与第一期条件表达式「出错按 false +
  warn」的哲学一致；
- `mod_header` 的既有 `%var` 格式串路径完全保留，仅新增互斥的 `=` 整体
  表达式分支；
- **不新增 `%var` 能力**：`bfe_basic/action`（`mod_rewrite`/`mod_prison`）与
  `mod_redirect` 本期仍不支持 `%var`（与改造前一致），不承担与 go-bfe-pro
  的能力对齐；
- 未注入编译器时（`bfe_basic/action` 被单独使用/单测）表达式参数按字面量
  处理，与注入前行为一致。

## 后续范围预告

- `bfe.data` Expr 配置段（命名 Redis/Http 服务、StringTable/MapTable）与
  `ApplyExprConf` 热更（对应 `ConfEnv` 与命名服务注册）；
- `HttpEnv`/`GlbEnv` 真实实现（SSRF 防护、超时与并发评估）；
- Redis 写操作（SetEx/Incr）按需开放。

## 修订记录

### 2026-10-31：初稿

第二期范围确立：动作参数内嵌表达式（`=`/`==` 三态语法、`bfe_basic/action`/
`mod_redirect`/`mod_header` 三个消费点、注入式编译器避免依赖环）+ 动态数据源
（RedisEnv 最小实现 Get/GetInt/HGetAll，HttpEnv/GlbEnv/ConfEnv stub）；无新增
配置项；验收 = 编译 + 单测 + 文档。
