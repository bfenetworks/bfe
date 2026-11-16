# provider 级补偿性请求归一化（首例：`AIConf.EnsureReasoningContent`）

## 1. 背景与目标

### 1.1 背景

上游大模型服务偶有"契约不自洽"的行为：自己产出的响应与自己的请求校验互相矛盾，导致**忠实镜像上游响应的客户端必然违规**。当前首个实例是思考模式的 `reasoning_content` 校验：

- 上游要求 chat 请求中"最后一条 `user` 消息之后"的 assistant 消息必须携带 `reasoning_content`（缺字段或 `null` 即 400）；
- 同一上游又有相当比例的回合不产出 reasoning 文本（流式首帧 `""`、末帧 `null`）；
- 于是凡是"把上游响应原样回传"的客户端，在下一轮就可能 400；路由从一个 provider 切到另一个 provider 时，首请求更是必然踩中（当前轮 assistant 由别的后端生成，根本不含该字段）。

这类问题有三个特征，决定了修复位置：

1. **作用维度是 provider（上游实体），不是协议、不是模型名**：同一 provider 下所有思考模式模型共用同一条校验；客户端经 ModelMapping 传入的模型名多样（含别名），按模型清单收敛必然漏配。协议维度也不够：同一 provider 可能同时提供 openai 与 anthropic 两种入口，两条路径都要补；
2. **客户端无法单方面合规**：镜像上游是最合理的实现，错的是上游契约；
3. **网关有能力单点收口**：网关同时看到双方报文，且是唯一知道"这一轮实际去了哪个 provider"的一跳。

### 1.2 目标

1. 把"上游契约不自洽 + 客户端无法单方面合规"这一类问题收敛成**一套受控机制**，避免每次出现同类问题都在请求链路里加一段无归属的 if；
2. 机制由 **cluster 级配置**驱动，默认关闭、按集群灰度、清空即回滚；
3. 改写只动必要的字节，任何异常 fail-open，绝不引入新的请求失败；
4. 自带可观测指标与明确的退出条件，上游修复后可以整块下线。

### 1.3 非目标

- 不修客户端实现，也不要求客户端配合；
- 不改变上游生成行为（不注入 prompt、不强制上游产出字段）；
- 不把这类补丁写进协议适配层（`bfe_model_protocol`）——协议层承载"协议知识"，不是"厂商怪癖"，边界见第 2 节；
- 不做响应侧改写。

## 2. 设计原则

### 2.1 准入红线

只有同时满足下面三条，才允许新增一个补偿性归一化：

| # | 判据 | 反例（应拒绝） |
|---|---|---|
| 1 | 上游契约自相矛盾：遵循协议/文档的正确实现会失败 | 客户端把 `tool_calls` 拼错，属于客户端 bug |
| 2 | 客户端无法在自身范围内合规（缺少"下一个后端是谁"的信息，或镜像行为本身合规） | 客户端漏传必填字段，客户端自己就能修 |
| 3 | 网关能在不改变服务语义的前提下做**最小**修正（补默认值/补空值），而不是猜测业务意图 | 需要网关生成内容、重排会话、裁掉用户消息 |

判据 1+2 回答"要不要兜底"，判据 3 回答"兜底到什么程度"。任一条不满足，退回到客户端修复或上游工单。

### 2.2 层次边界：provider 能力 ≠ 协议知识

BFE 的 AI 侧已有明确分层（见 `model_protocol_adapter.md`）：`model_protocol` 承载协议知识（认证头、路径形态、usage 字段、错误格式），provider 是用户在 cluster 上声明的上游实体。补偿性归一化属于"这个 provider 有这个问题"，因此：

- **落在 cluster 的 `AIConf` 上**，以开关形式表达；
- **不落在 `bfe_model_protocol`**：否则每多一家厂商的怪癖，协议层就多一个 `if provider == "xxx"`，协议知识被污染；
- **不落在 `mod_body_process`**：该模块在 `HandleAfterLocation` 执行，此时目标 cluster 尚未确定（cluster 是在 AI 转发循环里逐个 attempt 选出的），而条件引擎没有 cluster 原语，只能退化成"按模型名猜 provider"。

### 2.3 三条硬约束

1. **默认关闭**：字段缺省即不生效，存量配置无需变更，未开启的流量与旧版本逐字节一致；
2. **fail-open**：解析失败、结构异常、无法完整读取请求体，一律原样放行，只记指标；
3. **只增不改**：注入采用定点写入（sjson），不整体重排/重新序列化，字段顺序、数字格式、Unicode 转义保持原样——上游对请求体的其它解析不受影响。

## 3. 机制模型

```text
请求 ──► 路由选定 cluster（attempt）──► 若命中该 cluster 的补偿开关
                                        │
                                        ├─ 协议门禁：本次请求协议是否在开关的协议数组内
                                        ├─ 形态门禁：batch/file、非 JSON、压缩体、超限、解析失败 → 原样放行
                                        └─ 注入：按协议在请求体的既有结构里补最小默认值
                                        ▼
                                      转发到上游
```

三个正交维度，缺一不可：

| 维度 | 承载方式 | 为什么不能省 |
|---|---|---|
| **哪个 provider** | cluster 的 `AIConf.<开关>` | 故障属性所在；同 provider 的所有模型自动覆盖，无需维护模型清单 |
| **哪种协议** | 开关取值为协议名数组 | 同一 provider 的多协议入口结构不同、注入落点不同（`messages[].reasoning_content` vs `content[]` 的 thinking 块） |
| **哪次尝试** | 在 `doSingleAIForward` 内执行 | fallback/Key 轮换会换 cluster，开关必须按 attempt 判定 |

与既有同类机制的关系：

| 机制 | 位置 | 与本机制的关系 |
|---|---|---|
| `AIConf.NormalizeUpstreamError` | 转发结束后，改写错误响应 | 同一"provider 级开关 + AI 转发路径"范式，作用在响应侧 |
| `AIConf.ProtocolPaths` | `doSingleAIForward`，请求前 | 同位置；但它是**协议知识**（SDK base_url 形态），故按协议配置而非 provider 开关 |
| `Server.InjectStreamUsage` | `doSingleAIForward`，请求前 | 同形态的"缺字段注入"，但是全局开关（无 provider 差异），故不按 cluster 配置 |
| 本机制 | `doSingleAIForward`，请求前 | provider 开关 + 协议数组 + 定点注入 |

## 4. 配置模型

### 4.1 `AIConf` 扩展

```go
// bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go
EnsureReasoningContent []string
```

### 4.2 加载期校验（`AIConfCheck`）

| 规则 | 说明 |
|---|---|
| 取值 | 仅 `openai`、`anthropic`——本期实现了注入分支的协议。`gemini` **不适用**：其请求体是 `contents[].parts[]`，没有 `messages` 也没有 `reasoning_content`，注入无落点；其思考内容回传是 `thought`/`thoughtSignature` 机制，空串解决不了。配置即报错，避免"配了但什么都不做"的假开关 |
| 与本 cluster 协议一致 | 每项须满足 `modelprotocol.Supports(ModelProtocols, p)`；`ModelProtocols` 为空按 `["openai"]`（与 `Supports` 的既有语义一致） |
| 空 | 合法，表示关闭 |

### 4.3 语义

| 协议 | 注入内容 | 落点 |
|---|---|---|
| `openai` | `"reasoning_content": ""`（"本轮无思考过程"，即上游自己首帧可能发送的原值） | `messages[i]`（i 在"最后一条 `user` 之后"的窗口内） |
| `anthropic` | `{"type":"thinking","thinking":"","signature":""}` | assistant 消息的 `content` 数组头部（已有 thinking 块则不插） |

## 5. 处理逻辑

### 5.1 执行点

`bfe_server/reverseproxy.go` 的 `doSingleAIForward`，位于 `HandleAfterAITargetModel` 回调与 model 重写之后、`AIConf` 应用块附近。顺序是硬约束：

- 必须晚于 `HandleAfterAITargetModel` 回调：`mod_ai_context`（若启用）在回调内整体 re-marshal 请求体，其消息序列化会丢弃空 `reasoning_content`，跑在它前面会被抹掉；
- 必须晚于 model 重写：后者以 `ReqBodyJsonSet` 改写请求体并重置 `Content-Length`。

### 5.2 门禁顺序

| # | 门禁 | 处理 |
|---|---|---|
| 1 | 开关为空 / 本次协议不在数组内 | 直接返回 |
| 2 | `mode != chat`（`file`/`batch` 计跳过，`/responses` 等不计） | 返回；file/batch 计 `ReasoningSkip` |
| 3 | 非 JSON `Content-Type` | 跳过 + `ReasoningSkip` |
| 4 | 请求体带 `Content-Encoding` | 跳过 + `ReasoningSkip`（压缩字节上做 JSON 改写会破坏请求体） |
| 5 | `GetBodyAccessor()` 失败，或 `GetBytes()` 的 `all=false`（超过 `Server.AccessibleBodySize`） | 跳过 + `ReasoningSkip`（缺省 2MB，上限 8MB） |
| 6 | 请求体不是合法 JSON（`gjson.ValidBytes` 前置校验） | 原样放行 + `ReasoningFail`（必须显式校验：gjson 对截断 body 仍能部分解析，sjson 会继续追加字段，把畸形请求改得更长） |
| 7 | 结构异常（`messages` 非数组、`content` 既非 string 也非数组） | 原样放行 |

### 5.3 窗口与幂等

- 窗口：从 `messages` 末尾向前找到最后一条 `role == "user"`，只处理其后的 assistant 消息；历史消息不动（上游只校验该窗口）；
- 幂等：只在字段缺失或为 `null` 时写入，已有非空值不动；因此在 fallback 重复进入同一请求体时不会二次改写，也不会出现重复键；
- 长度：改写成功后置 `ContentLength = -1` 并删除 `Content-Length`（同时重置原请求上的同名字段），保证重试/fallback 时新构造的 outreq 长度一致——与 model 重写段的处理相同。

### 5.4 性能

开关打开的 cluster 上，**每个请求**都要付出一次 body 缓冲 + 一次 JSON 校验 + 一次窗口扫描（未命中同样有此成本），命中时再加一次定点写入。实测（`BenchmarkPatchReasoningContentLargeBody`，500KB body，单核笔记本 CPU）：合计 ~2.5 ms，其中 `gjson.ValidBytes` ~1.0 ms、`sjson.SetBytes` ~0.7 ms、窗口扫描 ~0.8 ms；换算 MB 级约 5 ms。典型 chat 请求体量远小于此，成本可忽略；MB 级请求接近预算上限，若生产压测超标，优先优化 JSON 校验（阈值化或写入后校验）而不是去掉它——去掉会重新引入"畸形 body 被改得更长"的问题。

## 6. 可观测性与退出

| 指标（`proxy_state`） | 含义 | 用途 |
|---|---|---|
| `ReasoningPatched` | 本次 attempt 至少补了一条消息 | 效果验证；按 cluster 观察可判断该开关是否真的需要 |
| `ReasoningSkip` | 门禁 2/3/4/5 跳过 | 覆盖缺口；超限计数持续增长说明需调 `Server.AccessibleBodySize` 或接受盲区 |
| `ReasoningFail` | 解析失败或结构异常导致 fail-open | 健康度；突增说明请求结构变化或门禁过宽 |
| `ReasoningStill400` | 已补字段但仍返回 400 | **契约反向变更的唯一信号**：上游开始拒绝 `""`（改为要求非空或校验签名）时唯一能看到的迹象，非零应立即清空配置 |

**退出条件**：上游修复有两种形态，退出门槛必须对两者都成立——

- 形态 A：每回合都产出 reasoning → 补丁命中数自然归零；
- 形态 B：校验改为容忍缺失 → 补丁仍会命中（客户端依旧不发字段），命中数**不会**归零。

因此不以"补丁零触发"为准，而取"目标错误签名连续 N 天归零 **且** 上游确认已修 A 或 B"。满足后先清空配置（行为立即消失），再删除实现与字段。

## 7. 兼容性与发布

- 字段缺省 = 关闭：旧 `cluster_conf.data` / `cluster_table.data` 无需变更；未开启集群的请求与旧版本逐字节一致；
- 发布顺序：先全量部署含实现的二进制（字段全空，行为不变）→ 再逐个集群开配置；回滚只需清空该字段，无需回退二进制；
- 控制面（ai-gateway-api）在 cluster 的 `llm_config` 中暴露该字段并导出到 `cluster_conf.data`，写入时的值域校验与本文件 4.2 同口径，形成"控制面拦截 + 数据面加载校验"双层防线。

## 8. 新增同类补偿的接入指南

出现新的"上游契约不自洽"问题时，按此清单落地，避免临时加 if：

1. 用第 2.1 节三条判据确认是否该由网关兜底，并写进对应的 `modifications/` 文档；
2. 在 `AIConf` 增加一个**默认关闭**的开关（多协议差异明显时用协议数组，否则用 bool）；
3. 在 `AIConfCheck` 校验取值与 cluster 协议的一致性；不可用的取值直接拒绝；
4. 实现放在 `bfe_server/reverseproxy_ai_*.go` 的独立文件里，在 `doSingleAIForward` 中调用，遵守 5.1 的顺序约束；
5. 改写遵守 5.2 的门禁与 5.3 的幂等/长度处理；
6. 至少带三个指标：命中数、跳过原因、失败透传数；若该补偿"可能随时间失效"（上游会修复），再加一个"补偿后仍失败"的指标作为退出信号；
7. 同步更新中英文配置文档、`conf/` 样例（如适用）与本目录下的机制文档；
8. 单测覆盖门禁全分支 + 字节级"只增不改"断言；集成测试在 `tests/integration` 建独立场景。

## 9. 与相关文档的关系

| 文档 | 关系 |
|---|---|
| `docs/zh_cn/modifications/2026-11-03-ai-reasoning-content-normalization/design-changes.md` | 首个实例的实施记录（改动清单、单测、灰度、风险） |
| `docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md` | 字段级配置说明（类型、取值、合法性条件） |
| `docs/zh_cn/sys_design/model_protocol_adapter.md` | 协议层边界；本机制刻意不进入该层 |
| `docs/zh_cn/sys_design/ai_protocol_paths.md` | 同一执行点的相邻机制（协议路径改写），可对照其执行点与顺序说明 |
| `docs/zh_cn/sys_design/ai_error_codes.md` | 目标是让上游错误不再外泄；本机制从源头减少一类 400 |

## 10. 边界与开放问题

1. **`/responses`（Responses API）不处理**：其 reasoning 是 `input` 数组中的独立 item，没有 `messages[].reasoning_content` 落点，硬塞字段有污染风险；
2. **`gemini` 协议不适用**：结构无落点（见 4.2）；
3. **超大请求存在盲区**：请求体超过 `Server.AccessibleBodySize` 时无法完整读取，跳过改写；若线上确有此类请求命中该 400，再评估调大该配置（内存代价，上限 8MB）；
4. **与 `mod_ai_context` 的顺序依赖**：两者都在 AI 转发路径上，本机制必须在其回调之后执行；若将来调整顺序，必须重跑幂等用例；
5. **补丁的语义损失**：客户端丢弃了非空 `reasoning_content` 时，网关无法区分"本轮真的没有 reasoning"与"客户端丢了"，一律补 `""`。请求从硬失败变为可用，但该轮推理上下文丢失、cache 前缀可能变化——这是已知取舍，靠灰度期的 cache 命中率与输出 token 分布对比监控；
6. **是否要覆盖更多协议**：只有当某协议出现"必须回传思考内容"的同类契约、且其结构里存在落点时，才实现分支并放开取值；不做"提前支持"。
