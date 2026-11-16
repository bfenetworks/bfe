# BFE Issue #1472 修复方案：亲和绑定未命中被误判为 Redis 故障

- Issue: https://github.com/bfenetworks/bfe/issues/1472
- 缺陷一句话：`SessionAffinity=true` 的集群上，任一 `ClientKeyId` 的首次请求（Redis 中尚无绑定）都会打出 `WARN aiKeyAffinity: redis get binding error[redigo: nil returned], fallback to random` 并把 `ReqAiKeyAffinityRedisErr` +1；而该事件的真实语义只是"未命中绑定"。
- 代码库：`bfe/`。§一/§三 的行号是**修复前**的 HEAD `484f830a`（2026-11-02，`v1.9.0-dev`），逐条直读核实；同一缺陷在线上版本 tag `v1.8.8.7` 亦存在，两处行号并列标注。修复后的落点见 §八。
- 设计依据：`docs/zh_cn/sys_design/multi_api_key.md` §3.5.3 第 4 条、§3.5.5、§3.5.6 指标表（原文见 §七）。
- 结论：**改读取点，两行判断**（`redis_client.IsKeyNotFound`），不动共享 `RedisClient.Get` 契约、不动指标口径、不动绑定写入时序。

## 一、根因

### 1.1 包装层把"键不存在"表达为错误

`bfe_util/redis_client/redis_bns.go:439-469` `RedisClient.Get`：

```go
value, err := conn.Do("GET", key)
// redigo may return both value and err is nil
if value == nil && err == nil {
    c.incrModuleState2(RedisGetMiss)
    return nil, redis.ErrNil          // :450-453 未命中 → 固定错误串 "redigo: nil returned"
}
```

这是**有意**的统一契约：同包 `bfe_util/redis_client/client.go:31-34` 提供了配套的 `IsKeyNotFound(err)`（`errors.Is(err, redis.ErrNil)`），调用方负责区分"未命中"与"真失败"。`mod_ai_cache/redis.go:52-70` 就是这么用的。

### 1.2 亲和读取点没有区分，把未命中上抛为错误

`bfe_server/reverseproxy.go:2730-2744`（`v1.8.8.7:2381-2395`，代码完全一致）：

```go
func redisGetBinding(...) (string, error) {
    val, err := client.Get(key)
    if err != nil {
        return "", err        // :2733-2735 未命中（ErrNil）与真失败同一出口
    }
    if val == nil {
        return "", nil        // :2736-2738 仅对返回 (nil, nil) 的客户端生效
    }
    ...
}
```

`val == nil` 分支在生产的 `redis_bns` 客户端下**永不可达**——`Get` 已经把未命中转成 `ErrNil` 提前返回了。

### 1.3 错误分支提前 return，吞掉未命中计数与内联写绑定

`reverseproxy.go:2875-2883`（`v1.8.8.7:2503-2511`）：

```go
boundName, err := redisGetBinding(clusterName, sessionID, policy, client)
if err != nil {
    log.Logger.Warn("aiKeyAffinity: redis get binding error[%v], fallback to random", err)
    proxyState.ReqAiKeyAffinityRedisErr.Inc(1)
    idx, key, ok := chooseNextAIKey(candidateKeys, state)
    return idx, key, "", ok          // 提前返回
}
```

未命中走这条分支后，紧随其后的 `redisSetBinding`（`:2903-2911` / `v1.8.8.7:2531-2541`）与 `ReqAiKeyAffinityMiss.Inc(1)`（`:2912-2915` / `v1.8.8.7:2542`）在生产**不可达**。注意与 issue 描述的措辞差异：v1.8.8.7 **有**内联写绑定代码（`:2533`），但只对"绑定存在但失效"的 stale 路径可达；**未命中路径被 1.2/1.3 短路，等价于不存在**。

由此绑定只能由成功补写产生（`reverseproxy.go:2207-2216` / `v1.8.8.7:2046-2056`）：

```go
if err == nil && statusCode < 400 {
    if policy.SessionAffinity && redisClient != nil && sessionID != "" && key.Name != boundName {
        redisSetBinding(...); proxyState.ReqAiKeyAffinityRebind.Inc(1)
    }
}
```

### 1.4 后果（与 issue 的生产观测一一对应）

| 事件 | 条款要求的计数 | 实际计数 | 实际日志 |
| --- | --- | --- | --- |
| 首次请求，无绑定（正常未命中） | `MISS +1`，并写入新绑定 | `MISS +0`、`REDIS_ERR +1` | 一条 `WARN ... error[redigo: nil returned]` |
| 空闲超过 TTL（`SessionAffinityTTL` 是空闲过期）后首条请求 | 同上 | 同上 | 同上 |
| 429/401 删除绑定后的首条请求 | 同上 | 同上 | 同上 |
| 真 Redis 故障（连接拒绝/读超时） | `REDIS_ERR +1` | `REDIS_ERR +1` | WARN（与上者同文案） |

- `ReqAiKeyAffinityMiss` 生产恒 0 ⇒ `hit/(hit+miss)` 恒 100%，首触比例不可观测；**同一提前返回还吞掉了 `ReqAiKeyAffinityPenaltySkip`（`:2916-2918`）**——惩罚过滤发生在 `redisGetBinding` 之前，但计数在之后，故所有未命中请求的"因惩罚跳过 N 个 Key"同样不计；
- `ReqAiKeyAffinityRedisErr` 100% 由假阳性构成 ⇒ 真实故障不可区分，该告警维度失效；
- 每会话一条 WARN 灌水（issue 记录 24h 203 条 / 5h 117 条）；
- 首请求失败（超时/5xx/4xx）的会话建立不了绑定 ⇒ 后续请求重新随机，跨 provider 重试时缓存命中归零。

### 1.5 同类第二处：batch 绑定读取（仅 `v1.9.0-dev` 有）

`reverseproxy.go:2666-2701` `redisGetBatchBinding` 同样 `if err != nil { Warn("batch binding get error") ; RedisErr++ }`。首个 batch 操作（Redis 无 batch 绑定）会打同性质的 WARN 并虚增 `REDIS_ERR`。`v1.8.8.7` 无 batch 亲和（`redisGetBatchBinding` 在该 tag 不存在），回移时无需处理。

### 1.6 单测为何没暴露

`bfe_server/reverseproxy_ai_test.go:433-441` 的 mock：键缺失返回 `(nil, nil)`，而生产 `RedisClient.Get` 返回 `(nil, redis.ErrNil)`。语义不一致导致 `TestChooseAIKeyWithAffinity_Miss`（`:574-610`）恰好走通了生产走不到的分支，测试基线因此不能作为 Expected 依据（issue 证据表同结论）。

## 二、修复方案

### 步骤 1（必改）：新增读取归一 helper，`redisGetBinding` 区分未命中

`bfe_server/reverseproxy.go`，在 `redisGetBinding` 前新增：

```go
// redisGetOrMiss reads key and normalizes an absent key to a miss instead of
// an error: RedisClient.Get reports a missing key as redis.ErrNil so that
// callers can tell "not bound yet" (normal control flow for affinity) from a
// real Redis failure (issue #1472). The (nil, nil) form is still handled for
// clients that report a miss as an empty value.
func redisGetOrMiss(client redis_client.Client, key string) ([]byte, bool, error) {
	val, err := client.Get(key)
	if err != nil {
		if redis_client.IsKeyNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if val == nil {
		return nil, false, nil
	}
	b, ok := val.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("unexpected redis value type %T", val)
	}
	return b, true, nil
}
```

`redisGetBinding`（`:2730-2744`）改为：

```go
func redisGetBinding(clusterName, sessionID string, policy cluster_conf.AIKeyPolicy, client redis_client.Client) (string, error) {
	key := aiKeyAffinityRedisKey(clusterName, sessionID, policy.SessionAffinityRedisPrefix)
	b, found, err := redisGetOrMiss(client, key)
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	return string(b), nil
}
```

修复后首次请求的链路（与 §3.5.3 第 4 条一致）：`Get` → `ErrNil` → 归一为 `("", nil)` → 走**正常分支** → `chooseNextAIKey` 加权随机 → 内联 `redisSetBinding` 写新绑定 → `MISS +1` → 转发。首请求失败也已写入绑定（转发前写）。

### 步骤 2（必改，仅 `v1.9.0-dev`）：`redisGetBatchBinding` 同归

`reverseproxy.go:2676-2683` 的 `client.Get` + `err != nil` 改为调用 `redisGetOrMiss`：

```go
b, found, err := redisGetOrMiss(client, key)
if err != nil {
	log.Logger.Warn("aiKeyAffinity: batch binding get error[%v]", err)
	if proxyState != nil {
		proxyState.ReqAiKeyAffinityRedisErr.Inc(1)
	}
	continue
}
if !found || len(b) == 0 {
	continue
}
// 命中后续沿用原有的 Expire 滑动续期分支
```

副作用（可接受）：原实现对"值类型不是 `[]byte`"静默跳过，归一后该异常落入 `err` 分支，会打一条 WARN 并计 `REDIS_ERR`。该值只由 `mod_ai_batch` 以 `[]byte` 写入，非 `[]byte` 只可能来自外部误写，按故障上报更合理。

### 步骤 3（必改）：单测 mock 对齐生产语义 + 补用例

1. `reverseproxy_ai_test.go:433-441` 的 `mockRedisClient.Get` 键缺失时返回 `(nil, redis.ErrNil)`（新增 `github.com/gomodule/redigo/redis` import，该测试文件当前未引入），与 `redis_bns.go` 一致；此改动会让 `TestChooseAIKeyWithAffinity_Miss`（`:574-610`）与 `TestChooseAIKeyWithAffinity_PenaltySkip`（`:644-675`，其断言依赖 `:2916-2918` 的计数）在**不打修复**时变红、打上修复后变绿（红-绿证明）。
2. 新增 `TestRedisGetBinding_MissIsNotError`：键缺失 → `("", nil)`；`failGet` → 非 nil err。
3. 新增 `TestChooseAIKeyWithAffinity_MissNotRedisErr`：`ProxyState` 同时挂 `Hit/Miss/RedisErr`，断言 `MISS=1`、`REDIS_ERR=0`、`HIT=0`，且绑定键已写入、值为被选中的 key。
4. 新增 `TestRedisGetBatchBinding_MissNotRedisErr`：`batchHint` 命中但无绑定 → 返回 `""` 且 `REDIS_ERR=0`。
5. 现有 `TestChooseAIKeyWithAffinity_RedisErrFallback`（`:612-645`，`failGet` 走真错误）语义不变，保持绿。

### 步骤 4：集成回归（`integration-test/`，SC18 TC-17 扩展）

`TestTC17_ConfAgentKeyAffinitySameClientKey`（`scenario-SC18-cluster-multi-apikey/sc18_cluster_multi_apikey_conf_agent_test.go`）已具备三件必需设施：真实 BFE + `[AIKeyAffinity]` 段、miniredis 句柄（原先 `_ = redisServer` 丢弃）、BFE monitor 端口（`e.bfeMonitorPort`）。已实施：

1. 新增 `common/monitor_state.go`：`FetchProxyStateCounters(t, monitorPort) map[string]int64`（解析 `GET /monitor/proxy_state` 的默认 JSON）与 `CounterDelta(before, after, name)`；
2. env 保留 `redisServer` 句柄与 `bfeLogDir`，新增 `affinityBindings(cluster)`（按 `bfe:ai:key_affinity:<cluster>:` 前缀取绑定）与 `bfeLogLines()`；
3. 首发请求后断言：`MISS +1`、`REDIS_ERR +0`、Redis 中恰有一条绑定且值为 key-a/key-b/key-c 之一（判据 1）；
4. 30 条同客户端请求后：`HIT +29`、`REDIS_ERR +0`，后端 `Authorization` 仍只有一种（粘性不回归）；
5. 第二个 API-Key：`MISS +1`、`REDIS_ERR +0`、绑定数变为 2（每个 `ClientKeyId` 一次首触）；
6. BFE 日志（`bfe.log` / `bfe.log.wf`）不含 `redis get binding error`（判据 4 的机检形式）。

判据 2（真 Redis 故障仍计数）不适合进程级集成：miniredis 不好构造读超时，改由步骤 3 的 `failGet` 单测覆盖（`REDIS_ERR=1`、WARN 保留、`MISS` 不增）。

### 步骤 5：文档

`docs/zh_cn/sys_design/multi_api_key.md` §3.5.3/§3.5.5 无需改（条款本已是目标语义）；§3.5.6 指标表已补脚注："`ReqAiKeyAffinityMiss` 统计未命中 Redis 绑定的次数，包含会话首触与绑定失效后重新选择；未命中不计入 `ReqAiKeyAffinityRedisErr`（issue #1472）"。

## 三、方案对比与取舍

| 备选 | 方案 | 结论 |
| --- | --- | --- |
| A | 改共享 `RedisClient.Get`：未命中返回 `(nil, nil)`，彻底不产生 `ErrNil` | **拒绝**。`ErrNil` 是包装层刻意的统一契约（同包 `IsKeyNotFound` 就是为它准备的），改约定会静默改写所有现存与未来调用方（`mod_ai_cache/redis.go:58` 的 `IsKeyNotFound` 判断将变成死代码）；且真正的缺陷在调用点"无视了配套 helper"，改底座等于掩盖错误用法，新的亲和类调用点还会再犯 |
| B | 只在 `chooseAIKeyWithAffinity` 的错误分支加 `IsKeyNotFound` | **次优**。能修主路径，但 `redisGetBinding` 仍是"不可靠的读取原语"，`redisGetBatchBinding` 继续假阳性；同时未命中的"选择 + 写绑定 + 计数"逻辑会与正常分支重复一份 |
| C | **在 `redisGetBinding`/`redisGetBatchBinding` 读取点归一（采纳）** | 未命中回流到既有的唯一正常分支：`MISS+1`、内联写绑定、TTL 刷新都不必新增代码；真错误仍走原 WARN + `REDIS_ERR`；与 `mod_ai_cache` 的既有写法一致 |
| D | 顺带把"成功补写"删掉，绑定只在前置选择时写；并让选择器回报"当前已绑定名"以消除双写 | **本卡不做**。前置写已在（步骤 1 修复后未命中路径即可达），删除成功补写会改变 `ReqAiKeyAffinityRebind` 的基线（现状：每个新会话首请求 +1；改后趋近 0），与"因失败重新绑定次数"的定义纠缠，属独立可观测性债，另卡评估（见 §六） |

## 四、行为变化清单

| 项 | 修复前 | 修复后 |
| --- | --- | --- |
| 首次请求（无绑定） | `MISS +0`、`REDIS_ERR +1`、WARN 一条、绑定靠成功补写 | `MISS +1`、`REDIS_ERR +0`、无 WARN、转发前即写入绑定 |
| 空闲超 TTL 后首请求 / 429·401 删绑定后首请求 | 同上 | 同上 |
| 首请求失败（超时/5xx/4xx） | 无绑定，下条请求重新随机 | 已有绑定，下条请求命中同一 Key |
| 真 Redis 故障 | `REDIS_ERR +1` + WARN | **不变**（`IsKeyNotFound` 为假，原分支保留） |
| `ReqAiKeyAffinityRebind` | 每个新会话首请求 +1 | **不变**（成功补写条件 `key.Name != boundName` 在两种情况下都为真） |
| 新会话首请求的 Redis 写次数 | 1 次（成功补写） | 2 次（前置写 + 成功补写同一值）；已知冗余，见 §六 |
| `ReqAiKeyAffinityHit` / 粘性 / TTL 滑动 / 单 Key 短路 | — | 不变 |
| `ReqAiKeyAffinityPenaltySkip` | 未命中请求的"因惩罚跳过 N 个"被提前返回吞掉 | 正常计数 |
| batch 亲和首次操作（`v1.9.0-dev`） | 同上假阳性 WARN + `REDIS_ERR +1` | 同主路径，`REDIS_ERR` 不增 |

## 五、验收标准

对应 issue 验收判据（条款映射 A-01～A-04 见 §七）：

1. 无绑定 `ClientKeyId` 首发请求：无 `redis get binding error[redigo: nil returned]`，`REDIS_ERR` 增量 0，`MISS` 增量 1，请求后 Redis 中可读到本次选中的 key name（单测 + SC10 TC-09 + SC18 TC-17）；
2. 真故障（连接拒绝/读超时）：`REDIS_ERR +1`、WARN 保留、`MISS` 不增（单测 `failGet` + SC10 TC-10 停 Redis 实跑）；
3. 同 `ClientKeyId` 连续 10 条请求 key 稳定，`hit/(hit+miss)` 收敛到实际首触比例（SC10 TC-01/TC-09、SC18 TC-17 粘性断言不回归）；
4. 上线后 5 小时窗口内该 WARN 条数 == 真实 Redis 故障数（本次基线 117 条 → 期望 0），`REDIS_ERR` 不再单调增长；
5. 无相邻回归：首请求失败后仍能建立绑定（SC10 TC-11）；`SessionAffinity=false`、单 Key 短路、TTL 空闲过期三条路径行为不变（SC10 TC-02/TC-04/TC-06）；`cd bfe && make test` 全绿；`go test ./tests/integration/implementation/scenario-SC10-ai-key-session-affinity/ -v` 与 `go test ./test-cases/implementation/scenario-SC18-cluster-multi-apikey/... -v` 全绿。

## 六、边界与非目标

- **不动**：`RedisClient.Get`/`GetInt64` 契约；`isKeyPenalized`（`:2756-2763`，真故障也 fail-open 返回"未惩罚"，属既有降级设计，本卡不改）；`ReqAiKeyAffinityRebind` 语义；绑定 key 结构与 TTL；单 Key 短路；身份识别（`clientKeySessionID`）。
- **已知冗余（有意接受）**：新会话首请求会写两次同一绑定（前置写 + 成功补写）。代价是新会话首请求多一次 `SETEX` RTT，无正确性影响；之所以不在本卡内收掉，是因为收掉同时会让 `REBIND` 基线从"每新会话 +1"变为"仅 stale 重绑"，属指标口径变更，需单独评审。
- **可观测性残余**：`MISS` 修复后才开始增长，历史看板（`MISS` 恒 0 区间）与上线后数据不可直接拼接，说明需写进发布公告。
- **回滚**：改动集中在一个 helper + 两个调用点，单 commit 可 revert；无配置面变化，无需控制面联动。
- **版本落点**：主线 `v1.9.0-dev` 落全量（步骤 1-4）；`v1.8.8` 线回移步骤 1/3/4（线上 `v1.8.8.7` 受此缺陷直接影响，回移无 batch 部分）；若 `v1.8.9.x` 在线维护，同理同步。

## 七、条款对照（Expected 来源）

`docs/zh_cn/sys_design/multi_api_key.md`：

- §3.5.3 第 4 条（`:453-455`）："从 Redis 读取 `ClientKeyId -> key_name` 绑定：命中且 Key 仍有效：返回该 Key；**未命中：按候选列表加权随机选择，并写入新绑定**" → 断言 A-01：未命中必须走选择+写绑定路径，而非错误降级。
- §3.5.5（`:533-540`）："Redis Get 失败 \| 记录错误指标，回退到加权随机" → 断言 A-04：降级仅适用于真失败，与未命中互斥。
- §3.5.6（`:603-607`）：`ReqAiKeyAffinityMiss` = "未命中 Redis 绑定次数"；`ReqAiKeyAffinityRedisErr` = "Redis 操作失败次数" → 断言 A-02/A-03。

## 八、实施记录（2026-11-03）

改动文件（`bfe/`，主线 `v1.9.0-dev`，HEAD `484f830a`）：

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | 新增 `redisGetOrMiss`（`:2731-2747`）；`redisGetBinding`（`:2749-2760`）与 `redisGetBatchBinding`（`:2676-2682`）改用之 |
| `bfe_server/reverseproxy_ai_test.go` | `mockRedisClient.Get` 键缺失返回 `redis.ErrNil`（对齐生产）；新增 `TestRedisGetBinding_MissIsNotError`、`TestChooseAIKeyWithAffinity_MissNotRedisErr`、`TestRedisGetBatchBinding_MissNotRedisErr` |
| `docs/zh_cn/sys_design/multi_api_key.md` | §3.5.6 指标表补未命中语义脚注 |

单元测试（红-绿实证）：临时移除 `IsKeyNotFound` 归一后，`TestChooseAIKeyWithAffinity_Miss`、`TestRedisGetBinding_MissIsNotError`、`TestChooseAIKeyWithAffinity_MissNotRedisErr`、`TestRedisGetBatchBinding_MissNotRedisErr`、`TestChooseAIKeyWithAffinity_PenaltySkip`、`TestChooseAIKeyWithAffinity_CircuitBreak` 六个用例变红（其中 `PenaltySkip` / `CircuitBreak` 为既有用例，证明它们此前被同一提前 return 短路）；恢复归一后 `go test ./bfe_server/` 全绿。

全仓检查：`go build ./...`、`go vet ./...` 通过；`go test ./...` 的失败仅两处且与本次改动无关——`tests/integration/implementation/scenario-SC13-tls-conf-reload-path`（Windows 路径/大小写问题，本仓已有同主题未合入文档）与 `tests/integration` SC01 首次运行时 bfe 二进制被占用（缓存文件锁，重跑即消失）。

集成回归（`bfe/tests/integration`，实跑）：

- 场景 `scenario-SC10-ai-key-session-affinity` 新增 3 个用例，testEnv 补 `bfeMonitorPort`（原先丢弃）与 `affinityState()` / `bindingProviderKey()` 等 helper，计数器经 `common.ModuleMetric(port, "proxy_state", field)` 读取：
  - **TC-09** 首触为未命中：`MISS +1`、`REDIS_ERR +0`、无 `redis get binding error`、绑定已写入；第二条请求 `HIT +1` 且沿用同一 Provider Key；
  - **TC-10** 真故障与未命中互斥：`RedisServer.Fault()` 后 `REDIS_ERR` 增长、`MISS` 不变、WARN 保留；`Recover()` 后重新未命中并写回绑定；
  - **TC-11** 首请求失败仍绑定：首条请求上游 404（避开 fallback 与 Key 轮换），绑定已存在，下一条请求粘住同一 Key。
- 三个用例的红-绿实证：把 `IsKeyNotFound` 归一临时去掉并强制重建 bfe 二进制后，TC-09 报 `MISS=0 / REDIS_ERR=1 / 出现 WARN`、TC-10 报 `MISS=0`、TC-11 报 `绑定不存在`——与 issue 现场观测一致；恢复修复后 SC10 全部 11 个用例 PASS。
- 设计文档：`测试设计文档/scenario-SC10-同cluster多API-Key会话保持/` 新增 `TC-09`/`TC-10`/`TC-11` 三份用例文档，并在 `场景说明.md` 的用例列表、详细说明与公共基础设施一节同步（`RedisServer.Fault/Recover`、`common/monitor.go` 计数器读取）。

集成回归（`integration-test/`，实跑）：

- 新增 `common/monitor_state.go`（`FetchProxyStateCounters` / `CounterDelta`）；`sc18_cluster_multi_apikey_conf_agent_test.go` 的 env 保留 miniredis 句柄与 `bfeLogDir`，新增 `affinityBindings` / `bfeLogLines`；
- `TestTC17_ConfAgentKeyAffinitySameClientKey` 实跑 **PASS**，断言全部成立：首发请求 `MISS +1`、`REDIS_ERR +0`、写入一条绑定（值为 key-a/b/c 之一）；30 条请求后 `HIT +29`、`REDIS_ERR +0`、后端只见一种 `Authorization`；第二个 API-Key `MISS +1`、绑定数 2；BFE 日志无 `redis get binding error`；
- 整个 SC18 包（17 个用例，含 429 轮换 / 401·403 判死 / 耗尽 fallback / 粘性）实跑 **PASS**（193s），无回归。

未在本卡完成：`v1.8.8` 分支回移（需切分支，工作区含未提交文档改动，另次提交处理）；真 Redis 故障的进程级集成（miniredis 无法构造读超时，由 `failGet` 单测覆盖）。