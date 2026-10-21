# BFE Issue #1440 修复方案：Key 耗尽返回 (nil, nil) 导致 cluster fallback 失效

- Issue: https://github.com/bfenetworks/bfe/issues/1440
- 生产事件编号：prod-20261021-02
- 缺陷：`aiClusterInvoke()` 在**首次选 Key 就失败**（`all ai keys exhausted`）时以 `res=nil, err=nil` 返回。外层 `ServeHTTPForAI()` 的成功判定要求 `res != nil`（`:1316`），降级判定 `shouldTriggerFallback(nil, nil, ...)` 得到 `status=0`（`getResponseStatus(nil)` 返回 0），既不 `>= 500` 也不在 `aiFallbackStatusCodes{400,401,402,403,422,429}` 中 → `false` → 循环 `break`，**`ai_route.data` 中配置的备用集群一次都没有被尝试**。请求随后落入 `err != nil || res == nil` 分支（`:1359`），被 `CreateInternalSrvErrResp` 合成为空 body 500。对照组：同窗口 16 笔真正收到 429 的请求 16/16 全部降级成功，证明降级通道本身完好，唯一失效点就是 `res==nil` 路径。
- 代码库：`bfe/` **v1.8.8 分支**（本文行号逐条核实于该分支）。
- 关联：#1439（Key 惩罚过滤缺陷，是本缺陷最常见的上游触发源；两者叠加把 429 退化为 500）、#1442（单可用 Key 的容量前提）。

## 一、根因（源码定点）

调用链（`bfe_server/reverseproxy.go`）：

1. `:1829-1834`：首次选 Key 失败 → `log.Warn("aiClusterInvoke: all ai keys exhausted ...")` + `break`，本轮未调用 `doSingleAIForward`，命名返回值 `res`、`lastErr` 从未赋值，均为 nil。
2. `:1903`：`return res, action, cluster, lastErr, newBodyModel` → `(nil, ..., nil)`；该分支未设置 `basicReq.ErrCode`（访问日志 `err_code=""` 与此互证）。
3. `:1316`：`invokeErr == nil && res != nil && res.StatusCode < 400` 不成立（`res==nil`）。
4. `:1339`：`shouldTriggerFallback(res, invokeErr, style)`（`:1919-1941`）：`err==nil` 跳过第一个 true 分支；`code := getResponseStatus(nil)`（`:1943-1947` 返回 0）；`0 >= 500` 不成立；`0` 不在 `aiFallbackStatusCodes`（`:1910-1917`）→ 返回 `false` → 外层 `break`。
5. `:1359-1369`：`res == nil` → `BfeStatusCode = 500` + `CreateInternalSrvErrResp`；`bfe_basic/common.go:77-84` 只设 `StatusCode`、`Server: bfe` 和空 body，无错误结构、无 `ErrCode` → 空 body 500。

**契约违反点**：设计文档 `2026-08-19-ai-fallback-status-codes/design-changes.md` §2.3「key 耗尽后把最终响应交给外层决策」与 §4.3「当 key 耗尽……`aiClusterInvoke` 把最终响应（可能为 4xx）返回给外层。外层 `shouldTriggerFallback` 再决定是否继续 cluster 级 fallback」要求 Key 耗尽必须返回**可判定的最终结果**；实现返回 `(nil, nil)`，外层无从判定。属控制流与已批准设计不一致。

## 二、修复方案

设计原则：

1. **Key 耗尽必须产出可判定的结果**——恢复 §2.3/§4.3 的契约：`aiClusterInvoke` 的调用者应总能依据 `(res, err)` 做出 fallback 判定；
2. **语义对齐**：Key 耗尽的本质是"上游侧容量不足/被限流"，对外应呈现 **429**（可重试、命中 fallback 白名单），而不是 500（网关内部错误）；
3. **不改变 no route / no cluster 等已有失败路径**——那些路径有自己的 `ErrCode` 与响应语义（见边界说明）；
4. **`ErrAiRateLimit` 本地限流标记语义不被污染**——合成响应**不得**设置 `basicReq.ErrCode = ErrAiRateLimit`，否则 `:1336-1338` 会把它当作"本地限流拒绝"而**禁止** fallback，方向正好相反。

### 步骤 1（必改，核心）：Key 耗尽时返回合成的 429 响应

`bfe_server/reverseproxy.go` 的 `aiClusterInvoke`，在选 Key 失败分支：

```go
if !keepKey {
    idx, key, boundName, ok = chooseAIKeyWithAffinity(cluster.Name, keys, policy, state, redisClient, sessionID, p.proxyState)
    if !ok {
        // No usable key could be selected (all penalized / all weight 0 /
        // all dead). Return a synthetic 429 so the outer fallback decision
        // can act on a decidable result (issues #1439/#1440): a configured
        // fallback cluster is tried, and without one the client gets a
        // retryable 429 instead of an opaque 500.
        log.Logger.Warn("aiClusterInvoke: all ai keys exhausted for cluster[%s], return synthetic 429", attempt.ClusterName)
        res = newAIKeysExhaustedResponse()
        return res, action, cluster, nil, newBodyModel
    }
    ...
}
```

新增 helper（与 `bfe_basic.CreateInternalSrvErrResp` 同风格，但不设 `ErrCode`、不置 `BfeStatusCode=500`）：

```go
// newAIKeysExhaustedResponse builds a synthetic 429 response for the
// "no AI key could be selected" case. It deliberately does NOT set
// basicReq.ErrCode: ErrAiRateLimit marks a *local* rate-limit rejection
// which must suppress cluster fallback (reverseproxy.go:1336); key
// exhaustion is an upstream-capacity signal and must ALLOW fallback.
func newAIKeysExhaustedResponse() *bfe_http.Response {
    res := bfe_basic.CreateInternalResp(basicReq /* 或无 req 变体 */, bfe_http.StatusTooManyRequests)
    // body 可选：写一个最小 JSON 错误体，便于客户端与排障区分
    // {"error":{"type":"rate_limit_error","message":"no available upstream api key"}}
    return res
}
```

实现细节取舍：

- 若沿用 `CreateInternalSrvErrResp(basicReq)` 会附带 `BfeStatusCode=500` 的访问日志语义，不可取；建议参照 `bfe_basic/common.go:77-84` 新增一个返回指定状态码的 helper（或在 `bfe_server` 内本地构造 `bfe_http.Response{StatusCode: 429, ...}`，body 用最小 JSON，Header 设 `Content-Type: application/json`、`Server: bfe`）；
- 保留 `all ai keys exhausted` WARN 日志（issue 证据链依赖该指纹），追加 `return synthetic 429` 使日志自解释；
- 重试轮内的耗尽（`retry>0` 后 `chooseAIKeyWithAffinity` 失败）走同一分支，行为一致。

### 步骤 2（必改）：fallback 循环消费合成响应

`ServeHTTPForAI()` 的循环（`:1306-1350`）**无需改动**：合成 429 使 `:1316` 成功判定不成立、`:1339 shouldTriggerFallback` 因 429 ∈ `aiFallbackStatusCodes` 返回 `true` → 打印 `fallback triggered, cluster[kimi] err[<nil>] status[429]` → 关闭 res body → 进入下一备用集群尝试。若已是最后一个 attempt 或未配置 fallback，循环结束后 `res` 即合成 429，直接作为最终响应返回给客户端（而非 `:1359` 的 500 分支）。

需核对的一个细节：响应写出路径（`response_got` 之后）对 body 为 `EofReader`/JSON body、`ContentLength` 的处理是否与常规上游响应一致；合成响应若带小 JSON body，需正确设置 `ContentLength` 或走 chunked，避免 hang。

### 步骤 3（已确认执行）：`shouldTriggerFallback` 对 nil/nil 的防御

**决策**：执行。`(nil, nil)` 是"不可判定的结果"，不应静默吞掉降级机会——任何调用点出现该形态一律视为可降级（fallback-eligible）。步骤 1 保证 `aiClusterInvoke` 不再产生该形态，本步骤把它固化为**不变量**：未来新增的调用路径即使漏构造返回值，最坏也是多尝试一个备用集群，而不是退化成 500。

```go
func shouldTriggerFallback(res *bfe_http.Response, err error, authStyle string) bool {
    if err != nil {
        return true
    }
    if res == nil {
        // (nil, nil) is not a decidable result; treat as fallback-eligible
        // as a hard invariant (issue #1440, step 3). aiClusterInvoke no
        // longer returns this shape after step 1; this branch is defense
        // for any future call path.
        return true
    }
    ...
}
```

**配套决策（消除对既有路径的影响）**：

1. `:1336-1338` 的 `basicReq.ErrCode == bfe_basic.ErrAiRateLimit → break` 短路**保持不变**，且仍位于 `:1339` 之前——本地限流拒绝（local RL）是"有意不放行降级"的语义，优先于本步骤的 nil/nil 判定；
2. **no route / no cluster 路径不在影响范围内**：这两条路径在进入 fallback 循环**之前**就以本地响应 + `ErrCode` 退出（`:1221-1232` 的 `AI route not found` 等），根本走不到 `:1339`，因此本步骤不会改变它们的对外行为；
3. 实施时复核一遍所有能到达 `:1339` 的调用路径，确认除 `aiClusterInvoke` 外没有其它 `res==nil && err==nil` 的合法形态；若有，为其补 `ErrCode` 或改为返回可判定结果，而不是依赖本分支兜底（本分支只兜底，不豁免设计义务）。

## 三、回归测试

单元测试：

1. `aiClusterInvoke`（mock 集群 2 Key 均 Weight=0 或全部 dead）：断言返回 `res != nil && res.StatusCode == 429`、`err == nil`、`basicReq.ErrCode` 未被设置；
2. `ServeHTTPForAI` 集成面（mock 2 集群，第 1 集群无可用 Key，第 2 集群正常）：断言 fallback 被触发、最终响应来自第 2 集群（2xx）；仅 1 个集群时断言客户端收到 429 而非 500、body 非空、`res_status_code=429`；
3. `shouldTriggerFallback(nil, nil, style)` → `true`（步骤 3 不变量）；`shouldTriggerFallback(res429, nil, style)` → `true`、`shouldTriggerFallback(res400, nil, style)` → `true`、`shouldTriggerFallback(res404, nil, style)` → `false`（白名单口径不变）；`basicReq.ErrCode == ErrAiRateLimit` 时在 `:1336` 即 break，不进入本判定（local RL 语义回归）。

集成测试（SC02 多 API-Key 场景，与 #1439 的 TC 合并或紧邻）：

- 复刻 issue 复现步骤：target 集群（2 Key：1 可用 + 1 Weight=0）+ `SessionAffinityPenaltyEnable=true` + mock 上游对可用 Key 返回 429 + `ai_route.data` 配置 fallback 到备用集群 → 断言：惩罚窗口内请求 **命中备用集群并成功**（出现 `fallback triggered ... status[429]` 日志），而非 0 上游交互的 500。

## 四、修复后目标行为

以现场 `yf-coding-peak-kimi`（target kimi + fallback bailian）为例：

1. kimi 无可用 Key → `aiClusterInvoke` 返回合成 429（WARN 日志一条）；
2. 外层 `shouldTriggerFallback` 判定 429 命中白名单 → 打印 `fallback triggered` → 尝试 bailian；
3. bailian 成功 → 客户端收到 2xx（issue 现场 932 笔 500 中的绝大部分本可走到这一步）；bailian 也失败/未配置 → 客户端收到**透传的 429**（可重试），而非空 body 500；
4. 16/16 对照组行为不变（真实 429 的降级路径原已工作）。

## 五、边界与残余风险

- **(nil, nil) 形态双重封闭**：步骤 1 从产生侧保证 `aiClusterInvoke` 的"无 Key 可选"出口有确定状态码；步骤 3 从消费侧把 `res==nil && err==nil ⇒ fallback-eligible` 固化为不变量，未来新增调用路径漏构造返回值时兜底生效。no route / no cluster 等**更早**的失败路径在进入 fallback 循环前就以本地响应/ErrCode 退出，不受步骤 3 影响。
- **合成 429 的对外语义**：客户端看到的是 429 + JSON 错误体，与真实上游 429 形态接近但不携带上游的 `Retry-After`/`x-ratelimit-*` 头。若产品要求区分"网关侧无 Key"与"上游限流"，可在 JSON body 的 `error.type` 中区分（如 `upstream_key_exhausted`），不影响状态码与 fallback 判定。
- **与 #1387 sister card 的交互**：`:1865-1867`（local RL 直接返回，不轮转不打惩罚）与 `:1336-1338`（local RL 禁止 fallback）两条规则依赖 `ErrCode == ErrAiRateLimit` 标记；本方案的合成响应**不设**该标记，因此不会被误判。新增单测需覆盖"合成 429 不触发 ErrAiRateLimit 短路"这一断言。
- **v1.8.9-dev 同步**：该分支 `ServeHTTPForAI`/`aiClusterInvoke` 行号不同，且新增上游错误归一化（`reverseproxy_ai_error.go`）可能改写 429 形态；移植时核对合成响应是否会与归一化器叠加（应避免双重包装）。

## 六、实施记录（2026-10-21，v1.8.8 分支）

| 文件 | 改动 |
| --- | --- |
| `bfe_server/reverseproxy.go` | `aiClusterInvoke` 选 Key 失败分支：删除 `break`，改为经 `bfe_basic.CreateSpecifiedContentResp` 返回合成 429（`application/json`，body `{"error":{"type":"rate_limit_error","message":"no available upstream api key"}}`，自带 Content-Length），不设 `ErrCode`；WARN 日志追加 `, return synthetic 429`。`shouldTriggerFallback` 新增 `res == nil → true` 分支（步骤 3 不变量，注释说明与 ErrAiRateLimit 短路的次序关系） |
| `bfe_server/reverseproxy_ai_test.go` | `TestShouldTriggerFallback` 新增 `(nil, nil) ⇒ true` 断言；新增 `TestAiClusterInvokeExhaustionReturnsSynthetic429`（唯一 Key Weight=0 驱动耗尽路径，断言 429/无 ErrCode/JSON Content-Type） |
| `docs/zh_cn/sys_design/multi_api_key.md` | §3 伪代码的耗尽分支同步为"返回合成 429"；§4.2 新增「Key 耗尽契约」与「nil/nil 不变量」两段；§5 关键日志补充带 `return synthetic 429` 后缀的日志行 |
| `tests/integration/测试设计文档/scenario-SC02-.../TC-09-首次选Key失败返回合成429并降级.md` | 新增 TC-09 设计文档（臂 A 有 fallback / 臂 B 无 fallback） |
| `tests/integration/测试设计文档/scenario-SC02-.../场景说明.md` | 场景目的、TC 列表（8→9）、§4.2 路由表设计补充臂 B 路由说明 |
| `tests/integration/测试设计文档/测试场景总体说明.md` | SC02 TC 数 8→9，TC 对应表新增 TC-09 |
| `tests/integration/implementation/scenario-SC02-multi-api-key/sc02_multi_api_key_test.go` | `sendRequest` 泛化为 `sendRequestToHost`；新增 `bfeLogContains`；新增 `TestTC09_FirstSelectionFailureSynthetic429`（两个子测试） |
| `tests/integration/implementation/scenario-SC02-multi-api-key/testdata/` | `host_rule.data` 新增 `multikey-nofallback.example.org`；`ai_route.data` 新增无 fallbacks 规则 `user_a-multikey-nofallback` |

**与本文档的偏差**：未新建独立 helper `newAIKeysExhaustedResponse`，直接使用 `bfe_basic.CreateSpecifiedContentResp`（自带 Content-Length/body，无需额外处理写出路径），行为与方案一致。

**验证结果（本次实跑）**：

| 范围 | 结果 |
| --- | --- |
| `go build` / `go vet`（bfe_server、SC02 测试包） | 通过 |
| `bfe_server` / `mod_ai_route` / `mod_ai_token_auth` 单测 | 全部通过 |
| SC02 全量（TC-01 ~ TC-09） | 通过 |
| SC07（AI 限流 Redis 键）、SC08（AI 模型 fallback 重算） | 通过 |

**未实施项**：无（步骤 1/2/3 全部落地；步骤 2 经核对无需改动 `ServeHTTPForAI` 循环）。

**遗留事项**：v1.8.9-dev 分支移植时需按 §五 末条复核（行号差异 + 上游错误归一化叠加风险）；#1439/#1442 的处置按各自方案推进。
