# mod_ai_intent：语义路由意图模块

日期：2026-09-26

## 背景与目标

语义路由需求：对 LLM 请求先做本地小模型意图分类（编码/测试/文档 + 复杂度），
路由规则按"意图 × 复杂度 × 既有条件"选择目标模型。数据面此前为纯确定性路由
（condition DSL 匹配），无请求内容语义能力。

本次改动新增 `mod_ai_intent` 模块，对接决策服务（System One 协议
`POST /v1/systemone`，本地部署的 Laya/Kev/Jev 均兼容），使意图成为路由规则引擎
的一等条件维度。

## 主要改动点

### 新增：`bfe_modules/mod_ai_intent/` 模块

| 文件 | 职责 |
|------|------|
| `mod_ai_intent.go` | BfeModule 实现：Init 注入 resolver、监控 handler、热更入口 |
| `intent_resolve.go` | 懒解析主逻辑：提取→缓存→调用→写上下文（门控在读取时） |
| `decision_client.go` | System One HTTP 客户端（超时/熔断） |
| `msg_extract.go` | last user message 提取（openai/anthropic/gemini） |
| `questions_conf.go` | questions 动态配置：加载/校验/原子替换/版本管理 |
| `intent_cache.go` | 进程内 LRU（key 含 questions Version） |
| `conf.go` | 模块静态配置加载 |

核心机制：

- **懒解析**：分类由 condition 原语求值按需触发（resolver 注入 `bfe_basic`），
  不用意图的规则零开销；失败/超时/低置信一律降级为 unknown，不阻断主请求；
- **读取时门控**：缓存与上下文保存原始概率，`Unknown` 按当前 `MinConfidence`
  派生，阈值随 questions 数据文件热更即时生效；
- **显式信号优先**：`X-AI-Intent: <question>=<option>` 请求头直接采信
  （合法值不调决策服务），优先于模型分类。

### 新增：`bfe_basic/request_ai_intent.go`

`CtxAiIntent` 常量、`AiIntent`/`IntentAnswer` 结构（答案按问题名索引，结构不绑定
具体问题）、`Get/Set` 访问器、`Match()`、包级 resolver 注入点
（`SetAiIntentResolver`，避免 bfe_basic 反向依赖模块包）。

### 新增：condition 原语

`req_ai_intent_in(String, String [, Float])`：按问题名匹配任意已配置问题
（choice 按选中项、score 按档位名），选项列表为 `|` 分隔串（BFE `*_in` 惯例）；
第三参可省略（= 该问题当前 MinConfidence 作门槛），显式 N 为更严门槛，显式
`0.0` 为关闭置信检查（DSL 中零必须写作 `0.0`，整数字面量 `0` 是 INT 会被
原型检查拒绝）。

**对 BFE 惯例的小幅扩展**：BFE 原语严格 arity（`parser/semant.go` 的
`prototypeCheck`），为实现"第三参可省略"，在 `prototypeCheck` 增加白名单制的
尾参缺省机制（`optionalLastArgProtos`，目前仅 `req_ai_intent_in`），不影响任何
现有原语。另：FLOAT token 需在 `cond.y` 的 Lex 词法分发中补映射到 `BASICLIT`
（一行改动），并 `make prepare` 重新生成 parser（goyacc v0.27.0）。

### 注册

`bfe_modules/bfe_modules.go`：AI 组内新增 `mod_ai_intent.NewModuleAiIntent()`
（注册顺序不敏感，懒解析机制不依赖回调先后）。

## 配置影响

新增配置（不修改任何既有配置，全部向后兼容）：

- `conf/mod_ai_intent/mod_ai_intent.conf`（INI，模块静态配置：决策服务地址、
  超时、缓存、熔断、显式意图头名）；
- `conf/mod_ai_intent/intent_questions.data`（JSON，热加载：Version、
  全局/逐问题 MinConfidence、Questions[]（choice/score 两类））。

路由规则消费：在 `ai_route.data` 的规则 `Cond` 中使用
`req_ai_intent_in("task_type", "test_writing")` 等表达式。

## 兼容性说明

- 不引用意图原语的路由规则行为完全不变；模块未加载/未启用时意图原语求值为
  false（规则不命中，走原有默认路由）；
- 分类调用是网关内部子请求，不影响鉴权/配额/计费链路；
- 决策服务不可用时全部问题为 unknown，流量自动落到默认规则，主请求不受影响；
- 本机开发联调注意：`TimeoutMs` 默认 300ms，连本机 CPU 版 Laya（665–800ms/次）
  需调至 2000（见 `decision-service` 环境文档）。

## 修订记录

### 2026-09-26：放宽 Questions 下限 1→0（空数组软开关）

控制面冻结契约要求 `intent_questions.data` 的 `Questions` 允许为空数组
（`[]` = 停用意图分类软开关）：BFE 收到后所有意图条件不命中、流量走默认路由。

- `questions_conf.go` 校验放宽：空 Questions 合法（0 个问题，Version 仍必填、
  变更仍需 bump）；非空问题的逐项校验（名称/类型/Instructions/Criteria/Levels/
  MinConfidence）不变；
- resolver 行为：空 questions 时直接返回 Resolved 的空未知意图——不提取消息、
  不解析显式意图头（无已配置问题可匹配）、不调用决策服务、不读写缓存；
  计数 ReqTotal + ReqUnknown；返回的 `AiIntent.QuestionsVersion` 仍为当前版本；
- 缓存自然失效：缓存 key 含 QuestionsVersion，软开关热更（bump 版本）后旧
  条目不再被命中，随 TTL 淘汰；
- 配置文档同步（zh_cn + en_us）：Questions 合法性改为 0–255 项，补充软开关
  语义说明；`docs/zh_cn/sys_design/mod_ai_intent.md` §1.2/§6 同步。
