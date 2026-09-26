# AI Key Session Affinity Redis 所有权优化：bfe_server 自持连接，移除向模块借句柄

- 类型：架构优化（依赖方向修正），独立小卡；来源：#1387 姊妹卡实施中的评审发现（`docs/zh_cn/modifications/2026-09-23-issue-1387-sister-rate-limit-models-target-model/` §遗留）
- 代码库：`bfe/`（行号基于姊妹卡合并后 HEAD，即含 `HandleAfterAITargetModel` 回调点与 SC19 的版本）

## 一、问题（架构层面）

### 现状

AI Key 会话保持（session affinity，SC10 特性）是 `bfe_server` 的核心转发逻辑：选 key、key 轮换、失败分类、惩罚与重试状态机全部在 `reverseproxy.go` 的 `aiClusterInvoke`（`:1660-1800` 一带）及其 helper（`clientKeySessionID`/`redisSetBinding`/`redisDeleteBinding`/`setKeyPenalty`/`chooseAIKeyWithAffinity`，`:2041-2200`）。该逻辑需要 Redis 存取会话绑定与惩罚标记，但连接句柄当前**向模块借用**：

```go
// bfe_server/reverseproxy.go，aiClusterInvoke 内（约 :1698-1704）
var redisClient redis_client.Client
if policy.SessionAffinity && srv.Modules != nil {
    if module := srv.Modules.GetModule(mod_ai_rate_limit.ModAiRateLimit); module != nil {
        if m, ok := module.(*mod_ai_rate_limit.ModuleAiRateLimit); ok {
            redisClient = m.RedisClient() // 借用限流模块的 Redis 连接池
        }
    }
}
```

### 为什么是错误的依赖方向

- **资源所有权倒置**：session affinity 是核心转发能力，Redis 是它的基础设施；正确方向是「模块向核心要资源」，现状却是核心反向依赖 `mod_ai_rate_limit` 的**包常量 + 具体类型断言 + 导出方法**，`bfe_server` 因此无法移除对 `mod_ai_rate_limit` 的 import；
- **隐式配置耦合**：affinity 是否可用，取决于限流模块是否加载、其 Redis 配置（`mod_ai_rate_limit.conf` 的 `[Redis]` 段）是否可用——两个无关特性的可用性被绑在一起；不加载限流模块（或限流 Redis 配置变更）会静默改变 affinity 行为；
- **借用的理由已消失**：当初借用只是因为「模块恰好有一个配好的 Redis」。这不是架构理由。

### 备选方案（不采纳）

| 备选 | 不采纳理由 |
|------|-----------|
| 能力接口（`interface{ RedisClient() ... }` + 按能力查找） | 只是给倒置加上类型抽象，依赖方向没有纠正，属于包装而非修复 |
| 新增回调点供给资源 | 回调点是请求生命周期通知（req 进、状态出），不是资源供给通道；为借一个句柄新增回调点属机制误用 |
| affinity 逻辑整体迁入 `mod_ai_rate_limit` | 等于把核心转发状态机（选 key/轮换/惩罚）搬进功能模块，是 SC10 特性级重构，改动与回归面远超本卡目标 |

## 二、目标

1. `bfe_server` 自持 AI key affinity 专用 Redis 连接（自建、自配、自销毁），彻底移除 `reverseproxy.go` 对 `mod_ai_rate_limit` 的 import 与类型断言；
2. affinity 的可用性只由自己的配置决定，与限流模块的加载/配置解耦；
3. 默认零行为变化：未配置时保持 fail-open（affinity 静默不生效），与现行「模块缺失拿不到句柄」的行为一致；
4. 回归通过：SC10（affinity 本体）、SC02（key 轮换）、SC19（限流姊妹卡场景）+ 全量 `make test`。

## 三、方案

### 步骤 1（必改）：`bfe.conf` 新增配置段

仿 `SessionCache` 段的既有模式（`bfe_config/bfe_conf/bfe_config_load.go:21-33`：`BfeConfig` 加字段 + `SetDefaultConf` 默认值 + `Check` 校验）：

- 新增 `bfe_config/bfe_conf/conf_ai_key_affinity.go`：`ConfigAIKeyAffinity` 结构，字段镜像 `redis_client.Options`（`ServiceConf`（Bns 名字服务）或 Addr、`MaxIdle`、`MaxActive`、`ConnectTimeoutMs`、`ReadTimeoutMs`、`WriteTimeoutMs`、`Password`）；默认 `Disabled = true`（未配置段时 affinity 不启用，fail-open）；`Check` 做范围校验（仿 `conf_session_cache.go` 的校验风格）；
- `BfeConfig` 增 `AIKeyAffinity ConfigAIKeyAffinity` 字段并接入 `SetDefaultConf`；
- `conf/bfe.conf` 样例追加注释掉的示例段（`make package`/Docker 默认可用）；
- 配置文档：`docs/zh_cn/configuration/`（bfe.conf 章节）补段说明。

### 步骤 2（必改）：`BfeServer` 自持 client，生命周期入核心

仿 `initTLSSessionCache`（`bfe_server.go:240-246`）的先例：

- `BfeServer` 结构新增 `AIKeyAffinityRedis redis_client.Client` 字段；
- StartUp 流程新增 `initAIKeyAffinityRedis()`：`Config.AIKeyAffinity.Disabled == true` → 字段留 nil（fail-open）；否则用 `redis_client.NewRedisClient(options)` 创建；
- shutdown 路径关闭该 client（Redis client 的连接池随进程退出回收，若无显式 Close 接口则文档注明依赖进程退出，与模块侧行为一致——实施时核实 `redis_client.Client` 是否有 Close）。

### 步骤 3（必改）：`aiClusterInvoke` 调用点切换

`reverseproxy.go` 约 `:1698-1704` 的 `GetModule` + 类型断言块整段替换为：

```go
// Session affinity state (binding/penalty) is core forwarding logic; the
// Redis handle is owned by the server (see initAIKeyAffinityRedis), not
// borrowed from any module. Unconfigured (nil) means affinity is disabled.
redisClient := srv.AIKeyAffinityRedis
```

- 删除 `mod_ai_rate_limit` import（至此 `bfe_server` 对该模块依赖归零）；
- helper 函数签名不变（本就接收 `redis_client.Client`）；nil client 的下游行为与现行一致（`chooseAIKeyWithAffinity` 等在 nil client 时跳过绑定逻辑——实施时逐点核实并补注释）。

### 步骤 4（回归测试）

1. 配置加载单测：仿 `conf_session_cache_test.go` 新增 `conf_ai_key_affinity_test.go`（默认值/字段解析/Check 边界）；
2. `bfe_server`：初始化单测（disabled→nil；enabled→非 nil；无法连接时不阻塞启动——行为以 `redis_client` 惰性连接语义为准，实施时核实）；
3. 集成回归：**SC10**（session affinity 本体，需给其 testdata 的 `bfe.conf` 补上新段——enabled 场景绑定/惩罚断言）、**SC02**（key 轮换）、**SC19**（限流姊妹卡，验证去除借用后其场景不依赖模块 Redis 复用）；
4. `cd bfe && make test` 全量。

### 步骤 5（文档与发布）

- `docs/zh_cn/configuration/` bfe.conf 章节 + `docs/zh_cn/operation/` 相关运维文档补配置说明；
- **上线注意**：`bfe.conf` 由 conf-agent 分发——新配置段要纳入控制面下发需 conf-agent 联动（另卡）；手工维护 `bfe.conf` 的部署本卡即可生效；
- 版本公告：affinity 的 Redis 配置来源从「限流模块配置（隐式）」变为「bfe.conf 独立段（显式）」，存量启用 affinity 的部署升级时需迁移配置，否则升级后 affinity 静默关闭（fail-open，不报错但绑定失效）。

## 四、行为变化清单

| 项 | 改动前 | 改动后 |
| --- | --- | --- |
| Redis 句柄来源 | 向 `mod_ai_rate_limit` 借用（模块未加载/未配置 → nil → affinity 静默失效） | `bfe_server` 自持（`bfe.conf [AIKeyAffinity]` 段，未配置/disabled → nil → affinity 静默失效） |
| `bfe_server` → `mod_ai_rate_limit` | import + 常量 + 类型断言 | 零依赖 |
| affinity 与限流模块的耦合 | 可用性隐式依赖限流模块加载与其 Redis 配置 | 完全解耦，各自独立配置 |
| 连接池数量 | 与限流共用一池 | 各持一池（指向同一实例时为两个池；见 §五后续方向） |
| 显式配置 affinity + 未配新段 | 正常生效（借用限流 Redis） | **升级后静默失效**（fail-open）——需配置迁移，版本公告必含 |

## 五、边界与非目标

- **不改**：`mod_ai_rate_limit`/`mod_ai_token_auth` 的模块级 Redis（限流计数器、配额各自独立演进）；affinity 的状态机逻辑与 Redis key 结构（绑定/惩罚 key 格式不变，数据兼容）；SC10 确立的 affinity 语义（绑定优先级、惩罚 TTL 等）。
- **非目标：服务器级共享 Redis 连接管理器**。长期方向是核心统一管理各 Redis 连接池、模块反向从核心获取（顺带消掉两个 AI 模块各自的 `[Redis]` 重复配置），但牵动模块配置面与热加载语义，另立卡评估。
- **fail-open 是有意保留**：未配置即静默不启用 affinity，与现行 nil-client 行为一致；不做启动强校验报错（避免无 affinity 需求的部署被强制配置）。
- 多实例部署下 affinity 数据本就在共享 Redis，连接归属变化不影响一致性模型。

## 六、验收标准

1. `reverseproxy.go` 不再 import `mod_ai_rate_limit`，无 `GetModule` 借用；
2. `bfe.conf` 新段未配置时全部既有场景（含 SC10 未配新段的用例）行为不变（fail-open）；
3. SC10 补充 enabled 配置后绑定/惩罚断言全绿；SC02/SC19 回归全绿；`make test` 通过；
4. 配置文档与版本公告（配置迁移提示）齐备。

## 七、延伸评估：模块级 Redis 统一（评审结论 2026-09-23）

问题：`mod_ai_rate_limit` / `mod_ai_token_auth` 各自的模块级 Redis（`mod_ai_rate_limit.conf` / `mod_ai_token_auth.conf` 的 `[Redis]` 段）能否统一改用 `bfe.conf` 配置的 Redis？

结论：**统一「配置来源」值得做；统一「运行句柄」不推荐**。关键事实与理由：

1. **硬障碍——模块读不到 `bfe.conf`**：模块 `Init(cbs, whs, cr)` 不接收 `BfeConfig`；共享句柄须改框架（非破坏式可选接口 `InitWithDeps(deps)`，类型断言探测，仅需要共享的模块实现）；
2. **热加载语义倒退**：模块 `[Redis]` 今天可 `/reload/mod_xxx` 热更新（换 Redis 地址不断流）；`bfe.conf` 是启动 conf（`docs/zh_cn/operation/reload.md` 可热加载清单不含它），统一后 Redis 变更必须重启进程；
3. **池共享的噪音邻居**：三个消费者画像不同——限流计数器（高频 INCR、可容忍抖动）、token_auth 配额扣减（鉴权热路径、低延迟敏感）、affinity（低频绑定读写）；共享单池会让限流 burst 耗尽连接拖累鉴权路径；
4. **`mod_ai_token_auth` 确有独立 Redis**：`conf_mod_ai_token_auth.go:32-41` + `mod_ai_token_auth.go:392-406`，用于配额扣减 `plan.Deduct`（`:297/:304`）。

推荐分层架构（统一配置源、不统一句柄）：

```
bfe.conf [Redis]（全局端点/凭据，唯一配置源）
   │ 框架经 deps 传给模块
   ▼
各模块 conf 的 [Redis] 段保留但可省略：
  写了 → 模块级覆盖（保留热加载能力）
  省略 → 回落全局默认（仍建独立命名池，不共享句柄）
```

交付路径：

- **零 BFE 改动（立即可做）**：conf-agent 从单一 Redis 配置源把参数渲染进各模块 conf——「配置统一」在控制面立即实现；
- **BFE 侧小卡**：框架 `InitWithDeps` + 模块 `[Redis]` 省略回落全局默认；
- **长期终态**：服务器级 Redis 连接管理器（命名池），affinity/限流/配额均为消费者——届时本卡的 `[AIKeyAffinity]` 段随之并入管理器，不必先行建设。

