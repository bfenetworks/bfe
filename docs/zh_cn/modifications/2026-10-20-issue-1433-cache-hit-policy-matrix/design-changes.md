# 缓存命中与模型访问控制 / 限流 / 配额的关系（条款补齐：命中不执行、命中不计量）

日期：2026-10-20

状态：**条款已确认（2026-10-20），数据面代码零改动**。§二 的行号与结论基于工作区 HEAD `ca1bbfc8` 逐条核实；§三 是已生效条款（分歧点裁定见 §3.3）；§四 只含文档落地与测试断言改写。

- Issue：[bfenetworks/bfe#1433](https://github.com/bfenetworks/bfe/issues/1433)（SC1401-TC028：【文档】缓存命中短路与 target allowlist / 限流的先后顺序缺少产品条款）
- 代码库：`bfe/`（数据面：`mod_ai_cache`、`mod_ai_token_auth`、`mod_ai_rate_limit`、`mod_access_pb3`）
- 关联文档：`2026-10-02-ai-cache-lookup-before-route`（缓存查找前移，命中短路的来源）、`2026-09-23-issue-1387-key-model-allowlist-target-model`（allow/block 校验后移到 `HandleAfterAITargetModel`）、`2026-09-24-ai-cache-exact-match` / `2026-09-30-ai-cache-semantic-cache`、`docs/zh_cn/sys_design/ai_cache.md`、`docs/zh_cn/sys_design/ai_access_log_fields.md`
- 产品口径来源（allowlist 的目的）：`ai-gateway-web/docs/zh-cn/09-api-key.md:57,200`、`ai-gateway-web/docs/zh-cn/08-entity.md:63,163-179`、`document-ai-gateway/API设计/v0.2.2/瑛菲AI网关配额控制与限流-APIv0.2.2.md:808-809`——"允许模型/禁止模型"与配额计划、限流策略同属**配额控制与限流**能力，语义是"限制这个 Key 可以使用哪些（要花成本的）模型"。

---

## 一、结论先行

issue 实测确认的行为（同一 Key 收紧 `models` 后对照请求 `400 MODEL_NOT_ALLOWED`、rpm 额度耗尽后对照请求 `429 RPM_LIMIT_EXCEEDED`，而两种状态下的缓存命中请求均 `200` + `X-Bfe-Ai-Cache: hit`）**不是缺陷，是符合条款的预期行为**。issue 缺的是把这条写成可执行的条款，不是修代码。

三个问题的答案：

1. **命中时是否仍执行 target allowlist？** 否。模型访问控制（允许/禁止模型）的目的是约束 Key 能**使用**哪些（有成本、有价格档位的）模型；命中不发起上游调用、不产生 token、不计费，成本口径的目的没有被触达，因此不属于"绕过"。当前"命中直接出内容"与"命中跳过配额/费用扣减"是同一口径（`2026-10-02` §5）。
2. **命中时是否仍执行限流/配额策略？** 否，且命中不消耗 rpm/tpm/concurrency 额度、不扣配额。限流保护的是上游 provider 容量与计费公平，命中两者皆不涉及。
3. **不需要执行的部分如何在日志/计费体现？** 不改 schema。`ai_cache_status=hit` / `hit_semantic` 就是"成本类策略未执行"的充分标识：命中请求必然 `ai_rate_limit_hits` 为空、`ai_auth_info` 为空、`token usage` 与费用为 0。把这套推导规则在 `ai_access_log_fields.md` 里写死一次即可，不新增字段（理由见 §4.1）。

配套必须写进条款的两条推论（否则条款不完整）：

- **策略变更为"后生效"**：收紧 `allow_models` / 新绑限流策略**不会主动失效已写入的缓存条目**，对命中请求的滞后上界 = 条目 TTL（默认 3600s，`conf_mod_ai_cache.go:27`）。
- **命中仍然消耗网关自身资源**（BFE CPU、Redis、带宽），只是不消耗 provider 容量与配额。命中 QPS 目前没有租户级约束，属已知残余风险（§七）。

### 1.1 对上一稿结论的更正

本文档前一稿以"allowlist 是内容访问边界，命中出内容等价于一次成功的模型调用"为由，要求命中仍校验 allowlist，并把拒绝形态设计为"降级为未命中 → 400"。该论证不成立：

- "拿到内容" ≠ "发生了一次模型调用"。命中路径没有 provider 调用、没有 token 计量、没有扣费，与"一次成功的模型调用"在成本、计费、上游容量三个维度上都不同；
- 产品文档把 `allow_models`/`block_models` 与配额、限流放在同一能力域（"配额控制与限流"），并未定义为合规/内容边界；
- 按"内容边界"解读会引入一笔不小的实现代价（§3.2），且在成本口径下收益为零。

因此本稿的结论翻转为"命中不执行 = 符合条款"，**整份变更不涉及数据面代码**：条款 + `sys_design` 文档 + 控制面文档 + 测试断言改写（§四）。原实现方案保留在附录 A，仅在产品明确改判为"命中仍须校验"时才需要。

## 二、现状与根因（为什么"命中不执行"是结构性事实）

### 2.1 命中链路的调用序（逐条核实）

```text
http_conn.go:559-570          InitAiBasicInfo：ClientModel = TargetModel = body/路径提取的模型
HandleFoundProduct
  mod_ai_token_auth   :737    鉴权（Key 有效/启用/过期/子网）→ SetTokenAuthContext(:829)
  mod_ai_cache        :243    规则匹配 → 读 body → 取 question → 建键 → Redis GET
                              ├─ 命中：AiCacheStatus=hit、AiCacheHit=true → BfeHandlerFinish（request.go:113）
                              └─ 未命中：存 ctx → GoOn
  mod_ai_route        :171    路由规则求值（命中时被短路）
HandleAfterAITargetModel
  mod_ai_token_auth   :746    ValidateTargetModel（#1387：按 target model 校验 allow/block）
  mod_ai_rate_limit   :501    targetModelCheckHandler（rpm/tpm/concurrency）
  mod_ai_batch        bfe_modules/mod_ai_batch/mod_ai_batch.go:182   批量操作分类
  mod_ai_context      bfe_modules/mod_ai_context/mod_ai_context.go:115 上下文压缩
HandleReadResponse
  mod_ai_cache        :250    回写（response.go:63 writeBack → :90 Setex / :98 语义 upsert）
HandleRequestFinish
  mod_ai_token_auth   :756    配额/费用结算（AiCacheHit 跳过扣减）
  mod_ai_rate_limit   :506    并发释放 / TPM 结算（依赖 PolicyLimiterContext，命中时不存在）
```

模块注册序：`bfe_modules/bfe_modules.go:155`（token_auth）→ `:166`（cache）→ `:171`（route）→ `:192`（rate_limit）→ `:202`（batch）→ `:212`（context）。

### 2.2 为什么不能"便宜地"在命中前校验

`target model` 由 `computeTargetModel`（`bfe_server/reverseproxy.go:1534-1557`）在**每次集群尝试**里计算，输入是①路由目标 `attempt.Model` 覆盖、②`AIConf.StripPrefix` 前缀裁剪、③`AIConf.ModelMapping` 集群映射；①②③ 都要先完成 `mod_ai_route` 的规则求值与目标选择。命中恰恰短路了这一步，所以"命中前校验 allowlist"必然要求把路由求值（以及 `req_ai_intent_in` 的懒解析）重新加到命中路径上——正好抵消 `2026-10-02` 前移的全部收益。

换句话说：**"命中不执行 allowlist"不是遗漏，而是"命中不路由"的直接推论**。条款只要认下这个推论，就不需要任何代码改动。

### 2.3 判定点全景（当前 HEAD）

| # | 判定 | 注册点 | 命中时 | 目的 |
|---|------|--------|--------|------|
| A | Key 有效/启用/过期/子网 | `HandleFoundProduct`（token_auth `:737`） | **执行** | 身份与凭证有效性（安全，非成本） |
| B | target allow/block | `HandleAfterAITargetModel`（token_auth `:746`） | **不执行** | 可使用的模型范围（成本） |
| C | rpm/tpm/concurrency | `HandleAfterAITargetModel`（rate_limit `:501`） | **不执行** | 上游容量与计费公平（成本） |
| D | 配额/扣费 | `HandleRequestFinish`（token_auth `:756`） | 不扣减（`AiCacheHit`） | 计费（成本） |
| E | 路由/意图 | `HandleFoundProduct`（route `:171`） | 不执行 | 选目标（命中无需） |
| F | 访问日志 | `mod_access_pb3/request_log.go:665` | `ai_cache_status=hit`，`ai_route_rule_hits` 空 | 可观测 |

注意 A 与 B/C/D 的性质差异：鉴权是"身份是否可信"，与成本无关，必须执行；B/C/D 是"用量与成本控制"，命中不产生用量。这条区分就是条款的骨架。

## 三、条款（已确认）

### 3.1 条款正文

> **T-1433-1（命中不执行成本类策略）**：请求在 `mod_ai_cache` 命中并短路时，**不执行**模型访问控制（allow/block）、限流（rpm/tpm/concurrency）与配额扣减，也**不消耗**上述任何额度。原因是这些策略的目的都是约束"模型调用产生的成本与上游容量占用"，而缓存命中不产生 provider 调用、不产生 token、不计费。
>
> **T-1433-2（鉴权不被跳过）**：API-Key 的有效性、启用状态、过期时间、子网校验始终先于缓存查找执行（`bfe_modules.go:155` 的注册序保证）。任何非法/失效 Key 都不会拿到缓存内容。
>
> **T-1433-3（变更为后生效）**：模型访问控制与限流策略的变更**不主动失效**已写入的缓存条目；变更对命中请求的生效滞后上界 = 条目 TTL（默认 3600s）。若要求"策略收紧立即对命中生效"，属于新需求，需另行设计（附录 A）。
>
> **T-1433-4（可观测，不新增字段）**：命中请求"哪些策略未执行"由 `ai_cache_status=hit` / `hit_semantic` 唯一标识——命中 ⟹ 模型访问控制未求值、限流未检查未计量、配额未扣减、未计费。判定规则写入 `ai_access_log_fields.md`；不新增日志字段、不改 `bfe-access-pb`。
>
> **T-1433-5（错误码不变）**：命中路径不产生新的错误码；`MODEL_NOT_ALLOWED` / `RPM_LIMIT_EXCEEDED` 只出现在未命中链路。

### 3.2 为什么不选"命中仍校验 allowlist"

| 代价 | 说明 |
|---|---|
| 要么撤销 `2026-10-02` 的前移 | 命中前必须完成路由求值 + 意图懒解析（§2.2），命中路径延迟回到前移之前 |
| 要么给缓存条目加模型维度 | 写入时记录 target model（精确值信封 + 语义 metadata），受限 Key 换独立键空间；`mod_ai_cache` 的读写两侧都要改，且引入"条目模型 ≠ 本次请求模型"的近似（附录 A） |
| 收益为 0（成本口径下） | 命中不花钱、不占上游容量；校验通过或不通过，都不改变"这次请求没有被计费"的事实 |
| 引入新的一致性负担 | 拒绝形态若在命中侧直接 400，会与正常链路的错误体/日志口径形成两处实现；若降级为未命中，则每次都要付一次路由求值 + 一次上游调用（在允许的情况下） |

### 3.3 分歧点裁定（2026-10-20 已确认）

裁定问题：**允许模型是否仅用于成本/配额治理？**

裁定结论：**是，仅用于成本/配额治理**（product owner，2026-10-20）。因此：

- 成本口径成立 → **T-1433-1~5 即为生效条款**，命中不执行 allowlist/限流/配额是预期行为，`mod_ai_cache` 的命中短路路径不需要任何准入校验；
- 附录 A 的"命中仍校验 target allowlist"方案**不启用**，仅作为将来产品语义改判时的备用设计存档；
- 若后续出现"某租户只能看到某供应商模型产出的内容"这类合规/溯源诉求，属于**新的产品条款**，需要重新立卡并按附录 A 实施，不能沿用本次裁定。

## 四、修复方案（本次不改数据面代码）

### 4.1 不做的事：不新增 `ai_policy_skipped` 字段（前稿提案已撤）

前稿曾提议给访问日志加 `repeated string ai_policy_skipped`（`model_access` / `rate_limit` / `quota`）来"显式声明策略未执行"。该提案撤销，理由三条：

1. **零增量信息**："命中 ⟹ 成本类策略未执行"是 `ai_cache_status` 的纯函数——`ai_cache_status ∈ {hit, hit_semantic}` 等价于"allow/block 未求值、rpm/tpm/concurrency 未检查未计量、配额未扣、计费为 0"。再加一个字段等于把同一个事实写两遍。
2. **多一个事实来源就多一处不一致**：将来语义一旦变化（例如产品决定命中开始计入 rpm），该字段与 `ai_cache_status` 必须靠人肉保持同步，迟早出现"`ai_cache_status=hit` 但 `ai_policy_skipped` 为空"这类自相矛盾的日志，而消费方无法判断该信哪个。
3. **成本是永久的**：新字段要跨仓改 `bfe-access-pb` 的 proto + 重新生成 + 该仓字段文档 + `mod_access_pb3` 写入 + `bfe_basic` 字段 + 单测，全部进入长期维护面；相对"省掉一次推导"的收益不成比例。同理不新增 `CACHE_HIT_POLICY_SKIP` 计数——`cache_hit` 已存在，跳过量即命中量。

issue 第 3 问（"命中响应是否应在访问日志或计费上体现策略未执行"）的答案因此是：**不改 schema，把推导规则写死一次**，让消费方不必各自推理（见 4.2 第 2 条）。计费侧本就无需表达——`token usage` 与费用为 0 已经说明一切。

### 4.2 步骤 1（必改）：文档落地

- `docs/zh_cn/sys_design/ai_cache.md`：新增小节"命中时的策略语义"，把 §3.1 的 T-1433-1~5 原样落进去；流水线图（§4）在命中分支标注"不执行 model_access / rate_limit / quota"。
- `docs/zh_cn/sys_design/ai_access_log_fields.md`：**不新增字段**。在 `ai_cache_status` 条目下补一段判定规则：`ai_cache_status ∈ {hit, hit_semantic}` ⟹ 该请求未执行模型访问控制、未检查/未计量限流、未扣配额、未计费；并补一条"命中请求的 `ai_target_model` 仍为客户端模型"的说明（`mod_ai_cache` 不覆盖 `TargetModel`，命中日志里它来自 `InitAiBasicInfo`，值等于请求体模型）。
- `docs/zh_cn/sys_design/ai_rate_limit_redis_key.md`：说明 rpm/tpm/concurrency 的 Redis 计数只在未命中路径发生，命中不计量。
- `2026-10-02-ai-cache-lookup-before-route/design-changes.md` §5：补一行指向本文档（该文只覆盖了"鉴权在缓存之前"，未定义命中时的策略语义——正是 issue 指出的缺口）。
- 控制面文档（另仓 `ai-gateway-web`）：`09-api-key.md:200`（§10.7 运行时生效机制）与 `08-entity.md:163-179`（§8.5）补一句"缓存命中的请求不执行模型访问控制与限流，命中不计量"，避免前端/运维按"命中也会 403/429"预期排障。
- `CHANGELOG.md`：**不加条目**（本变更无代码改动，属文档类）。

### 4.3 步骤 2（必改）：测试只改断言，不加单测

不需要新增单测：`mod_ai_cache_test.go:84-113` 已经钉住"命中路径只注册 `HandleFoundProduct` + `HandleReadResponse`"，即"命中不注册 allowlist/限流回调"这一事实由既有注册点断言覆盖，条款变化不改变实现。

集成断言：两条新用例都放在 **`scenario-SC20`（精确缓存）**，用"对照请求证明策略在未命中链路生效 + 命中请求不触发策略"的成对断言（负向回归）：

- **TC-12 `TestTC12_CacheHitSkipsModelAllowList`**：先用 `allow_models="deepseek-chat"` 写入缓存，停 BFE 后用同一 Redis 重启、把 `allow_models` 收紧为 `other-model`；
  ① 对照（带 `x-bfe-skip-ai-cache: on`）→ `400` + `MODEL_NOT_ALLOWED`，且不打上游；
  ② 主断言：同问句不带 skip → `200` + `X-Bfe-Ai-Cache: hit`，`backend.Hits()` 不增加。
- **TC-13 `TestTC13_CacheHitSkipsRateLimit`**：专用 Key `ak_cache_rl` 绑定 rpm `max_requests=1`；
  ① 首请求 miss 消耗唯一额度；② 主断言：同请求再发 → `200` + `hit`，不出现 `429`，不打上游；
  ③ 对照（新问句 + skip 头）→ `429` + `RPM_LIMIT_EXCEEDED`，证明额度确实耗尽。
- **不做**：SC25 无需新增用例（其 `TestTC03` 覆盖命中日志口径，本次不改字段，断言保持）；`ai_rate_limit_hits` 为空这类日志断言留在 SC25/SC05（SC20 模板未加载 `mod_access_pb3`）。
- **未完成（需另开任务）**：集成测试仓的 `SC1401-TC028` 的 `A-05` 改写。该用例不在本工作区（`integration-test` 仓 HEAD `13ba26f` 内检索不到 `SC1401` / `TC028`），无法在此仓落地，需在承载 TC028 的仓里把"allowlist/限流在 Cache 之前"改为上述负向断言。

## 五、兼容性与行为变化

| 项 | 变化 | 影响面 |
|---|---|---|
| 命中时的 allowlist / 限流 / 配额 | **无变化**（本次只把口径写成条款） | 零 |
| 访问日志 schema | **无变化**（不新增 `ai_policy_skipped`；命中语义由 `ai_cache_status` 派生） | 零；`bfe-access-pb` 不动 |
| `ai_cache_status` / `ai_target_model` / `X-Bfe-Ai-Cache` / `ai_rate_limit_hits` | 无变化 | 零 |
| 模块注册序 / 配置项 / Redis 键格式 / 数据面代码 | 无变化 | 控制面无需同步 |
| 错误码 | 无新增 | 零 |

**不变量清单（回归锚点）**：`X-Bfe-Ai-Cache` 取值、命中响应 header/body 形态、SSE 模板、`ai_cache_status` 四值语义、`ai_cache_key` 调试字段、`maxBodyBytes`/`maxValueBytes`、`x-bfe-skip-ai-cache`、TTL 与 `CacheKeyPrefix`、`AiCacheHit` 驱动的跳过扣减。

## 六、边界与残余风险

- **命中 QPS 无租户级约束**：同一问句可以无限次命中，既不消耗 rpm 也不扣配额。条款认下这一点的前提是"命中只花网关自己的资源"；若运营侧出现"用命中刷网关"的滥用，需要的是一条**新的**"命中 QPS 限制器"（或把命中计入 rpm 但不计费），而不是复用现有 rpm 语义。这是新的产品决策，本方案不覆盖。
- **策略变更滞后**：见 T-1433-3，最多一个 TTL 窗口内旧策略仍可通过缓存"生效"。运维需知：收紧 allowlist / 绑定限流后若要求立即生效，可清空对应 `ai_cache:<key_id>:*` 键（当前数据面没有删除接口，只能等 TTL 或由运维手工清理）。
- **跨模型内容混用（独立缺陷，未闭合）**：`buildCacheKey`（`request.go:222-233`）只哈希 question，语义侧隔离单元只有 `tenant_id`（`provider/vector/chroma.go:129-147`）。同一 Key 下问同一句话但 `model` 不同会共享条目，可能返回另一个模型产出的内容。这与 allowlist 无关，是正确性问题，建议单独立项（§附录 B）。
- **控制面缓存规则顺序**（issue 的附带观察）：`PUT /open-api/v1/ai-cache-rules` 返回的数组顺序与数据面实际匹配顺序不一致；数据面按加载顺序取第一条 `Cond.Match` 成立的规则，顺序语义完全取决于下发数组。该问题在 `ai-gateway-api` / `conf-agent` 侧。
- **跨仓依赖**：只有测试与文档跨仓——TC028 在 `integration-test` 仓，控制面说明在 `ai-gateway-web` 仓。数据面与 `bfe-access-pb` 本变更零改动，可独立发布。

## 七、实施前待定项（非产品语义问题）

1. ~~**允许模型是否仅用于成本/配额治理？**~~ → **已确认：是（2026-10-20，见 §3.3）**，T-1433-1~5 生效。
2. ~~`ai_policy_skipped` 字段与 `bfe-access-pb` 联改~~ → **不做**（§4.1，由 `ai_cache_status` 派生）。
3. T-1433-3"策略变更为后生效、滞后上界 = TTL"是否随交付一起告知运维/前端（不阻断实施，属发布说明问题）。

## 八、决策与实施记录

| 日期 | 事项 | 结论 |
|---|---|---|
| 2026-10-20 | 允许模型的目的（§3.3） | **仅用于成本/配额治理** → 命中不执行 allowlist/限流/配额是预期行为，条款 T-1433-1~5 生效 |
| 2026-10-20 | 附录 A（命中仍校验） | 不启用，存档备用 |
| 2026-10-20 | `ai_policy_skipped` 字段 | **不做**：零增量信息、会与 `ai_cache_status` 形成双事实来源、跨仓 proto 属永久维护成本（§4.1） |
| 2026-10-20 | 数据面代码 | **不改**（本变更 = 条款 + 文档 + 测试断言） |
| 2026-10-20 | §4.2 文档落地 | **已完成**：`ai_cache.md`（§4 流水线标注 + 新增 §9.1 条款）、`ai_access_log_fields.md`（命中策略语义与 `ai_target_model` 口径）、`ai_rate_limit_redis_key.md`（新增 §6.4 命中不计量）、`2026-10-02` §5 补注、`ai-gateway-web` §8.5 与 §10.7 |
| 2026-10-20 | §4.3 测试 | **已完成**：SC20 新增 `TestTC12_CacheHitSkipsModelAllowList` / `TestTC13_CacheHitSkipsRateLimit` + 两份 TC 设计文档 + 场景说明登记；`scenario-SC20` 13 条全绿（7.9s）、`scenario-SC25` 6 条全绿、`go vet ./tests/integration/...` 通过 |
| 2026-10-20 | TC028 `A-05` 改写 | **未完成**：`SC1401-TC028` 不在本工作区（`integration-test` 仓 `13ba26f` 检索无 `SC1401`/`TC028`），需在承载该用例的仓另开任务 |

落地路线（按 `bfe-rd-workflow`）：Phase 1（本文档）→ Phase 2（无配置项变更，跳过）→ Phase 3（`sys_design` 文档同步 ✅）→ Phase 4（无代码改动，跳过）→ Phase 5/6（集成测试设计文档 + 用例 ✅）→ Phase 7（回归 ✅）→ Phase 8（收尾，Git 提交需人工确认）。

关闭 issue #1433 的判定条件（三项齐备）：①本文档（条款 T-1433-1~5）落库 ✅；②`SC1401-TC028` 的 `A-05` 由 deferred 改为负向断言（§4.3，⏳ 待承载仓执行）；③控制面文档补"命中不执行模型访问控制/限流、不计量" ✅。第②项是唯一未闭合项。

---

## 附录 A：备选方案（仅当条款改判为"命中仍须校验 allowlist"时启用）

保留自本文档前一稿，供 §3.3 判定为"允许模型含非成本含义"时直接取用。核心难点是命中时 target model 不可知（§2.2），因此必须**让条目自己携带产出模型**：

1. `bfe_basic`：新增 `AiModelPolicy{Allow,Block}` 与 `Permits/Blocked/Allowed`（判据与 `ValidateTargetModel` 逐字一致），由 `SetTokenAuthContext`（`mod_ai_token_auth.go:829`）在 FoundProduct 发布到 `AiBasicInfo`；
2. `tenantOf`（`request.go:238`）在 Key 配置了 allow/block 时返回 `<key_id>|<client_model>`，使受限 Key 拥有独立键空间（精确键与语义 `tenant_id` 同串）；
3. 受限键空间的值改为信封 `v1:{"a":<answer>,"m":<target_model>}`（写入点 `response.go:90`，模型取 `aiInfo.TargetModel`）；语义侧在 chroma metadata 增 `target_model`（`chroma.go:204` 写、`:149` 读）；`maxValueBytes` 仍只量答案；
4. 命中前准入：`modelPolicyPermits(req, entryModel)` 不通过 → **降级为未命中**（`serveMiss` 复用未命中尾巴），由既有链路给出 400，不在缓存侧构造错误；
5. 命中日志把 `ai_target_model` 改为条目模型（会改动 SC25 `TestTC03` 的断言）；
6. 残余近似：准入判定用"条目模型"而非"本次请求将转发的模型"，二者在客户端模型与集群映射配置不变时等价；策略变更滞后仍为 TTL。

该方案不改变不受限 Key 的任何行为，且因键空间是新增的而回滚安全；成本是全局多一次约束与一整套键空间/格式改动。

## 附录 B：关联缺陷（建议单独立项，与本条款解耦）

**缓存键不含模型**：同一 Key、同一问句、不同 `model` 的请求共享条目（精确与语义皆然），可能返回另一个模型产出的内容，表现为"请求了模型 B 却拿到模型 A 的答案"。最小修复是把客户端模型纳入键/租户维度（对所有 Key 生效时需升键前缀隔离存量条目），与 allowlist 是否执行无关。