# BFE Issue #1439 修复方案（v1.8.9-dev 前向移植）：Key 惩罚过滤排除 Weight=0 并改为熔断语义

- Issue: https://github.com/bfenetworks/bfe/issues/1439
- 生产事件编号：prod-20261021-01
- 性质：v1.8.8 分支修复（commit `e6c97fd4`）向 v1.8.9-dev 的前向移植。缺陷描述、根因分析、两处裁定点（方案乙"熔断"语义、单可用 Key 短路不实施）的完整记录见 v1.8.8 分支同名文档；本文记录 **dev 分支的移植内容与验证**。
- 缺陷（摘要）：集群配置 2 个 Key（1 个可用 + 1 个 `Weight=0`）且开启 `SessionAffinityPenaltyEnable` 时，唯一可用 Key 被上游 429 进入 60s 惩罚窗口后，`filterPenaltyKeys()` 只按"是否被惩罚"过滤、不排除 `Weight=0` 的 Key——过滤结果非空（仅剩 `Weight=0` 的 Key），不触发"全被惩罚"的判定；随后 `chooseNextAIKey()` 又因候选权重全 0 返回 `ok=false`，请求**零上游交互**直接失败，1 个 Key 限流被放大为整集群 60 秒不可用（生产 932 笔 500）。

## 一、修复内容（与 v1.8.8 一致）

### 1. `filterPenaltyKeys()`：排除 Weight=0 + 删除"回退全量"兜底（熔断）

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
		// surfaces a synthetic 429 (issue #1440) for cluster fallback. The
		// old "fallback to original list" branch was removed on purpose:
		// penalized keys must stay excluded until the penalty TTL expires.
		log.Logger.Info("aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break", clusterName)
		return nil, skipped
	}
	return filtered, skipped
}
```

语义要点（2026-10-21 裁定，方案乙"熔断"，owner：zhangmiao）：

- `Weight=0` 的 Key 从不参与选 Key，惩罚过滤时不计入候选，也不计入 `ReqAiKeyAffinityPenaltySkip` 的 skipped 计数；
- 被惩罚的 Key 在 TTL 窗口内**严格剔除**；全部可用 Key 被惩罚时返回空候选（**删除原"回退全量"兜底**），选 Key 失败 → #1440 的合成 429（dev 为统一 AiError 目录形态、带 `X-Bfe-Gw-Error` 网关标记）→ 外层 cluster fallback 降级；窗口内与上游零交互，TTL 到期自动恢复，新 429 续期熔断；
- 触发熔断时打 Info 日志 `aiKeyAffinity: all usable keys penalized for cluster[%s], circuit break`；
- 单可用 Key 短路泛化**不实施**（短路与熔断语义冲突）；物理上单 Key 配置的集群惩罚不生效（单 Key 短路绕过过滤），配置建议见 issue #1442（单 Key 集群应关闭 `SessionAffinityPenaltyEnable`）。

### 2. 与 dev 分支既有机制的交互（已核对）

- **上游错误归一化**（`reverseproxy_ai_error.go`）：合成 429 带 `X-Bfe-Gw-Error: 1`，归一化器对网关生成的响应跳过，无二次改写；
- **batch 透传**（`isBatchPassthroughMode`/`batchHint`）：`chooseAIKeyWithAffinity` 的签名在 dev 上多一个 `batchHint` 参数，惩罚过滤路径不变；
- **Redis 句柄**：dev 上亲和 Redis 来自 `srv.AIKeyAffinityRedis`（`bfe.conf [AIKeyAffinity]`），不影响 `filterPenaltyKeys` 本身；
- `mod_ai_cache` 缓存命中与 `mod_ai_batch` 的 404/413 均不经过惩罚过滤路径。

## 二、改动文件

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | `filterPenaltyKeys`：排除 `Weight=0`、删除"回退全量"兜底、熔断 Info 日志 |
| `bfe_server/reverseproxy_ai_test.go` | 新增 4 例：`TestFilterPenaltyKeys_ExcludesZeroWeight`、`TestFilterPenaltyKeys_CircuitBreakWhenAllPenalized`、`TestFilterPenaltyKeys_ZeroWeightNotCandidate`、`TestChooseAIKeyWithAffinity_CircuitBreak`（dev 上 `chooseAIKeyWithAffinity` 多 `batchHint` 参数，调用传 nil） |
| `tests/integration/.../scenario-SC10-ai-key-session-affinity/` | testEnv 增加 `cluster_session_affinity_fallback` 备用后端（全 TC 常驻，不影响 TC-01~TC-07）；`ai_route.data` 默认规则配置 fallbacks；`gslb.data` 增加备用集群条目；新增 `bfeLogContains`；新增 `TestTC08_AllKeysPenalizedCircuitBreaksToFallback` |
| `tests/integration/测试设计文档/scenario-SC10-.../` | 新增 TC-08 设计文档；场景说明 TC 列表与总体说明 TC 数（7→8）更新 |

> 说明：按 2026-10-21 约定，dev 分支**不改写** `2026-08-26-ai-key-session-affinity/design-changes.md` 原条款，熔断语义以本文档为准（v1.8.8 分支的 §6.2.2 回补见该分支提交 `e6c97fd4`）。

## 三、验证结果（v1.8.9-dev，本次实跑）

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、SC10 测试包） | 通过 |
| `bfe_server` 单测（含新增 4 例熔断用例） | 通过 |
| SC10 `TestTC08_AllKeysPenalizedCircuitBreaksToFallback` | 通过（实测日志出现 `circuit break` 与 `fallback triggered ... status[429]`） |
| SC10 全量 / SC02 / SC07 / SC08 回归 | 见提交前验证 |

## 四、修复后目标行为

以 issue 现场配置（`[yonggang/100, zhangmiao/0]`，`zhangmiao` 为管理员故意置 0）为例：

1. `yonggang` 被上游 429 → 写 60s 惩罚键；
2. 惩罚窗口内新请求：`filterPenaltyKeys` 剔除 `yonggang`（`zhangmiao` Weight=0 本就不计候选）→ 候选为空 → **熔断**：选 Key 失败，合成 429 → 降级到备用集群（bailian），与 kimi 零交互；
3. 60s 后惩罚键过期 → 过滤器放行 → 流量自动回 kimi；窗口内新 429 会刷新 TTL、顺延熔断；
4. 双可用 Key 集群：任一被惩罚走另一个；全部被惩罚时熔断降级。
