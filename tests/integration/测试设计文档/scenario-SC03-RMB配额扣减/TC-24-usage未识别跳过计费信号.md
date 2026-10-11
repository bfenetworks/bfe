# TC-24 usage 未识别跳过计费信号（issue #1410）

## 用例编号与名称

TC-24 usage 未识别跳过计费信号（2xx 但 usage 未识别且不可估算 → 零扣费 + `UsageUnrecognized` 计数 + `ai_usage_source=unrecognized`）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1410 修复，需 `bfe-access-pb >= v0.3.15` 的 `ai_usage_source` 字段）
- 依赖 `TestEnv` 改造：捕获 BFE monitor 端口（`StartBFE` 第二个返回值），新增读取 `/monitor/mod_ai_token_auth` 模块计数器的辅助方法

## 测试目的

验证 issue #1410 的观测性语义：当请求返回 2xx、但上游 usage 从未被识别（无 final usage）且不满足估算计费条件时，计费口径**保持既定行为**（计费字段清零、RMB 零扣费），同时必须产生可自动检测的产品侧信号：

1. `mod_ai_token_auth.UsageUnrecognized` 模块计数器自增（可经 monitor 接口读数、可配告警）；
2. 访问日志标记 `ai_usage_source=unrecognized`（与"真实零用量"`parsed`、"估算计费"`estimated` 三态可区分）。

同时验证区分性（对应 issue 判据要求）：

3. **真实零用量对照臂**：后端返回 `completion_tokens=0` 的合法 usage（`prompt=100, completion=0, total=100`，#1398 起该形态被认可为 final usage）——解析成功、按输入价扣减、**不计入** `UsageUnrecognized`，日志 `ai_usage_source=parsed`——证明已识别流量（即使含零值分量）不会被误计为计费跳过；
4. **正常计费对照臂**：后端返回正常 usage，扣减金额与 TC-01 一致，`ai_usage_source=parsed`，计数器不增。

**已知限制（诚实边界）**：usage 对象**全部计数为 0**（`prompt=0, completion=0, total=0`）的响应在识别层与"无 usage"不可区分——`isguess` 判定为值-based（`UsedQuota > 0`，`body_process.go` GetQuotaUsage 与 `tokenReadResponseHandler` 的 `UsedQuota > 0` 门槛同构），此类行会落入 `unrecognized` 计数。该形态计费为 0 属正确结果，仅分类语义受限；识别层 presence 检测（按 usage 键存在性而非数值判定）是独立的后续改造，不在 issue #1410（观测性）范围内。

修复前：守卫跳过分支全链零信号（无计数器维度、无 Warn），访问日志只能以 `ai_total_tokens=0` 表达、与真实零用量同形，漏计费只能靠带外人工对账发现（生产实测单日 95.5% 缺口）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件（含 issue #1410 修复）。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. SC03 测试环境的 `bfe.conf` 模板 `MonitorEnabled = true`（默认已满足）；`TestEnv` 需捕获 monitor 端口并提供计数器读数辅助方法。
4. SC03 `bfe.conf` 中 `Server.EstimateToken` 取默认值 **true**（`conf_basic.go` 默认）——因此复现臂必须构造"不可估算"形态：chunked + 非 SSE + 非 JSON 的 200 响应（`text/plain`）。此类响应既不被 `tokenReadResponseHandler` 整体解析（#1406 的 `isCompleteNonStreamBody` 仅认可定长或 chunked 非 SSE JSON），`mod_body_process` 在纯文本中也不产生终止事件 → `responseCompleted=false` → `estimateBillable=false` → 落入"usage 未识别且不可估算"分支。若用可整体解析的 JSON body，则会被 EstimateToken 估算计费（`source=estimated`），无法命中本分支（首次实现即此教训）。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`；`cluster_rmb` 价格表为 `defaultRMBAIConf`（`deepseek-chat`：`input 1e-6`、`output 2e-6`，定点 100/200）。

## 请求构造

同一 Key 依次发送 3 次 POST `http://rmb.example.org/v1/chat/completions`（`Authorization: Bearer ak_user_a`，body `{"model":"deepseek-chat"}`）：

| 臂 | 后端响应（均 HTTP 200） | 预期 |
|---|---|---|
| 未识别复现臂 | chunked（`NoContentLength`）+ `Content-Type: text/plain`，body `hello, no usage here` | 零扣费；`UsageUnrecognized` +1；日志 `ai_usage_source=unrecognized` |
| 零输出对照臂 | 定长 JSON：`{"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}` | 按输入价扣减 `100*100 = 10000`（#1398 认可零输出 final 事件）；计数器**不增**；日志 `ai_usage_source=parsed` |
| 正常计费对照臂 | 定长 JSON：`{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}` | 扣减 `100*100 + 50*200 = 20000`；计数器不增；日志 `ai_usage_source=parsed` |

## 后端响应

mock 后端 `cluster_rmb` 按臂切换 body（复用 `MockBackend.Body` 赋值模式，同 TC-23）；三次均返回 HTTP 200。

## 执行步骤

1. 按 SC03 公共流程启动嵌入式 Redis、mock 后端与 BFE（捕获 monitor 端口）。
2. 预置 `quota:plan_rmb = 10000000000`。
3. 臂 1（未识别）：切换后端 body 为无 usage 响应 → 发送请求 → 断言 200、后端命中 1 次。
4. 读取 `/monitor/mod_ai_token_auth` 模块计数器：断言 `UsageUnrecognized == 1`（其余臂执行期间持续断言）。
5. 臂 2（零输出对照）：切换 body 为 `completion_tokens=0` 的 usage（#1398 认可为 final usage）→ 发送请求 → 断言 200、计数器仍为 1。
6. 臂 3（正常计费）：切换 body 为正常 usage → 发送请求 → 断言 200、计数器仍为 1。
7. 等待异步扣减（500ms）后断言 `quota:plan_rmb` 余额 = `10000000000 − 10000 − 20000 = 99999970000`（仅臂 2/3 扣减；臂 1 零扣费为既定口径）。
8. BFE 日志中出现 1 条 `usage unrecognized and not estimatable, billing skipped` Warn（含 cluster/key_id/model）。**实现说明**：测试框架 `StartBFE` 将 BFE 进程 stdout 直挂测试进程控制台、且本场景 `bfe.conf` 无 `[log]` 段（不产生 exception.log 文件），Warn 无法从文件断言；Warn 与计数器在同一守卫体内同源发出，集成层以计数器为信号基准断言，Warn 由运行日志人工可见 + 单元测试覆盖守卫体内代码路径。
9. 访问日志 `ai_usage_source` 字段断言：SC03 未加载 `mod_access_pb3`，此断言由单元测试 `TestReqAiInfoGenUsageSource` 与 SC05 场景覆盖；本 TC 在集成层断言计数器与扣减金额。

## 预期结果

- 三次请求均返回 HTTP 200；`cluster_rmb` 共 3 次命中。
- `UsageUnrecognized` 计数器：臂 1 后为 1，臂 2、臂 3 后保持 1（区分性核心断言：已识别流量——即使 completion 为 0——不会污染计费跳过计数）。
- Redis 余额 `99999970000`（零输出臂扣 10000、正常臂扣 20000；未识别臂零扣费是 issue #1410 明确维持的既定口径，本 TC 只断言其**可观测**，不主张其应计费）。
- 运行输出中可见 1 条计费跳过 Warn（`response_completed=false allow_estimate=true`）；文件级断言受测试框架限制（见执行步骤 8），以计数器为集成层信号基准。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
