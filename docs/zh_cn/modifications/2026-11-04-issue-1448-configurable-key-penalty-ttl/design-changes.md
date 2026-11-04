# BFE Issue #1448 变更说明：Key 惩罚时长可配置

- Issue: https://github.com/bfenetworks/bfe/issues/1448
- 一句话变更：把 429 档（60s）与 401/402/403 档（3600s）的 Key 惩罚 TTL 从代码常量改为集群级配置 `KeyPolicy.SessionAffinityPenaltyTTL{429,Auth}`（默认 **60 / 300** 秒）；合成 429 的 `Retry-After` 与写侧同源；批量 404 的 60s 不动。
- 代码库：`bfe/`，分支 **v1.9.0-dev**。§二/§三 行号为设计时 HEAD `cddebd45`（2026-11-03）逐条读码核实。
- 关联：`docs/zh_cn/modifications/2026-10-21-issue-1439-ai-key-penalty-filter-zero-weight/`（惩罚过滤）、`2026-10-21-issue-1440-key-exhausted-no-fallback/`（耗尽降级）、`2026-10-21-issue-1441-key-exhaustion-error-contract/`（合成 429 错误契约）。
- 状态：**已实现（2026-11-04），尚未提交/推送**。实现记录与验证结果见 §八。

## 一、背景

2026-10-22 11:21–15:02，kimi 集群（2 把 Key，其中 1 把 `Weight=0`）连续出现 6 次上游 401。每次 401 都把当次选中的 Key 写入 Redis 惩罚键，**TTL 硬编码 3600 秒**；集群实际只有 1 把可用 Key，首次 401 后候选集即为空，`filterPenaltyKeys` 走「全部被惩罚」熔断（`:2908-2916`），后续请求直接返回网关合成的 429。惩罚状态存在 Redis 而非进程内存，重启 BFE 也无法清除，只能等 TTL 到期或人工删键，整体降级约 2 小时。

惩罚机制本身工作正常（#1439/#1440/#1441 已修完边界）；真正的缺口是**这个时长不可配、不可观测**：

| 缺口 | 表现 |
|------|------|
| 不可配 | 60s / 3600s 是代码常量，测试环境想用 60s 验证「到期自动恢复」都做不到 |
| 事故中不可调 | 只能等过期，或人工登 Redis 删键（`{prefix}:penalty:<cluster>:<key_name>`） |
| 不匹配故障语义 | 「限流（秒级恢复）」与「凭证失效（需人工换 Key）」被同一把尺子度量 |
| 提示与写侧可能矛盾 | 合成 429 的 `Retry-After` 复用 429 常量，写侧一旦可配就会「写 30s、提示 60s」 |

本变更**不改变「要不要惩罚」**（仍由 `SessionAffinityPenaltyEnable` 控制），只把「罚多久」开放为配置。

## 二、现状核对

| 事实 | 位置 | 形态 |
|------|------|------|
| 429 惩罚 TTL | `bfe_server/reverseproxy.go:2354` | 具名常量 `aiKeyPenaltyTTL429`（定义 `:2413-2417`） |
| 401/402/403 惩罚 TTL | `reverseproxy.go:2364` | 字面量 `3600` |
| 批量 404 惩罚 TTL | `reverseproxy.go:2386` | 字面量 `60`（reason `404-batch`，本期不动） |
| 合成 429 的 `Retry-After` / `details.retry_after_seconds` | `reverseproxy.go:2288-2293` | 复用 `aiKeyPenaltyTTL429` |
| 配置结构体 | `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:141-153` | `AIKeyPolicy` 只有 `SessionAffinityPenaltyEnable`，无时长 |
| 加载期默认值 | 同文件 `:1438-1459`（`AIKeyPolicyCheck`，调用点 `:1389-1393`） | 只补 `SessionAffinityTTL` / `RedisPrefix` / `Strategy` |
| nil 策略兜底 | `reverseproxy.go:2733-2745`（`defaultAIKeyPolicy`） | 无惩罚 TTL |
| 写入入口 | `reverseproxy.go:2886-2891`（`setKeyPenalty`） | `ttl` 直接交给 `client.Setex` |
| 生效前提 | 写侧 `:2353`/`:2363`；过滤侧 `:2957-2971` | 需 `SessionAffinity && SessionAffinityPenaltyEnable`（`batchHint` 路径例外） |
| 单可用 Key 短路 | `reverseproxy.go:2965-2967` | `len(keys)==1 && Weight>0` 直接返回，跳过惩罚过滤 |
| 指标 | `bfe_server/proxy_state.go:132`，累加 `reverseproxy.go:3037-3039` | `ReqAiKeyAffinityPenaltySkip` |

两条与实现直接相关的边界：

1. `policy` 是从 `cluster.AIConf.KeyPolicy` **整体替换**而来（`reverseproxy.go:2198-2201`），不是字段级合并。旧控制面下发的对象里新字段为 `0`，若直接透传给 `Setex`，Redis 收到 `expire=0` 会写成**永不过期**的惩罚键——比现状更危险，必须在加载期补默认值。
2. `AIKeyPolicyCheck` **仅在 `KeyPolicy != nil` 时调用**；`KeyPolicy == nil` 的集群只走 `defaultAIKeyPolicy()`。两条路径都要补默认值，漏一处就会出现「有的集群 60/300、有的 0」。

## 三、变更设计

### 3.1 配置结构体

```go
// bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:141-153
type AIKeyPolicy struct {
    Strategy            string
    MaxRetries          int
    RetryBackoffInitial int
    RetryBackoffMax     int

    SessionAffinity              bool
    SessionAffinityTTL           int
    SessionAffinityRedisPrefix   string
    SessionAffinityPenaltyEnable bool

    // SessionAffinityPenaltyTTL* is the penalty TTL (seconds) written when an
    // upstream Key becomes unusable. 0 is filled to the default at load time;
    // it must never reach Redis, where expire=0 means "no expiry".
    SessionAffinityPenaltyTTL429  int // upstream 429 (rate limited), default 60
    SessionAffinityPenaltyTTLAuth int // upstream 401/402/403, default 300
}
```

### 3.2 默认值、校验与上限

`AIKeyPolicyCheck`（`:1438-1459`）追加两个字段的归一化，语义与既有 `SessionAffinityTTL` 保持一致：

```go
const (
    aiKeyPenaltyDefaultTTL429  = 60
    aiKeyPenaltyDefaultTTLAuth = 300
    // guard against a misconfiguration locking a Key for days
    aiKeyPenaltyMaxTTL = 86400
)

func checkPenaltyTTL(name string, ttl *int, def int) error {
    if *ttl < 0 {
        return fmt.Errorf("%s must be >= 0", name)
    }
    if *ttl == 0 {
        *ttl = def
        return nil
    }
    if *ttl > aiKeyPenaltyMaxTTL {
        log.Logger.Warn("AIKeyPolicy: %s[%ds] exceeds max[%ds], clamped", name, *ttl, aiKeyPenaltyMaxTTL)
        *ttl = aiKeyPenaltyMaxTTL
    }
    return nil
}
```

| 输入 | 结果 |
|------|------|
| 字段缺省（旧控制面 / 老配置） | `0` → 60 / 300 |
| 显式 `30` / `120` | 原值保留 |
| 负数 | 加载失败（`KeyPolicy:... must be >= 0`） |
| `> 86400` | 截断为 86400 + `Warn` |

取舍：

- **0 必须补默认**，不能解释成「关闭」——`Setex(...,0)` 是永不过期，与「缺省 = 默认值」的契约也不一致；
- **超限截断而非报错**：控制面已做业务范围校验（10–600 / 60–1800），数据面的 86400 只是防呆；截断可保证手工改配置时加载不失败（加载失败会导致整次 reload 回退，事故中反而有害），`Warn` 留痕；
- 与 `SessionAffinityTTL` 一致：只有负值返回 error。

### 3.3 惩罚写入点与 `Retry-After`

| 位置 | 现状 | 改为 |
|------|------|------|
| `reverseproxy.go:2288` | `retryAfter := aiKeyPenaltyTTL429` | `retryAfter := policy.SessionAffinityPenaltyTTL429` |
| `:2354` | `setKeyPenalty(..., "429", aiKeyPenaltyTTL429, ...)` | `... policy.SessionAffinityPenaltyTTL429 ...` |
| `:2364` | `setKeyPenalty(..., fmt.Sprintf("%d", statusCode), 3600, ...)` | `... policy.SessionAffinityPenaltyTTLAuth ...` |
| `:2386` | `setKeyPenalty(..., "404-batch", 60, ...)` | **不动**（非目标） |
| `:2413-2417` | 常量 `aiKeyPenaltyTTL429` 与注释 | 删除；`Retry-After` 语义就地写在 `:2288` |
| `:2733-2745` | `defaultAIKeyPolicy()` 无惩罚 TTL | 补 `SessionAffinityPenaltyTTL429: 60`、`...Auth: 300` |

**`Retry-After` 语义**（写入 `cluster_conf.data.md`）：表示「按当前配置，被惩罚 Key 的最长剩余等待时间」，不是精确剩余值。精确值需要 `PTTL`，本期不做；至少保证「写侧 30s、提示侧 60s」这类自相矛盾不再出现。

**防御加固（推荐）**：`setKeyPenalty`（`:2886-2891`）增加 `ttl <= 0` 保护（`Warn` 日志 + 不写键）。生产链路必过 `AIKeyPolicyCheck`，但 `aiClusterInvoke` 是包级函数，单测与后续新代码可能直接构造 `AIKeyPolicy`；漏写一次惩罚远好于永久封禁一把 Key。若采纳，`retryAfter` 处同样需兜一个 `<=0 → 60`（或抽 `penaltyTTLOrDefault`）。

### 3.4 可观测性

不新增日志行、不改指标，只在两条既有惩罚日志上追加 TTL：

```go
// :2359
log.Logger.Info("aiClusterInvoke: ai key [name=%s] rate limited (429), rotate, penalty_ttl=%ds",
    key.Name, policy.SessionAffinityPenaltyTTL429)
// :2369
log.Logger.Info("aiClusterInvoke: ai key [name=%s] auth failed (%d), dead, penalty_ttl=%ds",
    key.Name, statusCode, policy.SessionAffinityPenaltyTTLAuth)
```

`ReqAiKeyAffinityPenaltySkip` 口径不变。命中侧剩余 TTL（需给 `redis_client.Client` 增加 `PTTL`）留二期。

## 四、改动文件

| 文件 | 改动 |
|------|------|
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIKeyPolicy` 增 2 字段；新增常量与 `checkPenaltyTTL`；`AIKeyPolicyCheck` 接入 |
| `bfe_server/reverseproxy.go` | `:2288` / `:2354` / `:2364` 取值换 `policy`；`:2359` / `:2369` 日志带 TTL；删除 `:2413-2417` 常量；`defaultAIKeyPolicy()` 补默认；`setKeyPenalty` 加固（推荐） |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go` | `TestAIConfCheck` 增默认值 / 显式值 / 负值 / 截断子测试 + 旧报文反序列化用例 |
| `bfe_server/reverseproxy_ai_test.go` | 更新 `:960/961/977/978` 与两条默认值断言；新增「`Retry-After` 跟随 policy」「`setKeyPenalty` 写入策略 TTL / 拒绝非法 TTL」 |
| `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md` | §9.2 示例 + 字段表 + 亲和性说明；文末整段示例 |
| `docs/en_us/configuration/server_data_conf/cluster_conf.data.md` | 同上英文版 |
| `docs/zh_cn/sys_design/multi_api_key.md` | 结构体片段；`TTL:` 口径；失败处理表 |

`conf/` 无需改动：`conf/server_data_conf/cluster_conf.data` 与 `conf/bfe.conf` 均无 `KeyPolicy` 样例，新字段全部有默认值。

## 五、行为变更与兼容

**可感知行为变更：凭证档默认惩罚 3600s → 300s**（同一故障下惩罚窗口缩短 12 倍），需写进发布说明。

| 组合 | 行为 |
|------|------|
| 新 BFE + 旧控制面（无新字段） | 解析为 0 → 补 60/300；凭证档从 3600 变为 300 |
| 新 BFE + 新控制面（未配置） | 控制面导出 60/300，与上一行一致 |
| 新 BFE + 新控制面（已配置） | 按配置生效；改配置后下一次惩罚写入即生效（无需重启） |
| 旧 BFE + 新控制面 | `bfe_util/json` 的 jsoniter 标准兼容配置忽略未知字段 → 退回 3600/60，用户配置静默失效 |

发布顺序：bfe → ai-gateway-api（控制面字段/导出）→ ai-gateway-web（表单）。反序会让配置在旧数据面上静默失效。

回滚：数据面无状态迁移，回滚即回到常量 60/3600；Redis 中已写入的惩罚键不受影响，仍需等 TTL 到期或 `DEL`。

## 六、验收

| 编号 | 验收项 | 判定 |
|------|--------|------|
| 1 | 不配置任何 TTL | 401 后键 TTL ≈ 300s、429 ≈ 60s |
| 2 | 集群配 `auth=120` | 401 后 TTL ≈ 120s，日志 `penalty_ttl=120` |
| 3 | 集群配 `429=30` | 429 后 TTL ≈ 30s；合成 429 的 `Retry-After: 30`、`details.retry_after_seconds=30` |
| 4 | 改配置不重启 | 修改前后各触发一次 401，TTL 不同 |
| 5 | 旧控制面报文 | 反序列化用例 + 集成用例：无 0 值写入 Redis |
| 6 | `penalty_enable=false` | 不写惩罚键，TTL 配置无效果 |
| 7 | 批量 404 | 键 TTL 仍为 60 |
| 8 | 存量指标 | `ReqAiKeyAffinityPenaltySkip` 口径不变 |

前提：验证集群需 **≥2 把可用 Key（`Weight>0`）**；单可用 Key 集群因 `:2965-2967` 短路，不产生惩罚过滤。

单测命令：

```bash
go test ./bfe_config/bfe_cluster_conf/cluster_conf/... -run 'AIConfCheck|ClusterConf' -v
go test ./bfe_server/... -run 'AIKeyPolicy|Penalty|Synthetic429' -v
go vet ./bfe_config/... ./bfe_server/...
```

## 七、边界与非目标

| 项 | 说明 |
|----|------|
| 批量 404 惩罚（`:2386`） | 属「资源不在该 Key 上」的内部路由修正，与配额/凭证语义不同，60s 固定不变 |
| 命中侧剩余 TTL（需 `PTTL`） | 要给 `redis_client.Client` 加方法并同步 4 处实现/桩，与二期「半开探测」共用能力，二期一起做 |
| 半开探测 / 递增退避 | 涉及选 Key 主流程与新状态机，二期 |
| 按状态码细分（402/403 单独一档） | 三码共用一条分支（`:2360-2369`），再细分会让配置面膨胀，暂不做 |
| 清除某 Key 惩罚的管理动作 | 本期用文档给出键名公式与 `DEL` 命令，管理接口需权限/审计设计，二期 |
| 生效范围 | 只有新写入的惩罚键用新 TTL；已存在的长惩罚仍需等过期或手工删键（Redis 不支持批量重设 TTL） |

## 八、实施记录

**实施日期**：2026-11-04。基线 `v1.9.0-dev` @ `cddebd45`（工作区改动未提交，未推送 fork）。

### 8.1 代码落点

| 文件 | 实施内容 |
|------|----------|
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` | `AIKeyPolicy` 增 `SessionAffinityPenaltyTTL429` / `...Auth`；新增常量 `aiKeyPenaltyDefaultTTL{429,Auth}`、`aiKeyPenaltyMaxTTL` 与 `checkPenaltyTTL`；`AIKeyPolicyCheck` 末尾接入两字段（0→默认、负数报错、>86400 截断 + Warn） |
| `bfe_server/reverseproxy.go` | 合成 429 的 `retryAfter` 改取 `policy.SessionAffinityPenaltyTTL429`（含 `<=0` 防御回退 60）；429 写入点改取 `TTL429`、401/402/403 改取 `TTLAuth`；两条日志追加 `penalty_ttl=%ds`；删除 `aiKeyPenaltyTTL429` 常量；`defaultAIKeyPolicy()` 补 60/300；`setKeyPenalty` 增加 `ttl <= 0` 保护（Warn + 不写键） |
| `bfe_server/reverseproxy_ai_test.go` | `TestDefaultAIKeyPolicy`、`TestDefaultAIKeyPolicyIncludesAffinityDefaults` 补两字段断言；合成 429 用例改为按 `defaultAIKeyPolicy()` 断言；新增 `TestAiClusterInvokeSynthetic429RetryAfterFollowsPolicy`、`TestSetKeyPenaltyTTL`（含非法 TTL 不写键） |
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_test.go` | `TestAIConfCheck` 增 4 个子测试（默认/显式/负值/截断）；新增 `TestAIKeyPolicyCheckLegacyPayload`（旧报文反序列化后补默认） |

### 8.2 文档落点

- `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md`、`docs/en_us/...`：§9.2 示例、字段表（两行）、亲和性说明（惩罚键格式、只影响新写入、与 `SessionAffinityTTL` 无关、批量 404 固定 60s、`Retry-After` 语义、单 Key 短路），以及文末整段示例 JSON；
- `docs/zh_cn/sys_design/multi_api_key.md`：结构体片段、`TTL:` 口径、失败处理表（增批量 404 行、改为字段名）；
- 本变更说明。

### 8.3 集成测试（新增 SC46）

- 设计文档：`tests/integration/测试设计文档/scenario-SC46-Key惩罚时长可配置/`（`场景说明.md` + TC-01…TC-06），并更新 `测试场景总体说明.md`；
- 实现：`tests/integration/implementation/scenario-SC46-key-penalty-ttl/`（含 `testdata/` 与 6 个用例）；
- 公共框架：`tests/integration/common/redis_server.go` 新增 `TTL(key)`（返回剩余 TTL 与是否存在），用于直接断言惩罚键时长。

### 8.4 验证结果

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 编译 | `go build ./...` | 通过 |
| 单测（包级） | `go test ./bfe_server/... ./bfe_config/bfe_cluster_conf/... ./bfe_modules/mod_ai_token_auth/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_rate_limit/...` | 全部通过 |
| 静态检查 | `go vet`（改动包 + 新增集成包）、`gofmt -l` | 通过、无未格式化文件 |
| 新场景 | `go test ./tests/integration/implementation/scenario-SC46-key-penalty-ttl/ -v` | 6 个 TC 全部通过 |
| 回归 | `go test ./tests/integration/implementation/scenario-SC10-ai-key-session-affinity/ ./tests/integration/implementation/scenario-SC02-multi-api-key/` | 全部通过 |

集成用例实测到的惩罚键 TTL：默认 401 → 300s、429 → 60s；显式 `auth=120` → 120s、`429=30` → 30s；热加载 `auth 120 → 60` 后新惩罚 60s（未重启）；字段为 0 → 300s/60s；合成 429 的 `Retry-After` 与 `details.retry_after_seconds` 随 429 档配置（30）。

### 8.5 遗留事项

1. **未提交、未推送 fork**：需要人工确认后提交；控制面方案依赖 fork 上含本改动的提交（伪版本号待推送后回填）。
2. **发现的既有缺陷（不在本期范围）**：`chooseNextAIKey` 用传入切片下标索引 `deadSet`/`usedSet`，而 `filterPenaltyKeys` 返回的是过滤后的子切片，下标会重排。2 把 Key 时，若第一把先返回 401（dead + 惩罚过滤），剩下的健康 Key 可能被误判为 dead，同一请求直接合成 429、健康 Key 未获尝试；429 路径因 `usedSet` 会重置而不受影响。详见 `tests/integration/测试设计文档/scenario-SC46-Key惩罚时长可配置/场景说明.md` §8，建议单独开卡修复。
3. 命中侧剩余 TTL 日志（需 `PTTL` 与 4 处实现/桩）仍为二期。