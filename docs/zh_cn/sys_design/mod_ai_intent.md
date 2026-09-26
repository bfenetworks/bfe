# mod_ai_intent 系统设计文档

## 1. 背景与目标

### 1.1 背景

`mod_ai_route` 实现了确定性的 AI 路由（condition DSL 匹配 apikey/entity/global
路由表），但条件维度仅限 host/path/header/model 等**请求显式携带的字段**，不理解
请求内容的语义。语义路由需求要求按"任务意图"（编码/测试/文档）与"复杂度"等
语义维度路由到不同模型，必须先对请求做一次本地小模型意图分类。

决策服务侧采用 System One 协议（`POST /v1/systemone`，TypeSafe 事实标准）：
输入 state（文本）+ 一组类型化问题（choice/score），一次前向返回带概率与校准
置信度的结构化答案。本地可部署的实现有 Laya（322M 多语言，CPU 可跑）与
Kev（Qwen3.5 底座，GPU）等，网关侧通过同一协议对接，后端可替换。

### 1.2 目标

1. 新增 `mod_ai_intent` 模块，对 AI 请求做意图分类，结果供 `mod_ai_route`
   路由规则通过新 condition 原语 `req_ai_intent_in` 消费；
2. **懒解析**：仅当路由规则真的引用意图条件时才触发分类，不用意图的流量零开销；
3. **可热更的问题集**：问题定义（名称/类型/选项）与置信度门控阈值放在
   `intent_questions.data` 中，热加载、零代码扩展新维度；`Questions` 允许为
   空数组（0 个问题）= 停用意图分类软开关，BFE 侧所有意图条件不命中、流量走
   默认路由；
4. **失败永不阻断主请求**：决策服务超时/故障/低置信一律降级为 unknown，意图
   条件不命中，流量回落默认路由。

## 2. 术语定义

| 术语 | 定义 |
|------|------|
| Decision Service（决策服务） | 提供 System One 协议意图分类能力的服务（Laya/Kev/Jev），网关以 HTTP 客户端对接 |
| Question（问题） | 一次分类询问的一个维度（如 task_type/complexity），choice 或 score 两类 |
| Answer（答案） | 决策服务对某问题的回答：choice 为选中项+概率分布；score 为期望档位索引+各档概率 |
| answer_confidence | 报告答案的概率值，门控与规则阈值的比较对象 |
| unknown | 答案不可用状态：决策服务错误、未返回、问题未配置，或置信度低于门控阈值 |
| 懒解析（lazy resolve） | 意图分类由 condition 原语首次求值触发，而非独立回调 |
| 读取时门控 | 缓存/上下文保存原始概率，unknown 按当前阈值在答案被读取时派生 |

## 3. 方案选型

### 3.1 插入点：懒解析 vs 模块顺序硬约束

`AddFilter` 不支持优先级，HandleFoundProduct 回调按模块注册顺序执行。若用
"eager 分类 + 注册在 mod_ai_route 之前"的方案，所有 AI 请求都要付分类延迟
（本机 CPU 版 Laya 实测 665–800ms/次），即使路由规则根本不用意图。

**选定方案：懒解析**。分类入口是 condition 原语求值本身——`req_ai_intent_in`
首次求值 → 调用注册在 `bfe_basic` 的 resolver 函数 → 分类一次 → 结果写入
请求上下文（`CtxAiIntent`）→ 同请求后续求值直接复用（`Resolved` 标记防重）。
`bfe_basic` 只持有 resolver 函数指针（`SetAiIntentResolver`，Init 注入），避免
`bfe_basic` 反向依赖模块包。由此模块注册顺序不再敏感。

### 3.2 决策服务协议

System One `POST /v1/systemone`：请求体 `{state, questions{...}}`，响应每个
问题的 `{choice/score, probabilities, confidence}` 与 `usage`。问题集全量组装
进单次调用（并行评估），问题数 2–10 对延迟影响在毫秒级。后端可配置多地址
（主备），Laya → Kev 替换仅改地址。

### 3.3 置信度门控：解析时固化 vs 读取时派生

**选定：读取时派生**。缓存与请求上下文保存原始概率；`Unknown` 在规则读取答案时
按当前生效阈值（`intent_questions.data` 的全局/逐问题 `MinConfidence`）计算。
收益：调阈值热更后对新请求立即生效，标定迭代（调阈值→看日志→再调）不需要等
缓存过期或重新分类。

### 3.4 显式信号优先

客户端可通过请求头（默认 `X-AI-Intent: <question>=<option>`）显式声明意图：
合法值直接采信（`Source=header`，confidence=1.0，**不调决策服务**），优先于
模型分类；非法条目忽略并回落模型分类。适用：agent sub-agent 调用、IDE 斜杠
命令、CI 管道、联调绕过分类器。

## 4. 数据流

```
HandleFoundProduct（mod_ai_token_auth 完成鉴权、AiBasicInfo 就绪）
   │
   ▼  mod_ai_route.routeTable.Search 求值规则条件
req_ai_intent_in("task_type", "test_writing" [, 0.9]) 首次求值
   │
   ▼  bfe_basic.GetAiIntent(req)（resolver 懒解析入口）
① 显式头检查：X-AI-Intent 合法条目直接采用
② msg_extract：GetBodyAccessor 读 body，按协议提取 last user message
   （openai/anthropic/gemini），按字符截断（MaxStateChars）
③ 缓存查询：key = sha1(apikey + QuestionsVersion + 提取文本)，hit 即返回
④ 决策服务调用：POST /v1/systemone（TimeoutMs / 熔断保护）
⑤ 写上下文：req.SetAiIntent(AiIntent{Answers: map[问题名]IntentAnswer})
   │
   ▼  原语匹配
答案 ∈ options 且 answer_confidence ≥ 有效门槛（省略第三参 = 该问题 MinConfidence）
   │ 不匹配 / unknown → fall through 下一条规则
```

## 5. 状态存储

### 5.1 请求上下文（单请求生命周期）

| 存储 | 键/字段 | 说明 |
|------|---------|------|
| req.Context | `CtxAiIntent`（`__REQ_AI_INTENT`） | `*AiIntent`：QuestionsVersion、Answers（按问题名索引的 IntentAnswer，含原始概率）、Source（header/model/cache）、BackendVersion、LatencyMs、Resolved 标记 |
| AiBasicInfo | `IntentAnswers`/`IntentSource`/`IntentLatencyMs` | 透传访问日志（access log 新增 `ai_intent_*` 字段） |

### 5.2 进程内缓存（跨请求）

| 项 | 值 |
|----|----|
| 结构 | LRU（容量 CacheSize，默认 10000 条）+ TTL（默认 1800s） |
| Key | `sha1(apikey + "\x00" + QuestionsVersion + "\x00" + 提取文本)` |
| Value | `*AiIntent`（原始概率，门控在读取时按当前阈值派生） |
| Version 语义 | `intent_questions.data` 内容任何变更必须 bump Version（变更检测 + 缓存失效依据）；版本不变的热更视为无变更跳过 |

### 5.3 熔断器

连续失败/超时达到 `Breaker.FailureThreshold`（默认 5）后打开，期间意图一律
unknown；每 `ProbeIntervalMs`（默认 5000）放行一次探测请求，成功则关闭。

## 6. 边界情况与优化

| 场景 | 行为 |
|------|------|
| 决策服务超时/5xx/网络错误/熔断 | 全部问题 unknown + `ReqErr` 计数；路由不命中走默认规则；**主请求不受影响** |
| 答案置信度低于门控阈值 | 该问题读取时视为 unknown（原始概率保留供日志/离线分析） |
| 规则引用未配置的问题名 | 该条件恒 false（debug 日志）；配置补齐后规则自动生效 |
| `Questions` 为空数组（软开关停用） | 所有意图条件不命中、流量走默认路由；不提取消息、不调用决策服务、零分类开销 |
| 请求头显式意图非法值 | 忽略该条目，回落模型分类；多条目逐条独立判定 |
| 非 AI 协议请求 / body 提取失败 | 不分类（unknown），意图条件不命中 |
| body 已被消费 | 提取走 `GetBodyAccessor`（可回绕），不影响后续转发 |
| questions 热更校验失败 | 拒绝热更并保留旧版 |
| 同请求多次 attempt（fallback 重试） | 意图解析一次，attempts 间不变 |
| mod_ai_cache 命中短路 | v1 仍先解析意图（懒触发）；短路优化列入二期 |

## 7. 路由规则示例

```
# 测试任务 + 高置信 → 便宜模型；其余（含 unknown）→ 主力模型
cond: req_ai_intent_in("task_type", "test_writing", 0.9)
targets: [{ClusterName: "cluster_flash", Model: "deepseek-v4.1-flash", Weight: 100}]
fallbacks: [{ClusterName: "cluster_kimi", Model: "kimi-2.8"}]
```

## 8. 监控

模块计数器：`ReqTotal / ReqResolved / ReqHeader / ReqCacheHit / ReqUnknown /
ReqErr / BreakerOpen` + 分类延迟直方图；模块状态页展示当前 `QuestionsVersion`。

## 9. 性能

- 生产预算 ≤50ms P95（GPU 后端 + 同机房）；本机 CPU 版 Laya 实测 665–800ms/次，
  联调时 `TimeoutMs` 调至 2000；
- 懒解析保证不用意图的规则零开销；缓存命中 <1ms；
- 决策服务容量由缓存命中率决定（独立消息速率远低于请求速率）。
