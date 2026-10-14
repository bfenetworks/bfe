# condition 原语 time_weekday_in：按星期集合匹配

日期：2026-10-14

状态：**代码与单元测试已实现（Phase 4 完成）**，集成测试与回归待做。本文件是本次变更的完整说明（改动点／DSL 规格／兼容性／测试）；
集成测试（Phase 5/6）与回归（Phase 7）完成后，在本文件末尾补写"实施记录"小节（变更文件清单与验证结果）。

## 一、背景与目标

需求：条件原语需支持"当前是否属于某个星期集合"，形如 `time_weekday_in("0|1")`，`0` = 星期天。

现有的两个系统时间原语都表达不了这个语义（`bfe_basic/condition/parser/semant.go:80-81`）：

| 现有原语 | 能力 | 局限 |
| --- | --- | --- |
| `bfe_time_range(start_time, end_time)` | 判断是否落在某个**绝对**时间窗内 | 一次性时间窗，无法表达"每周几" |
| `bfe_periodic_time_range(start_time, end_time, period)` | 判断是否落在某个**日周期**时间窗内 | `period` 非空直接报错（`primitive.go:1056-1058`："periodStr is not supported, should not be set!"），只能按日循环，没有周维度 |

目标：

1. 新增 `time_weekday_in`，参数形态、错误处理、调试手段与现有时间原语保持一致；
2. 参数在**配置加载期**解析与校验（同 `NewTimeMatcher`/`NewPeriodicTimeMatcher` 的做法），匹配期零解析、O(1) 判定；
3. 复用 `X-Bfe-Debug-Time` 调试头，规则行为可离线复现；
4. 纯新增：既有原语的语义与解析路径零改动。

非目标：不实现"按周循环的时间段"原语（用 `&&` 组合表达）；不支持区间简写 `"1-5"`；不改 `bfe_periodic_time_range` 的 `period` 语义。

## 二、DSL 规格

```text
time_weekday_in(weekday_list)
time_weekday_in(weekday_list, zone)
```

| 位置 | 参数 | 类型 | 必填 | 描述 |
| --- | --- | --- | --- | --- |
| 1 | `weekday_list` | String | Y | `\|` 分隔的星期数字列表，元素取值 `0-6`；`0`=周日，`1`=周一，… `6`=周六。允许重复（幂等）、允许元素两侧空白。不可为空串 |
| 2 | `zone` | String | N | 单个军事时区字母，见 `bfe_util.TimeZoneMap`（`bfe_util/time.go:25-51`）：`Z`=UTC、`H`=UTC+8、`M`=UTC+12、`Y`=UTC-12…；大小写不敏感。省略时按 `Z`（UTC）处理 |

判定流程：

```text
PrimitiveCond.Match(req)
  ├─ 前置检查：req != nil && req.Session != nil && req.HttpRequest != nil   // primitive.go:70-73
  ├─ BfeTimeFetcher.Fetch(req)（复用，无改动）
  │    ├─ 有请求头 X-Bfe-Debug-Time → t = bfe_util.ParseTime(该值)（带时区字母，返回 UTC 时刻）
  │    └─ 否则                      → t = time.Now().In(time.UTC)
  └─ WeekdayMatcher.Match(t)
       ├─ t = t.In(loc)                  // loc 由 zone 参数在构建期确定
       ├─ wd = int(t.Weekday())          // 0=周日 .. 6=周六
       └─ return weekdayMask & (1 << wd) != 0
```

语义要点：

- **星期编号与数据面既有约定一致**：`0`=周日、取值 `0-6`，与分层定价的 `cluster_conf.TimeRange.Weekdays` 同源
  （`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:155-162` 的注释 "0=Sunday, 1=Monday, ..., 6=Saturday"，
  运行期同为 `int(t.Weekday())`，见同文件 `1556-1580`）；运营与研发不需要记两套编号。
- **时区必须显式**：星期几完全取决于时区——北京时间 2026-10-19（周一）00:30 时，UTC 仍是 2026-10-18（周日）16:30。
  缺省 `Z`（UTC）与原语族"时间参数一律自带时区字母"的既有习惯一致（`bfe_time_range` 的每个时间串都带 `H`/`Z` 等后缀）；
  国内规则请显式写第二参数 `"H"`。
- **组合表达"按周循环的时间段"**：`time_weekday_in("0|6", "H") && bfe_periodic_time_range("090000H", "180000H", "")`。
- 示例：`time_weekday_in("0|6", "H")`（周末）、`time_weekday_in("1|2|3|4|5", "H")`（工作日）、`!time_weekday_in("1|2|3|4|5", "H")`（非工作日）。

命名说明：`time_` 作为前缀，表示"基于系统时间的原语"，结构为 `<前缀>_<属性>_<动作>`
（同构先例 `req_host_tag_in` = `<前缀>_<主体>_<属性>_<动作>`）。与 `condition_naming_convention.md:16-17`
"系统原语以 `bfe_` 开头"的约定存在差异，处置见 §六 文档更新清单与 §八 风险。

## 三、主要改动点

改动共 3 个源码文件 + 2 个测试文件，**不涉及语法文件与生成代码**（论证见 §3.4）。

### 3.1 `bfe_basic/condition/parser/semant.go`：登记原语原型

`funcProtos`（`:25-87`，紧随 `:81` 的 `bfe_periodic_time_range`）：

```go
	"time_weekday_in":            []Token{STRING, STRING},   // 新增：zone 可省略
```

`optionalLastArgProtos`（`:92-94`）：

```go
var optionalLastArgProtos = map[string]bool{
	"req_ai_intent_in": true,
	"time_weekday_in":  true,   // 新增
}
```

该白名单是本 fork 为实现 `req_ai_intent_in` 尾参可省略而引入的机制（`prototypeCheck` 的放行条件为
`optionalLastArgProtos[name] && argsLen == len(argsType)-1`，见 `:104-109`），本原语沿用同一机制，
因此 `time_weekday_in("0|6")` 与 `time_weekday_in("0|6","H")` 均通过，`time_weekday_in()` 与三参写法被拒。

### 3.2 `bfe_basic/condition/primitive.go`：新增 `WeekdayMatcher`

置于 `PeriodicTimeMatcher` 之后（`:1092` 后）：

```go
// WeekdayMatcher matches whether the weekday of a time value is in a weekday set.
// Weekday numbers follow Go's time.Weekday convention: 0=Sunday, 1=Monday, ...,
// 6=Saturday, the same convention as the cluster_conf TimeRange.Weekdays field.
type WeekdayMatcher struct {
	weekdayMask uint8          // bit i set means weekday i matches
	loc         *time.Location // time zone used to derive the weekday
}

// NewWeekdayMatcher follows weekdayStr format: dd[|dd]*, dd in [0, 6], 0=Sunday.
// zoneStr is one letter defined in bfe_util.TimeZoneMap (e.g. "H" for UTC+8);
// an empty string means "Z" (UTC).
func NewWeekdayMatcher(weekdayStr, zoneStr string) (*WeekdayMatcher, error) {
	if strings.TrimSpace(zoneStr) == "" {
		zoneStr = "Z"
	}
	offset, ok := bfe_util.TimeZoneMap[strings.ToUpper(strings.TrimSpace(zoneStr))]
	if !ok {
		return nil, fmt.Errorf("invalid zone:%s", zoneStr)
	}

	if strings.TrimSpace(weekdayStr) == "" {
		return nil, fmt.Errorf("weekday list is empty")
	}
	var mask uint8
	for _, item := range strings.Split(weekdayStr, "|") {
		wd, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || wd < 0 || wd > 6 {
			return nil, fmt.Errorf("invalid weekday:%s, should be an integer in [0, 6]",
				strings.TrimSpace(item))
		}
		mask |= 1 << uint(wd)
	}

	return &WeekdayMatcher{
		weekdayMask: mask,
		loc:         time.FixedZone("zone", offset),
	}, nil
}

func (m *WeekdayMatcher) Match(v interface{}) bool {
	tm, ok := v.(time.Time)
	if !ok {
		return false
	}
	// tm is in UTC, convert it to the configured time zone
	tm = tm.In(m.loc)
	return m.weekdayMask&(1<<uint(tm.Weekday())) != 0
}
```

设计取舍（与邻居 `PeriodicTimeMatcher` 的两处有意差异）：

- `loc` 在**构建期**用 `time.FixedZone` 建一次并复用，而 `PeriodicTimeMatcher.Match` 每次匹配都新建
  （`primitive.go:1088`）—— 本原语顺带避开每请求一次 `Location` 分配；
- 用 `uint8` 位掩码而非 `map[int]bool`/切片：匹配期零分配、零哈希，重复值天然幂等。

`primitive.go` 所需 import（`strconv`/`strings`/`time`/`bfe_util`）已全部存在（`:18-34`），无需新增 import。
错误文案沿用本文件既有风格（`invalid zone:%s` 见 `bfe_util/time.go:71`）。

### 3.3 `bfe_basic/condition/build.go`：新增 `case`

紧随 `case "bfe_periodic_time_range"`（`:541-551`）之后：

```go
	case "time_weekday_in":
		// optional second arg (STRING): military time zone letter, "Z" (UTC) if omitted
		zone := "Z"
		if len(node.Args) == 2 {
			zone = node.Args[1].Value
		}
		matcher, err := NewWeekdayMatcher(node.Args[0].Value, zone)
		if err != nil {
			return nil, fmt.Errorf("time_weekday_in \"%s\": %s", node.Args[0].Value, err.Error())
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &BfeTimeFetcher{},
			matcher: matcher,
		}, nil
```

- 可选参数写法沿用本 fork 的 `req_ai_intent_in` 先例（`build.go:600-616`：先给缺省值，再按 `len(node.Args)` 覆盖），
  避免 `node.Args[1]` 越界 panic；
- 错误信息带原语名与配置原文，便于在 BFE 日志中定位到具体规则（`req_ai_intent_in` 亦带原语名，`:608`）。

### 3.4 不改动项（明确边界）

- **`parser/cond.y` 与 `parser/y.go` 不改，不需要跑 `make prepare`**。依据：
  1. 语法通用——`cond.y` 只有一条通用调用产生式 `callExpr: IDENT LPAREN paramlist RPAREN`（`:75-83`），
     token 表里没有任何原语名（`:36`）；
  2. 原语名→构造器的映射是手写 Go `switch`（`build.go:85-620`，未命中走 `:618-620` 的 `unsupported primitive %s`）；
  3. 还需同步登记的是原型表 `semant.go:25-87`（在 `parser.Parse` 内由 `primitiveCheck` 执行，缺登记报
     `primitive %s not found`，`:98-101`）——即"改了 `build.go` 忘了 `semant.go`"的表象来源；
  4. 先例：`git show --stat 54ee6b67`（新增 `bfe_time_range`）与 `818b3a6b`（新增 `bfe_periodic_time_range`）
     两笔提交均只改 `build.go`/`semant.go`/`primitive.go`/测试，未动 `.y` 与 `y.go`。
- **`BfeTimeFetcher`（`:1028-1045`）与 `bfe_util.TimeZoneMap` 直接复用**，无改动；`X-Bfe-Debug-Time` 能力由前者自带；
- **不改任何既有原语**：`bfe_time_range` / `bfe_periodic_time_range` 的解析与判定路径零改动；
- **不依赖进程本地时区**：取值链路上所有时刻都显式归一到 `UTC`/`FixedZone(offset)`，宿主机 `TZ` 与夏令时数据库均不影响判定。

## 四、错误处理与边界

| 表达式 | 结果 | 报错时机/信息 |
| --- | --- | --- |
| `time_weekday_in("0\|1")` | 正常（UTC 周日/周一命中） | — |
| `time_weekday_in("0\|6","H")` | 正常（北京时间周日/周六命中） | — |
| `time_weekday_in("0 \| 1")` | 正常（元素空白被容忍） | — |
| `time_weekday_in("0\|0")` | 正常，等价 `time_weekday_in("0")`（去重幂等） | — |
| `time_weekday_in("")` | 构建失败 | 配置加载期：`time_weekday_in "": weekday list is empty` |
| `time_weekday_in("7")` / `("-1")` / `("a")` / `("1-5")` | 构建失败 | 配置加载期：`time_weekday_in "7": invalid weekday:7, should be an integer in [0, 6]` |
| `time_weekday_in("0","QQ")` | 构建失败 | 配置加载期：`time_weekday_in "0": invalid zone:QQ` |
| `time_weekday_in(0)` | 构建失败 | **解析期**（semant）：`primitive time_weekday_in arg 0 expect STRING, got INT` |
| `time_weekday_in()` | 构建失败 | **解析期**（semant）：`primitive args len error, expect 2, got 0` |
| `time_weekday_in("0","H","Z")` | 构建失败 | **解析期**（semant）：`primitive args len error, expect 2, got 3` |
| 旧版本 BFE + 新表达式 | 构建失败 | **解析期**（semant）：`primitive time_weekday_in not found` |

参数个数/类型错误由既有的 `prototypeCheck`（`semant.go:96-122`）在 `parser.Parse` 阶段拦截；取值合法性由
`NewWeekdayMatcher` 在 `condition.Build` 阶段拦截。两者都发生在**配置加载/热加载**时，不在请求路径上。

## 五、配置影响

- **不新增任何配置文件**，也不修改既有配置格式；仅在模块规则的 `Cond` 中使用该原语，例如：

```text
# 路由/AI 路由规则条件
time_weekday_in("0|6", "H") && bfe_periodic_time_range("090000H", "180000H", "")
```

- 监控指标、访问日志字段、导出格式无变化。

## 六、文档更新清单（随代码同提交）

| 文件 | 改动 |
| --- | --- |
| `docs/zh_cn/condition/system/time.md` | 新增 `## time_weekday_in(weekday_list, zone)` 小节（语义、参数表含 `zone` 缺省 `Z` 的显著提示、示例）；§附A 调试头说明处补"`time_weekday_in` 同样受 `X-Bfe-Debug-Time` 影响" |
| `docs/en_us/condition/system/time.md` | 英文镜像 |
| `docs/zh_cn/condition/condition_primitive_index.md:132-135` | `### time` 下新增 `* [time_weekday_in(weekday_list, zone)](./system/time.md#time_weekday_inweekday_list-zone)` |
| `docs/en_us/condition/condition_primitive_index.md` | 英文索引同位置新增 |
| `docs/zh_cn/condition/condition_naming_convention.md:16-17` | "条件原语名称前缀"一节新增 `time_` 前缀条目（定义为"基于系统时间的原语"），并注明 `bfe_time_range`/`bfe_periodic_time_range` 属历史命名、本次不改名 |
| `docs/en_us/condition/condition_naming_convention.md` | 英文镜像同位置新增 |
| `CHANGELOG.md`（`## [Unreleased]` → `### Added`） | `- Support condition primitive time_weekday_in(weekday_list, zone)`（文内说明 zone 缺省为 UTC） |
| `tests/integration/测试设计文档/scenario-SC32-星期条件原语与时区语义/` | 新增 `场景说明.md` + `TC-01`（星期编号语义与集合命中）/`TC-02`（时区语义与缺省 UTC）/`TC-03`（非法参数配置加载失败）；`测试场景总体说明.md` 同步新增 SC32 场景行与 TC 列表 |
| `tests/integration/implementation/scenario-SC32-time-weekday-in/` | 新增 `testdata/`（2 个 Host、6 个 cluster、5 条路由规则）与 `sc32_time_weekday_in_test.go` |
| `tests/integration/common/process_env.go` | 新增 `ProcessEnv.BinaryPath()`（4 行访问器）：TC-03 需以 `bfe -c <conf> -t` 断言配置加载失败并读取退出码/报错文本，而二进制路径在 `common` 内为未导出字段 |

## 七、测试

### 7.1 构造器与匹配器单元测试（`bfe_basic/condition/primitive_test.go`）

风格对齐现有 `TestPeriodicTimeMatcher`（`:306-357`：构造 → 非法输入必须报错 → 用 `time.Date(...)` 直喂 matcher）：

```go
func TestWeekdayMatcher(t *testing.T) {
	// 2019-02-03 is a Sunday, 2019-02-04 is a Monday
	matcher, err := NewWeekdayMatcher("0|6", "H")
	if err != nil { /* t.Fatalf */ }

	tm := time.Date(2019, 2, 3, 23, 59, 59, 0, time.FixedZone("CST", 8*60*60))
	if !matcher.Match(tm) { /* should match: 北京周日 23:59:59 */ }

	// UTC 2019-02-03 16:00:00 == 北京 2019-02-04(周一) 00:00:00 -> 不再命中
	tm = time.Date(2019, 2, 3, 16, 0, 0, 0, time.UTC)
	if matcher.Match(tm) { /* should not match */ }

	for _, bad := range []string{"", "7", "-1", "a", "1-5"} {
		if _, err := NewWeekdayMatcher(bad, "H"); err == nil { /* should fail */ }
	}
	if _, err := NewWeekdayMatcher("0", "QQ"); err == nil { /* should fail */ }
}
```

断言要点：**时区边界**用同一时刻的两种表示互相印证（`time.FixedZone("CST", 8*60*60)` vs `time.UTC`），
这是本原语最容易出错的地方。

### 7.2 构建测试（`bfe_basic/condition/build_test.go`）

`buildPrimitiveTests`（`:60-339`，表驱动 `{name, cond, buildCond, buildErr}`）新增：

| 用例 `cond` | 期望 |
| --- | --- |
| `time_weekday_in("0\|6")` | build 成功 |
| `time_weekday_in("0\|6","H")` | build 成功 |
| `time_weekday_in("0\|6", "H") && bfe_periodic_time_range("090000H","180000H","")` | build 成功（组合） |
| `time_weekday_in("7")` / `("")` / `("0","QQ")` | build 失败 |
| `time_weekday_in(0)` / `("0","H","Z")` | build 失败（semant 类型/个数） |

并借 `req.HttpRequest.Header["X-Bfe-Debug-Time"]`（包级 `req` 见 `build_test.go:31-36`）验证 `Match`：
`20190203200000H`（北京周日 20:00）对 `time_weekday_in("0","H")` 命中、对 `time_weekday_in("1","H")` 不命中。

### 7.3 联调验证

```bash
# 含该原语的规则 + 调试头复现（周一 20:30 北京时间）
curl -H 'X-Bfe-Debug-Time: 20190204203000H' http://<bfe-vip>/<命中该规则的path>
# 期望：time_weekday_in("1","H") 命中；time_weekday_in("0|6","H") 不命中
```

### 7.4 验证命令

```bash
cd bfe && make test        # go test -cover ./... + go vet ./...
```

### 7.5 集成测试（Scenario SC32 星期条件原语与时区语义）

以路由决策为观测量的端到端验证（真实 `bfe` 进程 + mock 后端），单列独立场景（并入 SC01 会与既有断言混杂）：

- 设计文档：`tests/integration/测试设计文档/scenario-SC32-星期条件原语与时区语义/`（`场景说明.md` + `TC-01`/`TC-02`/`TC-03`）；
- 实现：`tests/integration/implementation/scenario-SC32-time-weekday-in/`；
- 覆盖：TC-01 一周七天遍历（锁定 `0`=周日、`6`=周六 编号语义）与真实时钟臂；TC-02 北京时间 08:00 偏移边界的「分歧/收敛」（zone 参与判定且缺省 UTC）与未命中回落；TC-03 非法参数 fail-fast 与错误可定位；
- 运行：`go test ./tests/integration/implementation/scenario-SC32-time-weekday-in/... -v`

## 八、兼容性、上线顺序与风险

**兼容性**

- 纯新增原语：既有表达式的词法/语法/构建路径完全不变，未使用该原语的配置行为零变化；
- 反向兼容（新配置 + 旧 BFE）：解析期报 `primitive time_weekday_in not found`，导致该次配置加载/热加载失败。
  这是**必要的失败**（fail-fast，优于静默不匹配），但要求上线顺序正确。

**上线顺序（必须遵守）**

```text
① BFE（数据面）升级到含 time_weekday_in 的镜像并全量
        ↓
② ai-gateway-api 升级 bfe 依赖（go.mod:83 的 replace 指向含该原语的提交）并部署
        ↓
③ ai-gateway-web 部署（表达式面板出现按钮）
```

原因：控制面保存规则时会**真实编译** BFE 表达式（`ai-gateway-api/lib/validate/validate.go:571`、
`model/iroute_conf/route_rule.go:227` 均调 `condition.Build`），依赖不升级则用户在保存/校验阶段即被拒
（`invalid route rule Cond: primitive time_weekday_in not found`），规则根本到不了 BFE；反之若先放开控制面
而 BFE 未升级，则会出现"配置保存成功但 BFE 热加载失败"的运维事件。

**回滚（逆序）**：`web → api →（先清理/回滚引用该原语的规则配置）→ bfe`。回退 BFE 镜像前必须先确认
存量配置不再引用 `time_weekday_in`，否则热加载失败。

| # | 风险 | 影响 | 缓解 |
| --- | --- | --- | --- |
| R1 | 漏写 `zone`，按 UTC 判定 | 北京时间 00:00–08:00 内按前一天判定，**静默错误、无报错** | 文档与前端面板示例统一两参写法；上线后抽样比对 `X-Bfe-Debug-Time` 复现结果 |
| R2 | 控制面未同步升级依赖 | 用户保存规则被拒绝 | 严格执行上述上线顺序；控制面补 `ConditionExpression` 单测 |
| R3 | 前端面板原语列表是静态副本 | 面板与 BFE 实际支持漂移（现存 `bfe_cluster_in` 在本 fork 的 BFE 中并无对应原语） | 变更时同步 `ExpressionConfig.js` |
| R4 | 运营误用区间写法 `"1-5"` | 配置加载报错（fail-fast） | 错误信息回显非法元素，见 §四 |

## 九、实施记录（2026-10-14）

已按本方案实施，代码与文档变更见 §六；实现与设计**无偏差**（`WeekdayMatcher`、`build.go` 的 `case`、`semant.go` 的原型登记、文档与集成场景均按 §三/§六/§7.5 落地）。

**验证结果**

| 项 | 命令 | 结果 |
| --- | --- | --- |
| 条件原语包单元测试 | `go test ./bfe_basic/...` | 通过 |
| 新增用例（-v） | `go test ./bfe_basic/condition/ -run 'TestWeekdayMatcher\|TestBuildTimeWeekdayIn' -v` | 两者均 PASS |
| 全量单元测试（非集成） | `go test -cover $(go list ./... \| grep -v /tests/integration)` | exit 0，94 个包全过、0 失败 |
| 静态检查 | `go vet ./...` | 通过 |
| 真实配置加载（正例） | 临时 conf 副本上 `bfe -c <conf> -t`（规则含省略 zone 的写法与原语组合） | `configuration file .../bfe.conf test is successful` |
| 真实配置加载（反例） | 同上，规则改为 `time_weekday_in("7", "H")` | 加载失败，报错定位到规则原文与非法值（fail-fast 符合 §四） |
| 集成场景 SC32 | `go test ./tests/integration/implementation/scenario-SC32-time-weekday-in/... -v` | TC-01/TC-02/TC-03 全 PASS（12.9s） |
| 全量集成回归 | `go test -cover -p 1 -timeout 5m ./tests/integration/...` | SC01–SC32 中 31 个场景通过（含 SC32）；**仅 SC13 失败，属既有问题**（见下） |

**关于 SC13（既有失败，非本次引入）**

`scenario-SC13-tls-conf-reload-path` 的 TC-01/TC-02 在 **Windows** 上失败：

- TC-01：`sc13_tls_conf_reload_path_test.go:173: decode reload rsp failed: invalid character 'U' in string escape code`（reload 响应含未转义的 Windows 路径反斜杠，构成非法 JSON 转义）
- TC-02：`sc13_tls_conf_reload_path_test.go:354: reload with path should fail: example_ca.crt missing in version dir`
- 同场景 TC-03 通过

已用 `git archive HEAD` 导出**不含本次改动的干净树**复现出完全相同的两个失败（同行号、同报错）；且本次改动未触及 `bfe_tls/`、`bfe_config/`、`bfe_server/` 任何文件。故判定为既有平台相关缺陷，不在本变更范围内处理。

**遗留事项**

- 控制面联动未做：`ai-gateway-api` 的 bfe 依赖（`go.mod` 的 `replace`）仍需升级到含本原语的提交，`ai-gateway-web` 表达式面板需新增按钮；上线顺序见 §八。
- 可选文档同步：`tests/integration/README.md` 的"当前覆盖"表未加 SC32；`测试场景总体说明.md` 缺 SC31 行（既有遗漏）。