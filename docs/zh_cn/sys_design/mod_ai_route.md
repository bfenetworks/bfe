# mod_ai_route 系统设计文档

## 1. 背景与目标

### 1.1 背景

BFE 原有路由基于租户（Product）组织，每张路由表命中后返回单个 `ClusterName`，再由负载均衡模块选择后端实例。AI 网关场景下，路由需求发生显著变化：

- 路由表不再按租户划分，而按 **apikey → entity → global** 三级优先级组织；
- 命中后返回的是 **targets 列表**（含集群、模型、权重）和 **fallbacks 列表**；
- 需要在多个 target 之间做加权选择，并在 target 转发失败时按 fallbacks 顺序降级。

### 1.2 目标

新增 `mod_ai_route` 模块，在 BFE 的 `HandleFoundProduct` 回调点完成 AI 网关路由查找，并将结果写入请求上下文，供后续转发流程使用。

主要目标：

1. 实现 AI 路由三级查找：API-Key 路由表 → Entity 路由表 → Global 路由表；
2. 支持命中规则后返回 `targets` 和 `fallbacks`；
3. 与现有 `mod_ai_token_auth`、`mod_ai_rate_limit` 等模块协同，复用 `AiBasicInfo` 上下文；
4. 保持对原 BFE 逻辑的侵入最小化；
5. 支持配置热加载与监控。

## 2. 术语定义

| 术语 | 定义 |
|------|------|
| API-Key | 调用方身份标识，路由匹配的最细粒度维度。 |
| Entity | 业务实体，如部门、应用、项目，一个 Entity 下可包含多个 API-Key。 |
| Global | 全局路由表，所有请求的最后兜底。 |
| Target | 路由命中的转发目标，包含 `ClusterName`、`Model`、`Weight`。 |
| Fallback | 当所有 target 不可用时，按顺序尝试的备用目标。 |
| Route Table Key | 路由表在配置文件中的唯一标识，格式为 `<type>_<owner>`，例如 `apikey_ak_user_a`。 |

## 3. 总体架构

### 3.1 在 BFE 中的位置

```
┌─────────────────────────────────────┐
│           HTTP 请求接入              │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  HandleBeforeLocation               │
│  (mod_trust_clientip, mod_logid 等) │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  findProduct()                      │
│  (BFE 原租户识别，AI 网关仍保留)      │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  HandleFoundProduct                 │
│  mod_ai_token_auth                  │
│  mod_ai_rate_limit                  │
│  mod_ai_route  ← 新增               │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  AI 网关转发路径（ ServeHTTPForAI）  │
│  - target 加权选择                   │
│  - model 覆盖/透传                   │
│  - fallback 顺序降级                 │
│  - clusterInvoke() 复用现有集群转发   │
└─────────────────────────────────────┘
```

### 3.2 模块协作关系

```
                    ┌─────────────────┐
                    │   HTTP Request  │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
    ┌─────────────────┐ ┌──────────┐ ┌──────────────┐
    │ mod_ai_token_auth│ │mod_ai_rate_limit│ │  mod_ai_route │
    │ (API-Key 鉴权)   │ │(限流)    │ │  (路由查找)   │
    └────────┬────────┘ └────┬─────┘ └──────┬───────┘
             │               │              │
             ▼               ▼              ▼
    ┌─────────────────────────────────────────────┐
    │  AiBasicInfo / Request.Context              │
    │  - ClientApiKey                             │
    │  - ClientModel / TargetModel                │
    │  - AiRouteResult (新增)                     │
    └─────────────────────────────────────────────┘
                             │
                             ▼
                 ┌───────────────────────┐
                 │ ReverseProxy.ServeHTTPForAI │
                 └───────────────────────┘
```

## 4. 数据结构与配置设计

> 配置文件字段及使用说明，请参考：
> - [mod_ai_route.conf](../configuration/mod_ai_route/mod_ai_route.conf.md)
> - [ai_route.data](../configuration/mod_ai_route/ai_route.data.md)

### 4.1 配置文件

#### 4.1.1 模块配置文件

路径：`conf/mod_ai_route/mod_ai_route.conf`

```ini
[basic]
RouteRulePath = ../conf/mod_ai_route/ai_route.data

[log]
OpenDebug = false
```

字段说明：

| 字段 | 类型 | 说明 |
|------|------|------|
| `RouteRulePath` | string | AI 路由规则数据文件路径。 |
| `OpenDebug` | bool | 是否开启调试日志。 |

#### 4.1.2 路由规则数据文件

路径：`conf/mod_ai_route/ai_route.data`

格式示例：

```json
{
    "Version": "20260718131505",
    "route_rules": {
        "apikey_ak_user_a": {
            "type": "apikey",
            "owner": "ak_user_a",
            "rules": [
                {
                    "name": "user_a-deepseek",
                    "Cond": "req_host_in(\"api.example.org\")",
                    "targets": [
                        {
                            "ClusterName": "cluster_deepseek_a",
                            "Model": "deepseek-v4-pro",
                            "Weight": 70
                        },
                        {
                            "ClusterName": "cluster_deepseek_b",
                            "Model": "deepseek-v4-pro",
                            "Weight": 30
                        }
                    ],
                    "fallbacks": [
                        {
                            "ClusterName": "cluster_deepseek_c",
                            "Model": "deepseek-v3.2"
                        }
                    ]
                }
            ]
        },
        "entity_dept_ai": {
            "type": "entity",
            "owner": "dept_ai",
            "rules": [
                {
                    "name": "dept_ai-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_dept_ai",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        },
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": [
                {
                    "name": "global-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_global",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        }
    },
    "ApikeyRouteTableBindings": {
        "ak_user_a": [
            "apikey_ak_user_a",
            "entity_dept_ai",
            "global_default"
        ]
    }
}
```

字段说明：

| 字段 | 类型 | 说明 |
|------|------|------|
| `route_rules` | object | 所有路由表的集合，key 为 `<type>_<owner>`。 |
| `type` | string | 路由表类型：`apikey` / `entity` / `global`。 |
| `owner` | string | 路由表属主。 |
| `rules` | array | 该路由表下的规则列表，按顺序匹配。 |
| `name` | string | 规则名称，用于日志和监控。 |
| `Cond` | string | BFE 条件表达式，命中则使用该规则。 |
| `targets` | array | 转发目标列表。 |
| `fallbacks` | array | 降级目标列表，**允许为空**。 |
| `ClusterName` | string | 后端集群名称。 |
| `Model` | string | 模型名称，空字符串表示透传原始模型。 |
| `Weight` | int | 权重，单个 target 时为 100，多个 target 时总和为 100。 |
| `ApikeyRouteTableBindings` | object | API-Key 到路由表查找顺序的映射。 |

### 4.2 内部数据结构

#### 4.2.1 配置结构体（bfe_modules/mod_ai_route/conf_load.go）

```go
package mod_ai_route

import (
    "fmt"
    gcfg "gopkg.in/gcfg.v1"
    "github.com/bfenetworks/bfe/bfe_util"
)

type ConfModAiRoute struct {
    Basic struct {
        RouteRulePath string // path for ai route rule
    }
    Log struct {
        OpenDebug bool
    }
}

func ConfLoad(filePath string, confRoot string) (*ConfModAiRoute, error) {
    var cfg ConfModAiRoute
    if err := gcfg.ReadFileInto(&cfg, filePath); err != nil {
        return &cfg, err
    }
    if err := cfg.Check(confRoot); err != nil {
        return &cfg, err
    }
    return &cfg, nil
}

func (cfg *ConfModAiRoute) Check(confRoot string) error {
    return ConfModAiRouteCheck(cfg, confRoot)
}

func ConfModAiRouteCheck(cfg *ConfModAiRoute, confRoot string) error {
    if cfg.Basic.RouteRulePath == "" {
        return fmt.Errorf("ConfModAiRouteCheck: RouteRulePath is empty")
    }
    cfg.Basic.RouteRulePath = bfe_util.ConfPathProc(cfg.Basic.RouteRulePath, confRoot)
    return nil
}
```

#### 4.2.2 路由规则与数据定义（bfe_modules/mod_ai_route/route_rule.go）

`route_rule.go` 同时定义 JSON DTO、运行时结构以及兼容性反序列化逻辑。DTO 与运行时类型分离，便于兼容不同来源的数据格式（如 `RouteRules` 与 `route_rules`）。

```go
package mod_ai_route

import (
    "encoding/json"
    "fmt"

    "github.com/bfenetworks/bfe/bfe_basic"
    "github.com/bfenetworks/bfe/bfe_basic/condition"
)

// route table types
const (
    RouteTypeApikey = "apikey"
    RouteTypeEntity = "entity"
    RouteTypeGlobal = "global"
)

// RouteRuleFile 是单条路由规则的 JSON DTO。
type RouteRuleFile struct {
    Name      string                      `json:"name"`
    Cond      string                      `json:"Cond"`
    Targets   []bfe_basic.AiRouteTarget   `json:"targets"`
    Fallbacks []bfe_basic.AiRouteFallback `json:"fallbacks"`
}

// RouteTableFile 是单张路由表的 JSON DTO。
type RouteTableFile struct {
    Type  string          `json:"type"`
    Owner string          `json:"owner"`
    Rules []RouteRuleFile `json:"rules"`
}

// AiRouteDataFile 是整个 AI 路由数据文件的 JSON DTO。
type AiRouteDataFile struct {
    Version                  string                    `json:"Version"`
    RouteRules               map[string]RouteTableFile `json:"route_rules"`
    ApikeyRouteTableBindings map[string][]string       `json:"ApikeyRouteTableBindings"`
}
```

`AiRouteDataFile.UnmarshalJSON` 做两项兼容性处理：

1. 同时支持 canonical 字段名 `route_rules` 与 InnerAPI 中的 `RouteRules`；
2. 将路由表类型 `"api_key"` 规范化为 `"apikey"`。

```go
func (f *AiRouteDataFile) UnmarshalJSON(data []byte) error {
    type rawFile AiRouteDataFile
    raw := &struct {
        *rawFile
        RouteRulesUpper map[string]RouteTableFile `json:"RouteRules"`
    }{
        rawFile: (*rawFile)(f),
    }

    if err := json.Unmarshal(data, raw); err != nil {
        return err
    }

    if f.RouteRules == nil && raw.RouteRulesUpper != nil {
        f.RouteRules = raw.RouteRulesUpper
    }

    for key, table := range f.RouteRules {
        if table.Type == "api_key" {
            table.Type = RouteTypeApikey
        }
        f.RouteRules[key] = table
    }

    return nil
}
```

运行时结构不再包含 JSON 标签；`Cond` 字段在加载时由 `ValidateRouteTable()` 编译为 `condition.Condition`。

```go
type RouteRule struct {
    Name      string
    CondStr   string
    Cond      condition.Condition
    Targets   []bfe_basic.AiRouteTarget
    Fallbacks []bfe_basic.AiRouteFallback
}

type RouteTable struct {
    Type  string
    Owner string
    Rules []RouteRule
}

type AiRouteData struct {
    Version                  string
    RouteRules               map[string]RouteTable
    ApikeyRouteTableBindings map[string][]string
}

func (rt *RouteTable) Match(req *bfe_basic.Request) *RouteRule {
    for i := range rt.Rules {
        rule := &rt.Rules[i]
        if rule.Cond != nil && rule.Cond.Match(req) {
            return rule
        }
    }
    return nil
}

func ValidateRouteTable(table *RouteTable) error {
    switch table.Type {
    case RouteTypeApikey, RouteTypeEntity, RouteTypeGlobal:
    default:
        return fmt.Errorf("invalid route table type: %s", table.Type)
    }

    for i := range table.Rules {
        rule := &table.Rules[i]
        if rule.Name == "" {
            return fmt.Errorf("rule name empty")
        }
        if rule.CondStr == "" {
            return fmt.Errorf("rule[%s] Cond empty", rule.Name)
        }
        cond, err := condition.Build(rule.CondStr)
        if err != nil {
            return fmt.Errorf("rule[%s] build cond[%s] err: %s", rule.Name, rule.CondStr, err)
        }
        rule.Cond = cond

        if len(rule.Targets) == 0 {
            return fmt.Errorf("rule[%s] targets empty", rule.Name)
        }

        totalWeight := 0
        for _, target := range rule.Targets {
            totalWeight += target.Weight
        }
        if totalWeight != 100 {
            return fmt.Errorf("rule[%s] total weight %d != 100", rule.Name, totalWeight)
        }
    }
    return nil
}
```

#### 4.2.3 路由数据加载（bfe_modules/mod_ai_route/data_load.go）

`data_load.go` 负责将 `ai_route.data` 反序列化为 DTO，再转换为运行时结构。

```go
package mod_ai_route

import (
    "fmt"

    "github.com/bfenetworks/bfe/bfe_util"
)

func AiRouteDataLoad(fileName string) (*AiRouteData, error) {
    var file AiRouteDataFile

    if err := bfe_util.LoadJsonFile(fileName, &file); err != nil {
        return nil, fmt.Errorf("LoadJsonFile(): err[%s]", err.Error())
    }

    data := &AiRouteData{
        Version:                  file.Version,
        RouteRules:               make(map[string]RouteTable, len(file.RouteRules)),
        ApikeyRouteTableBindings: file.ApikeyRouteTableBindings,
    }

    for key, tableFile := range file.RouteRules {
        rules := make([]RouteRule, len(tableFile.Rules))
        for i, ruleFile := range tableFile.Rules {
            rules[i] = RouteRule{
                Name:      ruleFile.Name,
                CondStr:   ruleFile.Cond,
                Targets:   ruleFile.Targets,
                Fallbacks: ruleFile.Fallbacks,
            }
        }
        data.RouteRules[key] = RouteTable{
            Type:  tableFile.Type,
            Owner: tableFile.Owner,
            Rules: rules,
        }
    }

    return data, nil
}
```

#### 4.2.4 路由结果上下文（bfe_basic/request_ai_route.go）

`AiRouteResult` 及上下文读写函数放在 `bfe_basic` 包中，便于 `bfe_server` 等其它包访问，避免循环依赖。

```go
package bfe_basic

const CtxAiRouteResult = "__REQ_AI_ROUTE_RESULT"

type AiRouteResult struct {
    RouteType string   // apikey / entity / global
    Owner     string   // route table owner
    RuleName  string   // hit rule name
    Targets   []AiRouteTarget
    Fallbacks []AiRouteFallback
}

type AiRouteTarget struct {
    ClusterName string
    Model       string
    Weight      int
}

type AiRouteFallback struct {
    ClusterName string
    Model       string
}

func (r *Request) SetAiRouteResult(result *AiRouteResult) {
    r.SetContext(CtxAiRouteResult, result)
}

func (r *Request) GetAiRouteResult() *AiRouteResult {
    val := r.GetContext(CtxAiRouteResult)
    if val == nil {
        return nil
    }
    result, ok := val.(*AiRouteResult)
    if !ok {
        return nil
    }
    return result
}
```

## 5. 模块设计

### 5.1 模块结构

新增目录：`bfe/bfe_modules/mod_ai_route/`

文件清单：

| 文件 | 职责 |
|------|------|
| `mod_ai_route.go` | 模块入口，实现 `BfeModule` 接口，注册回调。 |
| `conf_load.go` | 加载 `mod_ai_route.conf` 模块配置。 |
| `data_load.go` | 加载 `ai_route.data` 路由数据文件，转换为运行时结构。 |
| `route_table.go` | 路由表内存结构，提供查找接口。 |
| `route_rule.go` | 定义 JSON DTO、运行时结构体、条件编译与校验及兼容性反序列化。 |
| （无 `context.go`） | `AiRouteResult` 已移至 `bfe_basic/request_ai_route.go`。 |
| `prometheus_states.go` | Prometheus 监控指标（可选）。 |
| `mod_ai_route_test.go` | 单元测试。 |
| `testdata/` | 测试数据。 |

### 5.2 核心类图

```
┌─────────────────────────────┐
│      ModuleAiRoute          │
├─────────────────────────────┤
│ - name: string              │
│ - conf: *ConfModAiRoute     │
│ - routeTable: *AiRouteTable │
│ - state: ModuleAiRouteState │
├─────────────────────────────┤
│ + Name() string             │
│ + Init() error              │
│ + routeFoundProductHandler()│
│ + loadRouteRuleConf()       │
│ + getState()                │
└──────────────┬──────────────┘
               │ uses
               ▼
┌─────────────────────────────┐
│      AiRouteTable           │
├─────────────────────────────┤
│ - routeRules: map[string]*RouteTable │
│ - bindings: map[string][]string      │
├─────────────────────────────┤
│ + Update(data *AiRouteData) │
│ + Search(apiKey string, req *bfe_basic.Request) *AiRouteResult │
└──────────────┬──────────────┘
               │ uses
               ▼
┌─────────────────────────────┐
│      RouteTable             │
├─────────────────────────────┤
│ - Type: string              │
│ - Owner: string             │
│ - Rules: []RouteRule        │
├─────────────────────────────┤
│ + Match(req) *RouteRule     │
└──────────────┬──────────────┘
               │ uses
               ▼
┌─────────────────────────────┐
│      RouteRule              │
├─────────────────────────────┤
│ - Name: string              │
│ - Cond: Condition           │
│ - Targets: []Target         │
│ - Fallbacks: []Fallback     │
└─────────────────────────────┘
```

### 5.3 核心实现

#### 5.3.1 模块主文件（mod_ai_route.go）

```go
package mod_ai_route

import (
    "fmt"
    "net/url"

    "github.com/bfenetworks/go-lib/log"
    "github.com/bfenetworks/go-lib/web-monitor/metrics"
    "github.com/bfenetworks/go-lib/web-monitor/web_monitor"

    "github.com/bfenetworks/bfe/bfe_basic"
    "github.com/bfenetworks/bfe/bfe_http"
    "github.com/bfenetworks/bfe/bfe_module"
)

const ModAiRoute = "mod_ai_route"

var openDebug = false

type ModuleAiRouteState struct {
    ReqTotal       *metrics.Counter
    ReqHitApikey   *metrics.Counter
    ReqHitEntity   *metrics.Counter
    ReqHitGlobal   *metrics.Counter
    ReqMiss        *metrics.Counter
    ReqFallback    *metrics.Counter
}

type ModuleAiRoute struct {
    name       string
    conf       *ConfModAiRoute
    routeTable *AiRouteTable
    state      ModuleAiRouteState
    metrics    metrics.Metrics
}

func NewModuleAiRoute() *ModuleAiRoute {
    m := new(ModuleAiRoute)
    m.name = ModAiRoute
    m.metrics.Init(&m.state, ModAiRoute, 0)
    m.routeTable = NewAiRouteTable()
    return m
}

func (m *ModuleAiRoute) Name() string {
    return m.name
}

func (m *ModuleAiRoute) loadRouteRuleConf(query url.Values) error {
    path := query.Get("path")
    if path == "" {
        path = m.conf.Basic.RouteRulePath
    }

    data, err := AiRouteDataLoad(path)
    if err != nil {
        return fmt.Errorf("err in AiRouteDataLoad(%s): %s", path, err)
    }

    if err := m.routeTable.Update(data); err != nil {
        return fmt.Errorf("err in routeTable.Update: %s", err)
    }

    return nil
}

func (m *ModuleAiRoute) routeFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
    m.state.ReqTotal.Inc(1)

    aiMeta := req.GetAiBasicInfo()
    if aiMeta == nil {
        return bfe_module.BfeHandlerGoOn, nil
    }

    apiKey := aiMeta.ClientApiKey
    if apiKey == "" {
        if openDebug {
            log.Logger.Debug("%s: api key empty, skip", m.name)
        }
        return bfe_module.BfeHandlerGoOn, nil
    }

    result := m.routeTable.Search(apiKey, req)
    if result == nil {
        m.state.ReqMiss.Inc(1)
        if openDebug {
            log.Logger.Debug("%s: no route hit for apiKey[%s]", m.name, apiKey)
        }
        return bfe_module.BfeHandlerGoOn, nil
    }

    switch result.RouteType {
    case RouteTypeApikey:
        m.state.ReqHitApikey.Inc(1)
    case RouteTypeEntity:
        m.state.ReqHitEntity.Inc(1)
    case RouteTypeGlobal:
        m.state.ReqHitGlobal.Inc(1)
    }

    req.SetAiRouteResult(result)

    return bfe_module.BfeHandlerGoOn, nil
}

func (m *ModuleAiRoute) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
    confPath := bfe_module.ModConfPath(cr, m.name)
    var err error
    if m.conf, err = ConfLoad(confPath, cr); err != nil {
        return fmt.Errorf("%s: conf load err %v", m.name, err)
    }
    openDebug = m.conf.Log.OpenDebug

    if err := m.loadRouteRuleConf(nil); err != nil {
        return fmt.Errorf("%s: loadRouteRuleConf err %v", m.name, err)
    }

    if err := cbs.AddFilter(bfe_module.HandleFoundProduct, m.routeFoundProductHandler); err != nil {
        return fmt.Errorf("%s.Init(): AddFilter(routeFoundProductHandler): %s", m.name, err.Error())
    }

    monitorHandlers := map[string]interface{}{
        m.name: m.getState,
    }
    if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, monitorHandlers); err != nil {
        return fmt.Errorf("%s.Init(): RegisterHandlers(monitor): %v", m.name, err)
    }

    reloadHandlers := map[string]interface{}{
        m.name: m.loadRouteRuleConf,
    }
    if err := web_monitor.RegisterHandlers(whs, web_monitor.WebHandleReload, reloadHandlers); err != nil {
        return fmt.Errorf("%s.Init(): RegisterHandlers(reload): %v", m.name, err)
    }

    return nil
}

func (m *ModuleAiRoute) getState(params map[string][]string) ([]byte, error) {
    s := m.metrics.GetAll()
    return s.Format(params)
}
```

#### 5.3.2 路由表查找（route_table.go）

```go
package mod_ai_route

import (
    "fmt"
    "sync"

    "github.com/bfenetworks/go-lib/log"
    "github.com/bfenetworks/bfe/bfe_basic"
)

type AiRouteTable struct {
    lock sync.RWMutex

    // routeRules key: route table key (<type>_<owner>)
    // routeRules value: pointer to the route table
    routeRules map[string]*RouteTable

    // bindings key: API-Key string
    // bindings value: ordered list of route table keys to search
    bindings map[string][]string
}

func NewAiRouteTable() *AiRouteTable {
    return &AiRouteTable{
        routeRules: make(map[string]*RouteTable),
        bindings:   make(map[string][]string),
    }
}

func (t *AiRouteTable) Update(data *AiRouteData) error {
    // validate and compile conditions (outside the lock)
    rules := make(map[string]*RouteTable)
    for key, table := range data.RouteRules {
        if err := ValidateRouteTable(&table); err != nil {
            return fmt.Errorf("validate route table[%s] err: %s", key, err)
        }
        tableCopy := table
        rules[key] = &tableCopy
    }

    // only lock when swapping the atomic references
    t.lock.Lock()
    t.routeRules = rules
    t.bindings = data.ApikeyRouteTableBindings
    t.lock.Unlock()

    return nil
}

func (t *AiRouteTable) Search(apiKey string, req *bfe_basic.Request) *bfe_basic.AiRouteResult {
    t.lock.RLock()

    tableKeys, ok := t.bindings[apiKey]
    if !ok || len(tableKeys) == 0 {
        t.lock.RUnlock()
        return nil
    }

    // copy table references under lock; table.Match() may be expensive,
    // so we release the lock before matching.
    tables := make([]*RouteTable, 0, len(tableKeys))
    for _, key := range tableKeys {
        if table, ok := t.routeRules[key]; ok {
            tables = append(tables, table)
        } else if openDebug {
            log.Logger.Debug("mod_ai_route: route table[%s] not found", key)
        }
    }
    t.lock.RUnlock()

    // match outside the lock to reduce critical section
    for _, table := range tables {
        rule := table.Match(req)
        if rule != nil {
            return &bfe_basic.AiRouteResult{
                RouteType: table.Type,
                Owner:     table.Owner,
                RuleName:  rule.Name,
                Targets:   rule.Targets,
                Fallbacks: rule.Fallbacks,
            }
        }
    }

    return nil
}
```

#### 5.3.3 规则匹配与校验（route_rule.go）

```go
package mod_ai_route

import (
    "fmt"

    "github.com/bfenetworks/bfe/bfe_basic"
    "github.com/bfenetworks/bfe/bfe_basic/condition"
)

func (rt *RouteTable) Match(req *bfe_basic.Request) *RouteRule {
    for i := range rt.Rules {
        rule := &rt.Rules[i]
        if rule.Cond != nil && rule.Cond.Match(req) {
            return rule
        }
    }
    return nil
}

func ValidateRouteTable(table *RouteTable) error {
    switch table.Type {
    case RouteTypeApikey, RouteTypeEntity, RouteTypeGlobal:
    default:
        return fmt.Errorf("invalid route table type: %s", table.Type)
    }

    for i := range table.Rules {
        rule := &table.Rules[i]
        if rule.Name == "" {
            return fmt.Errorf("rule name empty")
        }
        if rule.CondStr == "" {
            return fmt.Errorf("rule[%s] Cond empty", rule.Name)
        }
        cond, err := condition.Build(rule.CondStr)
        if err != nil {
            return fmt.Errorf("rule[%s] build cond[%s] err: %s", rule.Name, rule.CondStr, err)
        }
        rule.Cond = cond

        if len(rule.Targets) == 0 {
            return fmt.Errorf("rule[%s] targets empty", rule.Name)
        }

        totalWeight := 0
        for _, target := range rule.Targets {
            totalWeight += target.Weight
        }
        if totalWeight != 100 {
            return fmt.Errorf("rule[%s] total weight != 100", rule.Name)
        }
    }
    return nil
}
```

## 6. 执行流程

### 6.1 请求处理流程

```
1. 接收 HTTP 请求
2. 经过 HandleBeforeLocation 回调
3. 执行 findProduct()，识别 BFE 租户（AI 网关场景下租户信息仅作为兼容保留）
4. 执行 HandleFoundProduct 回调：
   a. mod_ai_token_auth：鉴权并设置 AiBasicInfo.ClientApiKey
   b. mod_ai_rate_limit：执行限流策略
   c. mod_ai_route：
      - 获取 ClientApiKey
      - 按 ApikeyRouteTableBindings 顺序查找 apikey → entity → global 路由表
      - 命中规则后返回 AiRouteResult
      - 将完整的 targets 和 fallbacks 写入请求上下文
5. 在 AI 网关模式下，由 `ServeHTTPForAI()` 接管；`mod_ai_route` 未命中时直接返回 404
6. 命中 AI 路由后：
   - 进入 ServeHTTPForAI（独立实现，避免影响原有 ServeHTTP）
   - 根据 target 构造 OutRequest
   - Model 非空时覆盖请求体中的 model 字段
   - 调用 clusterInvoke() 转发到目标集群
   - 失败时按 fallbacks 顺序尝试
7. 返回响应
```

### 6.2 路由查找详细流程

```go
func (m *ModuleAiRoute) routeFoundProductHandler(req *bfe_basic.Request) (int, *bfe_http.Response) {
    // 1. 获取 AiBasicInfo 和 ClientApiKey
    // 2. 调用 routeTable.Search(apiKey, req)
    // 3. Search 内部：
    //    a. 根据 apiKey 获取 bindings 列表
    //    b. 遍历每个 route table key
    //    c. 在每张路由表中顺序匹配 rules
    //    d. 命中后构建 AiRouteResult 返回
    // 4. 将命中结果写入上下文
}
```

### 6.3 Fallback 流程

```
当 target 转发失败时：
1. 检查错误类型：
   - 触发 fallback：连接失败、超时、后端 5xx
   - 不触发 fallback：客户端 4xx、限流拒绝、鉴权失败
2. 按 fallbacks 列表顺序尝试：
   - 对每个 fallback，构造新的 target
   - 重新调用 clusterInvoke()
   - 第一个成功即停止
3. 所有 fallback 均失败：
   - 返回最后一个 fallback 的错误响应
```

## 7. 与现有系统的集成

### 7.1 模块注册

在 `bfe/bfe_modules/bfe_modules.go` 的 `moduleList` 中新增：

```go
import (
    "github.com/bfenetworks/bfe/bfe_modules/mod_ai_route"
)

var moduleList = []bfe_module.BfeModule{
    // ... existing modules ...

    // mod_ai_token_auth
    mod_ai_token_auth.NewModuleAITokenAuth(),

    // mod_ai_route
    // Requirement: after mod_ai_token_auth (needs ClientApiKey), before mod_body_process
    mod_ai_route.NewModuleAiRoute(),

    // mod_body_process
    mod_body_process.NewModuleBodyProcess(),

    // depends on token calc
    mod_ai_rate_limit.NewModuleAiRateLimit(),

    // ...
}
```

### 7.2 AI 网关开关

BFE 核心配置 `bfe_config/bfe_conf/conf_basic.go` 已包含 `EnableAiGateway bool` 字段，对应 `bfe.conf`：

```ini
[server]
ai_gateway_enabled = true
```

AI 网关开关在 `bfe_server/http_conn.go` 的 `conn.serveRequest()` 中读取，并决定请求进入 AI 网关转发路径还是原有路径：

```go
// serve the request
var ret1 int
if c.server.Config.Server.EnableAiGateway {
    ret1 = c.server.ReverseProxy.ServeHTTPForAI(w, request)
} else {
    ret1 = c.server.ReverseProxy.ServeHTTP(w, request)
}
```

`EnableAiGateway` 为 `true` 时，`http_conn.go` 还会初始化 `AiBasicInfo`、提取 `ClientApiKey` 与 `ClientModel`，供后续 `mod_ai_route` 等模块使用。`mod_ai_route` 本身不再判断该开关，仅依赖 `AiBasicInfo.ClientApiKey` 是否存在进行路由查找。

### 7.3 复用现有转发能力

- **集群选择**：将选中的 `ClusterName` 写入 `req.Route.ClusterName`，使后续 `findCluster()` 和 `clusterInvoke()` 可复用；
- **模型覆盖**：`Model` 非空时通过 `condition.ReqBodyJsonSet()` 修改请求体 model 字段，与现有 `ReverseProxy.ServeHTTP` 中 AIConf 的 model mapping 逻辑保持一致；
- **连接管理**：复用 `ReverseProxy.transports` 和 `clusterInvoke()` 进行实际转发；
- **降级重试**：在独立的 `ServeHTTPForAI()` 中实现 target/fallback 选择逻辑，调用 `clusterInvoke()` 完成每次转发尝试。

### 7.4 独立 ServeHTTPForAI

为避免对原 `ServeHTTP()` 产生较大影响，已在 `bfe_server/reverseproxy.go` 中新增独立的 `ServeHTTPForAI()`：

```go
func (p *ReverseProxy) ServeHTTPForAI(rw bfe_http.ResponseWriter, basicReq *bfe_basic.Request) (action int) {
    // 1. 调用 HandleBeforeLocation / findProduct / HandleFoundProduct
    // 2. 从 basicReq 中获取 AiRouteResult，未命中则返回 404
    // 3. 调用 HandleAfterLocation
    // 4. 加权选择 target，构造 [selected target] + fallbacks 尝试列表
    // 5. 循环调用 aiClusterInvoke() 转发；失败时按 fallbacks 顺序重试
    // 6. 发送响应
}
```

在 `bfe_server/http_conn.go` 中根据 `EnableAiGateway` 决定调用 `ServeHTTP` 还是 `ServeHTTPForAI`。

## 8. 错误处理与监控

### 8.1 错误码

| 错误场景 | 错误码 | 说明 |
|----------|--------|------|
| AI 路由未命中（无 bindings 或所有路由表均未匹配） | `ErrBkFindLocation` | 返回 404 Not Found。 |
| targets 权重总和不等于 100 | 配置校验错误 | 启动/热加载时拒绝。 |
| 条件表达式编译失败 | 配置校验错误 | 启动/热加载时拒绝。 |
| target 转发失败 | 复用现有 `ErrBk*` 错误码 | 进入 fallback 流程。 |

### 8.2 监控指标

在 `ModuleAiRouteState` 中定义：

| 指标 | 类型 | 含义 |
|------|------|------|
| `ReqTotal` | Counter | 处理请求总数。 |
| `ReqHitApikey` | Counter | 命中 apikey 路由表次数。 |
| `ReqHitEntity` | Counter | 命中 entity 路由表次数。 |
| `ReqHitGlobal` | Counter | 命中 global 路由表次数。 |
| `ReqMiss` | Counter | 未命中任何路由表次数。 |
| `ReqFallback` | Counter | 触发 fallback 次数。 |

可选 Prometheus 指标：

| 指标 | 标签 | 含义 |
|------|------|------|
| `ai_route_hit_total` | `type`, `owner`, `rule_name` | 各规则命中次数。 |
| `ai_route_fallback_total` | `owner`, `rule_name`, `fallback_cluster` | fallback 触发次数。 |

## 9. 配置热加载

通过 Web 监控接口支持热加载：

```
GET /reload/mod_ai_route
```

调用 `loadRouteRuleConf()` 重新加载 `ai_route.data`，校验通过后原子替换内存中的 `AiRouteTable`。

## 10. 实现步骤

1. **新增模块目录和配置文件**
   - `bfe/bfe_modules/mod_ai_route/`
   - `bfe/conf/mod_ai_route/mod_ai_route.conf`
   - `bfe/conf/mod_ai_route/ai_route.data`

2. **实现配置加载**
   - `conf_load.go`：加载 `mod_ai_route.conf`。
   - `data_load.go`：加载 `ai_route.data`。

3. **实现路由核心逻辑**
   - `route_rule.go`：定义数据结构、条件编译、校验。
   - `route_table.go`：实现路由表查找。

4. **实现模块入口**
   - `mod_ai_route.go`：注册 `HandleFoundProduct` 回调、监控与热加载。

5. **上下文传递**
   - `bfe_basic/request_ai_route.go`：定义 `AiRouteResult` 与上下文读写。`mod_ai_route` 通过 `req.SetAiRouteResult()` / `req.GetAiRouteResult()` 访问。

6. **注册模块**
   - 在 `bfe_modules/bfe_modules.go` 中引入并注册。

7. **转发层适配**
   - 在 `bfe_server/reverseproxy.go` 中新增 `ServeHTTPForAI()`，处理 target/fallback 逻辑。
   - 在连接处理层根据 `EnableAiGateway` 分发到 `ServeHTTPForAI`。

8. **测试**
   - 单元测试：配置解析、条件匹配、加权选择、fallback 触发。
   - 集成测试：与 `mod_ai_token_auth` 协同，验证完整请求链路。

## 11. 风险与注意事项

1. **与 mod_ai_token_auth 的执行顺序**：
   `mod_ai_route` 必须排在 `mod_ai_token_auth` 之后，确保 `ClientApiKey` 已被设置。

2. **与原有 BFE 路由的兼容**：
   当 `EnableAiGateway = false` 时，`mod_ai_route` 不执行任何逻辑，保持原有行为。

3. **Model 覆盖与请求体修改**：
   修改请求体 model 后需同步更新 `Content-Length`，或设置为 `-1` 使用 chunked 编码。

4. **Fallback 与请求体重用**：
   由于请求体可能被读取，fallback 时需确保 Body 可重复读取（复用 `BodyAccessor` 或提前缓存）。

5. **配置原子性**：
   热加载失败时不应影响当前内存中的路由表，更新操作应在校验完成后原子切换。

## 12. 附录

### 12.1 关键文件路径

| 文件 | 路径 |
|------|------|
| 模块主文件 | `bfe/bfe_modules/mod_ai_route/mod_ai_route.go` |
| 模块配置加载 | `bfe/bfe_modules/mod_ai_route/conf_load.go` |
| 路由数据加载 | `bfe/bfe_modules/mod_ai_route/data_load.go` |
| AI 路由结果上下文 | `bfe/bfe_basic/request_ai_route.go` |
| 路由表 | `bfe/bfe_modules/mod_ai_route/route_table.go` |
| 路由规则 | `bfe/bfe_modules/mod_ai_route/route_rule.go` |
| 模块注册 | `bfe/bfe_modules/bfe_modules.go` |
| AI 网关开关配置 | `bfe/bfe_config/bfe_conf/conf_basic.go` |
| 转发逻辑 | `bfe/bfe_server/reverseproxy.go` |

### 12.2 依赖的现有 BFE 能力

| 能力 | 来源 |
|------|------|
| 条件表达式编译 | `bfe_basic/condition` |
| 请求上下文 | `bfe_basic.Request.Context` |
| AI 基础信息 | `bfe_basic.AiBasicInfo` |
| 回调注册 | `bfe_module.BfeCallbacks` |
| 监控接口 | `web_monitor` |
| 集群查找与转发 | `bfe_server.ReverseProxy.clusterInvoke` |
| 请求体修改 | `bfe_basic/condition.ReqBodyJsonSet` |
