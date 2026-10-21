# BFE Issue #1439 修复方案：Key 惩罚过滤未排除 Weight=0 导致"全集群无 Key"

- Issue: https://github.com/bfenetworks/bfe/issues/1439
- 生产事件编号：prod-20261021-01
- 缺陷：集群配置了 2 个 Key（`yonggang` Weight=100、`zhangmiao` Weight=0）且 `SessionAffinityPenaltyEnable=true`。唯一可用 Key 被上游 429 进入 60s 惩罚窗口后，`filterPenaltyKeys()` 只按"是否被惩罚"过滤、**不排除 Weight=0 的 Key**，导致过滤结果非空（仅剩 `zhangmiao`），不触发"全被惩罚则回退全量"的兜底；随后 `chooseNextAIKey()` 又因候选权重全为 0 返回 `ok=false`，`aiClusterInvoke()` 打印 `all ai keys exhausted` 后直接 `break`，**一次上游请求都没有发出**。1 个 Key 被限流被放大为整集群 60 秒不可用。
- 代码库：`bfe/` **v1.8.8 分支**（本文行号逐条核实于该分支）。
- 关联：#1440（Key 耗尽返回 nil 导致外层降级失效，与本缺陷叠加后 429 退化为 500）、#1442（单可用 Key 集群的容量前提）。三者共同构成 2026-10-21 生产事件。

## 一、根因（源码定点）

调用链（`bfe_server/reverseproxy.go`）：

1. `chooseAIKeyWithAffinity()`（`:2242`）的"单 Key 短路"（`:2259`）只对**物理上仅 1 个 Key** 生效：

   ```go
   // short-circuit: single valid key needs no Redis lookup/write.
   if len(keys) == 1 && keys[0].Weight > 0 {
       return 0, keys[0], keys[0].Name, true
   }
   ```

   本集群有 2 个 Key，短路不生效，进入惩罚过滤分支（`:2263-2267`）。

2. `filterPenaltyKeys()`（`:2199-2214`）只判断 `isKeyPenalized()`，**不判断 Weight**：

   ```go
   for _, k := range keys {
       if !isKeyPenalized(clusterName, k.Name, policy, client) {
           filtered = append(filtered, k)
       } else {
           skipped++
       }
   }
   if len(filtered) == 0 {
       // all keys are penalized; fallback to original list to avoid no key
       return keys, skipped
   }
   ```

   `yonggang` 被惩罚剔除、`zhangmiao`（Weight=0）未被惩罚 → `filtered=[zhangmiao]` 非空 → 回退分支不触发，返回单元素列表。注意 `Weight=0` 的 Key 在语义上**从不参与选 Key**（`chooseNextAIKey():2060-2062`、`isKeyAlive():2229` 都显式排除它），惩罚过滤器却把它当作"可用候选"计入，是口径不一致的直接根因。

3. `chooseNextAIKey()`（`:2055-2072`）先排除 `Weight==0`（`:2060-2062`），`eligible` 为空 → 返回 `(-1, AIKey{}, false)`。

4. `aiClusterInvoke()`（`:1829-1834`）：`!ok` → 打 WARN + `break`，函数以 `res=nil, lastErr=nil` 返回（`:1903`），本轮未调用 `doSingleAIForward`。

**结论**：惩罚过滤的候选集口径（是否被惩罚）与选 Key 的候选集口径（Weight>0 且未 dead）不一致。过滤器的职责是"在被惩罚的 Key 中做减法"，其输入应当已经是"可参与选 Key 的 Key"。

## 二、修复方案（方案乙：惩罚 = 临时熔断，2026-10-21 定案）

设计原则：

1. **`Weight=0` 的 Key 在任何环节都不是候选**——惩罚过滤不改变这一点，也不能因它而改变"是否有候选"的判定；
2. **惩罚 = 临时熔断**：被惩罚的 Key 在窗口内**严格剔除**，不做任何"回退到被惩罚 Key"的兜底；过滤后无候选即判定"本集群暂不可用"——选 Key 失败 → 由 #1440 的合成 429 触发 cluster 降级，窗口内与上游零交互，TTL 到期后自动恢复；
3. 修复范围最小化：只动过滤器的输入口径与兜底语义，不改 `chooseNextAIKey` / `isKeyAlive` 的既有语义。

### 步骤 1（必改，核心）：`filterPenaltyKeys()` 排除 Weight=0，并删除"回退全量"兜底

`bfe_server/reverseproxy.go`：

```go
func filterPenaltyKeys(clusterName string, keys []cluster_conf.AIKey, policy cluster_conf.AIKeyPolicy, client redis_client.Client) ([]cluster_conf.AIKey, int) {
    var filtered []cluster_conf.AIKey
    skipped := 0
    for _, k := range keys {
        if k.Weight == 0 {
            // Weight=0 keys are never eligible for selection; they must not
            // count as candidates (issue #1439).
            continue
        }
        if !isKeyPenalized(clusterName, k.Name, policy, client) {
            filtered = append(filtered, k)
        } else {
            skipped++
        }
    }
    if len(filtered) == 0 {
        // All usable keys are penalized (circuit breaker, issue #1439):
        // return no candidates so chooseNextAIKey fails and aiClusterInvoke
        // surfaces a synthetic 429 (issue #1440) for cluster fallback.
        // The old "fallback to original list" branch is removed on purpose:
        // penalized keys must stay excluded until the penalty TTL expires.
        log.Logger.Info("aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break", clusterName)
        return nil, skipped
    }
    return filtered, skipped
}
```

要点：

- `skipped` 计数语义保持为"被惩罚而跳过的 Key 数"（`ReqAiKeyAffinityPenaltySkip` 指标口径不变）；Weight=0 的排除**不计入** skipped（它不是惩罚跳过，是配置级不可用）；
- **删除原 `:2209-2212` 的"全部被惩罚则回退全量"分支**——这是方案乙与方案甲（回退到被惩罚 Key）的分歧点：回退会让被惩罚的 Key 在窗口内继续被使用，违背熔断语义。该分支也是原实现把 `Weight=0` 的 Key 塞回候选的元凶；
- 集群没有任何 Weight>0 的 Key、或全部 Weight>0 的 Key 都被惩罚时，统一返回空列表 → `chooseNextAIKey` 得到 `ok=false` → #1440 的合成 429 → cluster 降级或透传 429。惩罚恢复（TTL 过期）后过滤器自然放行，**无需任何状态清理**。

### 步骤 2（不实施）：单可用 Key 短路泛化

原方案的"单可用 Key 短路"（`usable == 1` 时直接返回、跳过 Redis 惩罚过滤）**不实施**。原因：短路与熔断语义直接冲突——它会让被惩罚的唯一可用 Key 绕过惩罚过滤继续被选中，熔断形同虚设。原"物理上单 Key"短路（`len(keys)==1 && keys[0].Weight>0`）保留现状：单 Key 配置下 `keys` 长度为 1，原短路仍会生效、同样绕过过滤——这是既有行为，方案乙不扩大也不缩小它，仅注明：**启用 `SessionAffinityPenaltyEnable` 的集群若长期只配置 1 个 Key，惩罚不会生效**（由 #1442 §二的配置建议兜底：单可用 Key 集群应关闭 `SessionAffinityPenaltyEnable`）。

### 步骤 3（可观测）

`ReqAiKeyAffinityPenaltySkip` 指标口径不变（仍为被惩罚跳过的 Key 数）。新增（已纳入步骤 1）：过滤器触发熔断（全部可用 Key 被惩罚、候选为空）时打一条 Info 日志 `aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break`，便于事后区分"熔断降级"与"正常选 Key 后被 429"，也可作为 #1442 §三监控告警的日志源之一。

## 三、回归测试

单元测试（`bfe_server/reverseproxy_ai_test.go`）：

1. `filterPenaltyKeys`：2 Key（1 个被惩罚的 Weight=100 + 1 个 Weight=0）→ 返回**空列表**（熔断：Weight=0 不计候选、被惩罚者被剔除），`skipped=1`；
2. `filterPenaltyKeys`：1 个 Weight=0 + 1 个未惩罚 Weight=100 → 返回 `[Weight=100]`；
3. `filterPenaltyKeys`：全部 Weight=0 → 返回空列表、`skipped=0`；
4. `filterPenaltyKeys`：2 个可用 Key **都被惩罚** → 返回空列表（熔断）、`skipped=2`——这是方案乙相对原实现的关键行为变化（原实现会回退全量）；
5. `chooseAIKeyWithAffinity`：`keys=[{a,100},{b,50}]` + `SessionAffinityPenaltyEnable=true` + 模拟 redis 对 `a` 返回惩罚键 → 选中 `b`；`a`、`b` 均被惩罚 → `ok=false`（熔断）；
6. 既有单测（绑定命中/失效、usedSet 轮转、`TestChooseAIKeyWithAffinity_*` 等）全部保持通过。

集成测试（依赖 Redis 的会话亲和场景，如 SC10；SC02 无 Redis 环境不适用本 TC）：

- 配置 2 可用 Key + `SessionAffinityPenaltyEnable=true` + 路由配置 fallback 到备用集群；mock 上游对 Key A 返回 429 → 断言：① 第一笔请求拿到 429 并触发 Key 轮换；② 惩罚窗口（60s）内后续请求**与目标集群零交互**（后端命中数不增），直接由 fallback 集群以 2xx 响应；③ `bfe.log` 出现 `aiKeyAffinity: all usable keys penalized ... circuit break` 或 `all ai keys exhausted ... return synthetic 429` + `fallback triggered`；④ 无 0 上游交互的 500。

## 四、修复后目标行为

以现场配置（`[yonggang/100, zhangmiao/0]`，`zhangmiao` 为管理员故意置 0）为例：

1. `yonggang` 被上游 429 → 写 60s 惩罚键（既有行为）；
2. 惩罚窗口内的新请求：`filterPenaltyKeys` 剔除 `yonggang`（`zhangmiao` Weight=0 本就不计候选）→ 候选为空 → **熔断**：选 Key 失败，#1440 合成 429 → `shouldTriggerFallback` 命中白名单 → **降级到备用集群（bailian）**，与 kimi 零交互；
3. 60s 后惩罚键过期 → 过滤器放行 `yonggang` → 流量自动回 kimi，无需任何状态清理；
4. 窗口内若有新请求再次吃到 429（比如熔断前已发出的在途请求回包），惩罚键被**重新写入、TTL 刷新**，熔断期相应顺延——上游持续限流期间 kimi 持续被熔断，这正是方案乙的预期效果；
5. 双可用 Key 集群同理：任一 Key 被惩罚时请求走另一个；两个都被惩罚时熔断降级。

## 五、边界与残余风险

- **裁定点 1（惩罚过滤后"全军覆没"的处置）——已裁定（2026-10-21，owner：zhangmiao）：方案乙"熔断"**。即：删除 `:2209-2212` 的"回退全量"兜底，全部可用 Key 被惩罚时返回空候选 → 选 Key 失败 → #1440 合成 429 → cluster 降级，窗口内零上游交互，TTL 到期自动恢复。裁定动机（owner 原话意图）："如果 2 个 Key 都 429，就执行降级改用备用集群；过 60s 再重试 kimi"。被否决的方案甲"回退到被惩罚 Key"：会让被惩罚 Key 在窗口内继续被使用，违背熔断语义。**该边界需回补进 `2026-08-26-ai-key-session-affinity/design-changes.md` §6.2.2**（原条款只定义了 TTL 与"跳过被惩罚 Key"，未定义"过滤后无候选"的语义）。
- **裁定点 2（单可用 Key 短路泛化）——已裁定（2026-10-21，owner：zhangmiao）：不实施**。单可用 Key 短路会绕过惩罚过滤、使熔断失效；原"物理上单 Key"短路（`len(keys)==1`）保留既有行为。推论：**启用 `SessionAffinityPenaltyEnable` 的集群若长期只配置 1 个 Key，惩罚不会生效**——由 #1442 的配置建议兜底（单可用 Key 集群应关闭 `SessionAffinityPenaltyEnable`，或接受"惩罚不生效"的事实；`zhangmiao` Weight=0 为管理员故意设置，单可用 Key 是长期稳态）。
- **不改变**：`chooseNextAIKey` 的权重随机、`usedSet`/`deadSet` 语义、429/401/403 惩罚 TTL（60s/3600s）、session 绑定的 TTL 刷新逻辑、原"物理上单 Key"短路均不变。
- **与 #1442 的联动**：方案乙落地后，kimi 集群"唯一 Key 被 429 → 60s 全量降级 bailian"成为**设计内行为**（而非缺陷）；#1442 的处置重心相应从"修代码"转为"配置与监控"——建议按 #1442 §三落地"可用 Key 数 < 2"与熔断日志的告警。
- **v1.8.9-dev 同步**：`develop`/`v1.8.9-dev` 上相关函数行号不同，移植时按同名函数复核；该分支新增的上游错误归一化（`reverseproxy_ai_error.go`）在 fallback 循环收敛后运行，与本修复无交互（合成 429 带 `X-Bfe-Gw-Error` 网关标记会被跳过）。

## 六、实施记录（2026-10-21，v1.8.8 分支）

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | `filterPenaltyKeys`：排除 `Weight=0` 的 Key（不计候选、不计 skipped）；删除"全部被惩罚则回退全量"兜底，改为返回空候选并打 Info 日志 `aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break`（熔断，issue #1439 方案乙） |
| `bfe_server/reverseproxy_ai_test.go` | 新增 4 例：`TestFilterPenaltyKeys_ExcludesZeroWeight`（被惩罚可用 Key + Weight=0 → 空候选）、`TestFilterPenaltyKeys_CircuitBreakWhenAllPenalized`（全部可用 Key 被惩罚 → 空候选，关键行为变化）、`TestFilterPenaltyKeys_ZeroWeightNotCandidate`（Weight=0 不计 skipped）、`TestChooseAIKeyWithAffinity_CircuitBreak`（端到端选 Key 失败 + skipped 计数） |
| `docs/zh_cn/modifications/2026-08-26-ai-key-session-affinity/design-changes.md` | §6.2.2 回补过滤口径与熔断语义条款（含单 Key 短路绕过惩罚的注意事项） |
| `tests/integration/.../scenario-SC10-ai-key-session-affinity/` | testEnv 增加 `cluster_session_affinity_fallback` 备用后端（全 TC 常驻，不影响 TC-01~TC-07）；`ai_route.data` 默认规则配置 fallbacks；`gslb.data` 增加备用集群条目；新增 `bfeLogContains`；新增 `TestTC08_AllKeysPenalizedCircuitBreaksToFallback` |
| `tests/integration/测试设计文档/scenario-SC10-.../` | 新增 TC-08 设计文档；场景说明 TC 列表与总体说明 TC 数（7→8）更新 |

**实施中与文档的偏差**：

- TC-08 请求 1 的预期从"429"修正为"200（经降级）"——Key 级耗尽返回的最后一次**真实** 429 命中 fallback 白名单，请求 1 即触发降级（#1440 之前的既有语义）；两请求的关键区分证据是目标集群命中数（请求 1 ≥2，请求 2 零新增）。设计文档已同步。
- 步骤 2（单可用 Key 短路泛化）按裁定不实施，未改动 `chooseAIKeyWithAffinity`。

**验证结果（本次实跑）**：

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、SC10 测试包） | 通过 |
| `bfe_server` / `mod_ai_route` / `mod_ai_token_auth` 单测 | 全部通过 |
| SC10 全量（含新增 TC-08） | 通过（实测日志出现 `circuit break` 与 `fallback triggered ... status[429]`） |
| SC02 / SC07 / SC08 回归 | 通过 |

**遗留事项**：v1.8.9-dev 前向移植（`filterPenaltyKeys` 同名函数 + 单测/TC-08 同步，注意 dev 上 Redis 句柄来自 `srv.AIKeyAffinityRedis`）；#1442 的配置与监控处置按该文档推进。
