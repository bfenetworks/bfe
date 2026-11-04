# AI Key 选择：惩罚过滤后下标空间错位（state 误判）

- 性质：**既有缺陷**（早于 issue #1448），在实现 #1448 的集成场景 SC46 时由测试暴露并复现。
- 代码库：`bfe/`，分支 `v1.9.0-dev`。行号为 `a293ab6c`（含 #1448 改动）核对。
- 关联：`docs/zh_cn/modifications/2026-10-21-issue-1439-ai-key-penalty-filter-zero-weight/`（惩罚过滤引入）、`2026-10-21-issue-1440-key-exhausted-no-fallback/`（耗尽后的合成 429）、`2026-11-04-issue-1448-configurable-key-penalty-ttl/`（暴露该缺陷的改动）。
- 状态：**已实现（2026-11-04）**，实现记录见 §七。

## 一、缺陷一句话

`aiKeyAttemptState` 的 `deadSet` / `usedSet` 用 **`AIConf.Keys` 数组下标**索引，但 `chooseAIKeyWithAffinity` 把 `filterPenaltyKeys` 过滤后的**子切片**交给 `chooseNextAIKey`，后者的读写都改用**子切片下标**。两套下标混用，导致健康 Key 被判为 dead（合成 429、零尝试），或把失败 Key 的标记写到无辜 Key 上。

## 二、代码事实（HEAD `a293ab6c`）

`bfe_server/reverseproxy.go`：

```go
// :2633-2636  state 的契约：keys 数组下标
type aiKeyAttemptState struct {
	usedSet map[int]struct{} // index of keys used for 429 in this request
	deadSet map[int]struct{} // index of keys dead for 401/402/403 in this request
}

// :2674-2716  读写都以「传入切片」为下标空间
func chooseNextAIKey(keys []cluster_conf.AIKey, state *aiKeyAttemptState) (int, cluster_conf.AIKey, bool) {
	for i, k := range keys {                     // i = 传入切片下标
		if k.Weight == 0 { continue }
		if _, dead := state.deadSet[i]; dead { continue }
		...
	}
	...
	return filteredIdx[selectedIdx], filteredKeys[selectedIdx], true  // 仍是传入切片下标
}

// :2910-2935  过滤掉 Weight=0 与被惩罚的 Key，返回新切片
func filterPenaltyKeys(...) ([]cluster_conf.AIKey, int) { ... }

// :2986-2990 / :3009 / :3019 / :3041  用子切片做选择
candidateKeys := keys
if policy.SessionAffinityPenaltyEnable {
	candidateKeys, penaltySkipCount = filterPenaltyKeys(clusterName, keys, policy, client)
}
...
idx, key, ok := chooseNextAIKey(candidateKeys, state)
```

`idx` 回到 `aiClusterInvoke` 后按 `keys` 空间使用：

```go
// :2360 / :2371
state.usedSet[idx] = struct{}{}   // 429
state.deadSet[idx] = struct{}{}   // 401/402/403
```

同一 state 还被**绑定命中**路径按 `keys` 空间使用（`findKeyIndexByName` + `isKeyAlive(idx, keys, state)`，`:2996-3001`、`:3024-3036`），因此两套语义在同一个 map 里并存。

## 三、触发条件与后果

触发前提：`SessionAffinityPenaltyEnable=true` 且 `candidateKeys != keys` —— 即**有 Key 被过滤**（被惩罚，或 `Weight=0`）。只要有被过滤项，下标就可能重排。

### 3.1 读方向：健康 Key 被判 dead（本次复现的形态）

`keys = [A, B]`，A（下标 0）已 dead 且被惩罚 → `candidateKeys = [B]`（下标 0）→ `chooseNextAIKey` 查 `deadSet[0]` 命中 A 的标记 → B 被判 dead → 无候选 → `aiClusterInvoke` 直接返回合成 429，**B 从未被尝试**。

复现（临时单测，已删除）：

```go
keys := []AIKey{{Name:"key-a",Weight:50},{Name:"key-b",Weight:50}}
policy := AIKeyPolicy{SessionAffinity:true, SessionAffinityPenaltyEnable:true, ...}
client.Setex("...penalty:cluster:key-a", []byte("401"), 300) // A 被惩罚
state := newAIKeyAttemptState()
state.deadSet[0] = struct{}{}                                // A 本轮 401
_, key, _, ok := chooseAIKeyWithAffinity("cluster", keys, policy, state, client, "session", nil, nil)
// 实测: ok=false；期望: key-b/true
```

集成侧表现：SC46 TC-01 的 401 子用例有时只产生 1 个惩罚键（第二把 Key 未被尝试），因此该子用例暂按「至少一个」断言，并在场景说明 §8 记录。

### 3.2 写方向：失败标记写到无辜 Key 上

`keys = [Z(w=0), A, B]`，Z 被过滤 → `candidateKeys = [A, B]`；若选中 A（子切片下标 0），`aiClusterInvoke` 记为 `deadSet[0]`，而 `keys[0]` 是 Z。后果：

- 下一次重试 `chooseNextAIKey(keys, ...)` 认为 Z dead（Z 本就 Weight=0，无实际影响），A 仍"存活" → **同一把刚失败的 Key 被再次选中**，放大上游调用；
- 若 `keys = [Z, A]` 且 A 是唯一可用 Key，MaxRetries 次重试会全部打在 A 上（401 场景），而不是及时熔断。

### 3.3 影响面

| 维度 | 说明 |
|------|------|
| 触发面 | 仅 AI 多 Key 选择；需存在被过滤的 Key（惩罚窗口内或 `Weight=0`） |
| 用户可见 | 同一请求内健康 Key 未尝试即合成 429（有 fallback 时降级、无 fallback 时客户端收 429）；或对同一失败 Key 重复重试 |
| 不受影响 | 429 的 `usedSet` 在"全部 used"时会重置（`:2705-2710`），因此 429 轮换基本自愈；单可用 Key 短路（`:2982-2984`）不经过过滤，也不受影响 |
| 与 #1448 的关系 | #1448 只改惩罚时长来源，不改变本缺陷；但配置化后惩罚窗口更常被调整，暴露概率上升 |

## 四、修复方案

**原则**：state 的下标空间统一为 `keys` 原始下标；过滤只产出「允许的下标集合」，不再产出重排后的切片。

### 4.1 `filterPenaltyKeys` 返回原始下标

```go
// filterPenaltyKeys returns the indices (into keys) of the keys that may be
// selected, plus how many usable keys were skipped because of a penalty.
func filterPenaltyKeys(clusterName string, keys []cluster_conf.AIKey, policy cluster_conf.AIKeyPolicy,
	client redis_client.Client) ([]int, int) {

	var allowed []int
	skipped := 0
	for i, k := range keys {
		if k.Weight == 0 {
			continue // Weight=0 keys never count as candidates (issue #1439)
		}
		if !isKeyPenalized(clusterName, k.Name, policy, client) {
			allowed = append(allowed, i)
		} else {
			skipped++
		}
	}
	if len(allowed) == 0 {
		log.Logger.Info("aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break", clusterName)
		return nil, skipped
	}
	return allowed, skipped
}
```

### 4.2 `chooseNextAIKey` 接收允许下标

```go
// chooseNextAIKey selects a key among allowed (indices into keys; nil means
// every index). The returned index is always an index into keys, which is what
// aiKeyAttemptState.deadSet / usedSet are keyed by.
func chooseNextAIKey(keys []cluster_conf.AIKey, allowed []int, state *aiKeyAttemptState) (int, cluster_conf.AIKey, bool) {
	ordered := allowed
	if ordered == nil {
		ordered = make([]int, len(keys))
		for i := range keys {
			ordered[i] = i
		}
	}

	var eligible []cluster_conf.AIKey
	var indices []int
	for _, i := range ordered {
		if i < 0 || i >= len(keys) {
			continue
		}
		if keys[i].Weight == 0 {
			continue
		}
		if _, dead := state.deadSet[i]; dead {
			continue
		}
		eligible = append(eligible, keys[i])
		indices = append(indices, i)
	}
	if len(eligible) == 0 {
		return -1, cluster_conf.AIKey{}, false
	}

	var filteredKeys []cluster_conf.AIKey
	var filteredIdx []int
	for j, k := range eligible {
		idx := indices[j]
		if _, used := state.usedSet[idx]; used {
			continue
		}
		filteredKeys = append(filteredKeys, k)
		filteredIdx = append(filteredIdx, idx)
	}
	if len(filteredKeys) == 0 {
		state.usedSet = make(map[int]struct{}) // all alive keys used (429 only), retry
		filteredKeys = eligible
		filteredIdx = indices
	}

	_, selectedIdx := selectAIKey(filteredKeys)
	if selectedIdx < 0 {
		return -1, cluster_conf.AIKey{}, false
	}
	return filteredIdx[selectedIdx], filteredKeys[selectedIdx], true
}
```

要点：`eligible/indices` 直接来自 `keys` 空间，`filteredIdx` 因此始终是 `keys` 下标；`allowed == nil` 时行为与原实现一致。

### 4.3 `chooseAIKeyWithAffinity` 调用点

```go
	allowed := []int(nil)
	penaltySkipCount := 0
	if policy.SessionAffinityPenaltyEnable {
		var indices []int
		indices, penaltySkipCount = filterPenaltyKeys(clusterName, keys, policy, client)
		if indices == nil {
			// circuit break：无候选，交给 chooseNextAIKey 返回 false
			allowed = []int{}
		} else {
			allowed = indices
		}
	}
	...
	idx, key, ok := chooseNextAIKey(keys, allowed, state)   // 共用同一组下标
```

三处 `chooseNextAIKey(candidateKeys, state)`（`:3009`、`:3019`、`:3041`）与回退分支（`:2976`）统一改为 `chooseNextAIKey(keys, allowed, state)`；`allowed == nil`（未开启惩罚或早退分支）表示全部下标。

### 4.4 备选方案与取舍

| 方案 | 改动面 | 评价 |
|------|--------|------|
| A. 下标集合（本文方案） | 2 个函数签名 + 4 个调用点 + 单测 | **推荐**：最小改动，保持 `state` 契约不变，返回的 `idx` 与 `AIKey` 永远配套 |
| B. state 改按 Key 名索引 | `aiKeyAttemptState` 类型 + 全部读写点 | 彻底消除下标语义，但依赖 `Keys[].Name` 唯一（当前只在控制面保证），且回归面更大 |
| C. 过滤时给每把 Key 打上原始下标（包装结构） | 引入新结构体穿过选择链 | 表达清晰但改动比 A 大 |

## 五、测试方案

### 5.1 单元测试（`bfe_server/reverseproxy_ai_test.go`）

| 用例 | 断言 |
|------|------|
| `TestChooseAIKeyWithAffinity_PenalizedSiblingNotSkipped` | 复现 §3.1：A 被惩罚 + `deadSet[0]`，返回 `key-b` 且 `idx == 1`、`ok == true` |
| `TestChooseNextAIKey_AllowedKeepsOriginalIndex` | `allowed=[1]`、state 为空 → 返回 `idx == 1`；`state.deadSet[0]` 不应影响结果 |
| `TestChooseAIKeyWithAffinity_WeightZeroBeforeUsable` | `keys=[Z(w0), A]`：选中 A 返回 `idx == 1`；把 `deadSet[1]` 置位后重选应返回 `ok == false`（证明写入的标记落在 A 上） |
| `TestFilterPenaltyKeys_*`（现有 3 条） | 改为断言返回的下标集合（`[1]` / `[nil]` 等），并保留 `skipped` 计数断言 |

### 5.2 集成测试（SC46 扩展）

在 `scenario-SC46-key-penalty-ttl` 增 TC-07「健康 Key 不被惩罚过滤误跳过」：2 把可用 Key、上游对 `key-a` 返回 401、对 `key-b` 返回 200，一次请求应命中 `key-b` 并返回 200；修复前该请求会直接合成 429（零 `key-b` 交互）。同时把 TC-01 的 401 子用例恢复为「两个惩罚键」的严格断言。

### 5.3 回归

- `go test ./bfe_server/...`
- `go test ./tests/integration/implementation/scenario-SC46-key-penalty-ttl/`
- `go test ./tests/integration/implementation/scenario-SC10-ai-key-session-affinity/ ./tests/integration/implementation/scenario-SC02-multi-api-key/`

## 六、兼容性与风险

- **无配置/协议变化**：不改 `cluster_conf.data` 字段、不改 Redis 键格式与 TTL、不改指标口径；
- **行为变化（修复目标）**：2 把以上可用 Key 的集群，某把 Key 401 后同一请求会真正尝试下一把 Key，可能由"合成 429"变为"第二把 Key 成功返回 200"；惩罚跳过计数 `ReqAiKeyAffinityPenaltySkip` 的累加时机不变；
- **风险**：`chooseNextAIKey` 签名变化会波及全部调用点，遗漏即把 `nil` 当作"无候选"（`allowed = []int{}` 与 `nil` 语义不同，需在实现时核对每一处）；单测已覆盖两种语义；
- **不包含**：不改变"惩罚窗口内跳过 Key"的策略、不引入半开探测、不动 429/401 的失败分类。

## 七、实施记录

**实施日期**：2026-11-04。基线 `v1.9.0-dev` @ `a293ab6c`（#1448 之上）。

### 7.1 代码落点

| 文件 | 实施内容 |
|------|----------|
| `bfe_server/reverseproxy.go` | `filterPenaltyKeys` 改为返回 `[]int`（`keys` 原始下标，`nil` 表示熔断）；`chooseNextAIKey(keys, allowed, state)` 新增 `allowed` 参数，`nil` = 全部下标、空切片 = 无候选，返回值恒为 `keys` 下标；`chooseAIKeyWithAffinity` 四处调用统一传 `keys` + `allowed`，并在熔断时把 `nil` 显式转成 `[]int{}` |
| `bfe_server/reverseproxy_ai_test.go` | 现有 `chooseNextAIKey` 调用补 `nil`；`TestFilterPenaltyKeys_*` 改为下标断言（`ZeroWeightNotCandidate` 断言返回 `[1]`）；新增 3 条回归用例（见 7.3） |
| `tests/integration/implementation/scenario-SC46-key-penalty-ttl/` | 新增 TC-07 回归用例；TC-01 的 401 子用例恢复「两个惩罚键」严格断言 |
| `tests/integration/测试设计文档/scenario-SC46-Key惩罚时长可配置/` | 新增 `TC-07-健康Key不被误跳过.md`；`TC-01` 预期结果恢复严格口径；`场景说明.md` §8 改写为已修复并指向本文件 |
| `tests/integration/测试设计文档/测试场景总体说明.md` | SC46 用例数 6 → 7，增 TC-07 与本文档链接 |

### 7.2 关键实现细节

- `allowed == nil` 与 `allowed == []int{}` 必须区分：前者表示「未开启惩罚过滤」，后者表示「熔断，无候选」。实现上用 `if allowed == nil { allowed = []int{} }` 在过滤返回后收敛语义，避免把「未过滤」误当「无候选」。
- `chooseNextAIKey` 内部 `ordered` 即 `allowed`（或 `0..len(keys)-1`），`eligible/indices` 直接携带 `keys` 下标，`filteredIdx[selectedIdx]` 因此与 `aiClusterInvoke` 的 `usedSet/deadSet` 写入一致。

### 7.3 验证结果

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 新增/更新单测 | `go test ./bfe_server/ -run 'ChooseNextAIKey\|FilterPenaltyKeys\|ChooseAIKeyWithAffinity' -v` | 15 条全部通过 |
| 单测全包 | `go test ./bfe_server/ ./bfe_config/bfe_cluster_conf/cluster_conf/` | 通过 |
| 集成 SC46 | `go test ./tests/integration/implementation/scenario-SC46-key-penalty-ttl/` | 7 条（TC-01…TC-07）全部通过 |
| 回归 | SC10 会话亲和、SC02 多 Key 轮换 | 全部通过 |
| 格式/静态检查 | `gofmt -l`、`go vet`（改动包） | 干净 |

新增回归用例：

1. `TestChooseAIKeyWithAffinity_PenalizedSiblingNotSkipped`——`key-a` 被惩罚且 `deadSet[0]`，必须返回 `key-b@1`（修复前返回 `ok=false`）；
2. `TestChooseNextAIKey_AllowedKeepsOriginalIndex`——`allowed=[1]` 返回下标 1；`allowed=[]int{}` 返回无候选；
3. `TestChooseAIKeyWithAffinity_WeightZeroBeforeUsable`——`[Z(w=0), A]` 返回 A@1；把 `deadSet[1]` 置位后不再可选（证明标记落在正确条目）。

集成侧 TC-07：会话预绑 `key-a`、`key-a` 返回 401、`key-b` 返回 200 → 同一请求返回 200、上游命中 2 次（`key-a` → `key-b`）、只有 `key-a` 一个惩罚键。

### 7.4 遗留事项

- 未提交、未推送（需人工确认）。
- 无配置/协议/指标变化；不涉及 `cluster_conf.data` 字段与 Redis 键格式。