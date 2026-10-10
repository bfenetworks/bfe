# BFE EPP 非调度参数热更新修复

> 对应 issue：[issue 218] (https://github.com/rainway-ai-gateway/ai-gateway-api/issues/218)
> 前置文档：`bfe/docs/zh_cn/modifications/2026-09-06-epp-ai-gateway-integration/`（EPP 对接方案）、`2026-09-14-epp-plaintext-dial/`（明文拨号）

## 1. 背景与目标

EPP（ext-proc）路径的非调度类参数——`GslbBasic` 下的 `EPPTLS`、`EPPCheck`、`EPPTimeout`、`EPPBreaker`——在 `server_data_conf` / `cluster_conf.data` reload 后不能正确生效：控制面改了配置、BFE 表面上 reload 成功，但 EPP 的 **data 连接**与 **health 探针**仍在用旧参数工作。调度类参数（`EPPAddr`、`BalanceMode`）的热更新语义此前已实现，本次补齐非调度参数。

**目标**：让下列参数在 reload 后按新配置工作，且不破坏既有平滑切换语义与熔断状态。

**修复范围（本设计）**：

| 编号 | 问题 | 根因 |
| --- | --- | --- |
| **D1** | `EPPTLS.{Insecure,CAFile,Plaintext}` 热变更不生效：data 连接仍用旧凭据；probe 用新 `plaintext` 但旧 `tlsConf`，两通道形态可能不一致 | `updateConf` 只替换 `rt.conf`，不重建 `rt.tlsConf`；凭据在 `NewGrpcConn` 建连时固化 |
| **D2** | `Plaintext=true→false` 时 `rt.tlsConf` 仍为 nil，probe 走 `credentials.NewTLS(nil)` 异常 | `newEPPRuntime` 在 `plaintext` 时跳过 `BuildTLSConfig`，原地 `updateConf` 不重建 |
| **D3** | `EPPCheck.Disabled` 热切换不生效（健康检查循环不启/不停） | `healthLoop` 是否启动只在 `newEPPRuntime` 判断一次 |
| **D4** | `EPPTimeout.Connect` 对已有 data 连接不生效 | `MinConnectTimeout` 只在 `NewGrpcConn` 设置，连接不重建则沿用旧值 |
| **D5** | 每次 reload 无条件 `eppBreaker.updateConf`，清空熔断滑动窗口 | `updateConf` 总是 `results=make(...); count=0; idx=0` |

**涉及代码位置**：`bfe_balance/bal_gslb/bal_gslb.go`、`bfe_balance/bal_gslb/epp_runtime.go`、`bfe_balance/bal_gslb/epp_breaker.go`、`bfe_util/epp/epp_client.go`。

## 2. 现状盘点

热更新入口链路：配置 reload → `BalTable.SetGslbBasic`（`bfe_balance/bal_table.go`，对所有 cluster 全量遍历）→ `BalanceGslb.SetGslbBasic`（`bal_gslb.go:92`）→ 若 `BalanceMode==EPP` 且有地址则 `initEPP`，否则 `closeEPP`。

| 位置 | 现状 | 问题 |
| --- | --- | --- |
| `bal_gslb.go:92` `SetGslbBasic` | 按 `BalanceMode` 分发到 `initEPP` / `closeEPP` | 分发正确，问题在下游 |
| `bal_gslb.go:203` `initEPP` | **是否重建 runtime 只由 `bal.eppRt.sameAddrs(addrs)` 决定**（`:222`）；未换地址则一律 `rt.updateConf(conf)` | 连接型/生命周期型参数变化被吞掉（D1-D4） |
| `bal_gslb.go:135` `buildEPPRuntimeConf` | 从 `gslbBasic` 提取全部运行参数（含 `plaintext`、`connectTimeout`、`checkDisabled`） | 参数已就绪，缺的是按参数类别决定更新方式 |
| `epp_runtime.go:137` `updateConf` | 仅 `rt.conf = conf` | 不重建 `tlsConf` / conns / healthLoop |
| `epp_runtime.go:123` `sameAddrs` | 仅比较地址表 | 判据过窄 |
| `epp_runtime.go:80` `newEPPRuntime` | `plaintext` 时跳过 `BuildTLSConfig`；`checkDisabled` 决定是否 `go healthLoop`；per-addr `NewGrpcConn` 固化凭据与 `MinConnectTimeout` | 这些动作只在建 runtime 时执行一次 |
| `epp_runtime.go:220` `probe` | `credentials.NewTLS(rt.tlsConf)` | `tlsConf` 为 nil 时不防御（D2） |
| `epp_breaker.go:83` `updateConf` | 归一化后总是重建 `results`、清零 `count/idx`（保留 `state`/`openedAt`/`halfOpenInflight`） | 配置未变也重置窗口（D5） |
| `epp_breaker.go:61` `newEppBreaker` | 内联归一化（window/minVolume/errorRate/openTimeout） | 与 `updateConf` 归一化逻辑重复 |

> 已有优雅退出机制：`retireEPPLocked`（`bal_gslb.go:243`）先 `stopProbes()` 立即停探针，`eppRetireGrace`（默认 60s）后 `closeConns()` 关旧连接，保证在途请求不被中断。本次修复复用该机制。

## 3. 设计总览

引入 **"运行时签名（`runtimeSignature`）"** 替代"仅比较地址表"，把**连接型/生命周期型**参数纳入"需要重建 runtime"的判据；读取型参数仍走原地 `updateConf`。

| 参数类别 | 参数 | 更新方式 |
| --- | --- | --- |
| 连接型（建连时固化） | `EPPTLS.{Insecure,CAFile,Plaintext}`、`EPPTimeout.Connect` | **纳入签名 → 重建 runtime**（新 `tlsConf` + 新 conns），旧 runtime grace-retire |
| 生命周期型 | `EPPCheck.Disabled` | **纳入签名 → 重建 runtime**（按新值起停 `healthLoop`） |
| 拓扑型 | `EPPAddr` | 纳入签名（地址变 → 重建，保持现有语义） |
| 读取型（每请求/每轮读取） | `EPPTimeout.Call`、`EPPCheck.{CheckInterval,FailThreshold,Cooldown,SuccessThreshold}` | 原地 `rt.updateConf`（仅替换 `rt.conf`） |
| 状态型 | `EPPBreaker.*` | `eppBreaker.updateConf`，**未变化直接返回** |

一句话：**连接/生命周期参数变化 → 重建 runtime（复用现有 grace-retire 平滑切换）；其余 → 原地更新；熔断配置未变不动窗口。**

## 4. 详细设计

### 4.1 新增 `runtimeSignature`（`epp_runtime.go`）

```go
// bfe_balance/bal_gslb/epp_runtime.go（新增）
type eppRuntimeSignature struct {
	addrs          string // strings.Join(addrs, ",") —— 含地址顺序
	tlsInsecure    bool
	tlsCAFile      string
	plaintext      bool
	connectTimeout time.Duration
	checkDisabled  bool
}

func runtimeSignature(addrs []string, conf eppRuntimeConf) eppRuntimeSignature {
	return eppRuntimeSignature{
		addrs:          strings.Join(addrs, ","),
		tlsInsecure:    conf.tlsInsecure,
		tlsCAFile:      conf.tlsCAFile,
		plaintext:      conf.plaintext,
		connectTimeout: conf.connectTimeout,
		checkDisabled:  conf.checkDisabled,
	}
}
```

在 `eppRuntime` 增加不可变字段 `sig`，由 `newEPPRuntime` 设置：

```go
type eppRuntime struct {
	name    string
	addrs   []string
	conf    eppRuntimeConf
	tlsConf *tls.Config
	sig     eppRuntimeSignature // 新增：建 runtime 时的连接/生命周期签名（不可变）

	mu     sync.Mutex
	conns  []*grpc.ClientConn
	active int
	health []eppAddrHealth
	...
}

func (rt *eppRuntime) sameSignature(sig eppRuntimeSignature) bool {
	return rt.sig == sig // sig 只读，无需加锁
}
```

> `eppRuntimeSignature` 全部为可比较类型（string/bool/time.Duration），可直接 `==`。

### 4.2 改造 `initEPP`：签名比较决定"重建 or 原地"（`bal_gslb.go:203`）

```go
func (bal *BalanceGslb) initEPP(addrs []string, gslbBasic cluster_conf.GslbBasicConf) error {
	conf := buildEPPRuntimeConf(gslbBasic)
	breakerConf := buildEPPBreakerConf(gslbBasic)

	bal.eppMu.Lock()
	defer bal.eppMu.Unlock()

	if len(addrs) == 0 {
		bal.closeEPPLocked()
		return nil
	}

	// 熔断配置：未变化时 updateConf 内部直接返回（D5）
	if bal.eppBreaker == nil {
		bal.eppBreaker = newEppBreaker(bal.name, breakerConf)
	} else {
		bal.eppBreaker.updateConf(breakerConf)
	}

	newSig := runtimeSignature(addrs, conf)
	if bal.eppRt != nil && bal.eppRt.sameSignature(newSig) {
		// 仅读取型参数（callTimeout / check 滞回）：原地更新
		bal.eppRt.updateConf(conf)
		return nil
	}

	// 连接型/生命周期型/地址变化：重建 runtime
	rt, err := newEPPRuntime(bal.name, addrs, conf)
	if err != nil {
		return err
	}
	if bal.eppRt != nil {
		rt.inheritStateFrom(bal.eppRt) // addrs 相同则保留 active/health（见 4.3）
	}
	old := bal.eppRt
	bal.eppRt = rt
	if old != nil {
		bal.retireEPPLocked(old) // 现有机制：stopProbes 立即；closeConns 60s 后
	}
	return nil
}
```

要点：`sameAddrs` 判据升级为 `sameSignature`。**地址不变但 `EPPTLS`/`EPPTimeout.Connect`/`EPPCheck.Disabled` 变化时也会重建 runtime**，从而一并修复 D1/D2/D3/D4。

### 4.3 重建时继承状态（降低抖动）

地址表未变时，重建不应丢失 failover 状态（`active`/`health` 按相同下标一一对应）：

```go
// epp_runtime.go（新增）
func (rt *eppRuntime) inheritStateFrom(old *eppRuntime) {
	if old == nil || rt.sig.addrs != old.sig.addrs {
		return // 地址表不同：下标不再对应，不继承
	}
	old.mu.Lock()
	active := old.active
	health := append([]eppAddrHealth(nil), old.health...)
	old.mu.Unlock()

	rt.mu.Lock()
	if len(health) == len(rt.health) {
		rt.active = active
		rt.health = health
	}
	a := rt.active
	rt.mu.Unlock()
	eppProm.activeAddr.WithLabelValues(rt.name).Set(float64(a))
}
```

> 效果：改 `EPPTLS` / `EPPTimeout.Connect` / `EPPCheck.Disabled`（地址不变）时不重置 `active`，避免 failover 状态丢失与瞬时抖动。

### 4.4 修复 D1 / D2：TLS 与明文一致性

- **D1**：`EPPTLS` 变化现在触发重建，`newEPPRuntime` 用新配置重建 `tlsConf`，并为每地址重建 `NewGrpcConn`（新凭据）；probe 用新 `rt.tlsConf` / `rt.conf.plaintext`。data 与 health 两通道一致。
- **D2**：重建路径下，`plaintext` 由 true→false 时 `newEPPRuntime` 会执行 `BuildTLSConfig`，`rt.tlsConf` 不再为 nil。另加**防御性校验**，即使契约被破坏也不 panic：

```go
// epp_runtime.go:220 probe 内
var creds credentials.TransportCredentials
if rt.conf.plaintext {
	creds = insecure.NewCredentials()
} else {
	if rt.tlsConf == nil { // 不应发生（签名保证）；防御
		return fmt.Errorf("epp: TLS config missing while plaintext=false")
	}
	creds = credentials.NewTLS(rt.tlsConf)
}
```

> 因为 `updateConf` 现在**只在签名不变（即 TLS 未变）时**被调用，`rt.conf.plaintext` 与 `rt.tlsConf` 始终一致，从根本上消除"新 flag + 旧 conf"与"TLS flag + nil conf"。

### 4.5 修复 D3：`EPPCheck.Disabled` 热切换

`checkDisabled` 纳入签名 → 切换 `Disabled` 时重建 runtime：`newEPPRuntime`（`epp_runtime.go:113`）依据新 `conf.checkDisabled` 决定 `go rt.healthLoop()` 或 `close(rt.doneCh)`；旧 runtime 的 loop 由 `retireEPPLocked` → `stopProbes()` 停止。

```go
// epp_runtime.go:113 newEPPRuntime 内（已有逻辑，重建时即生效）
if !conf.checkDisabled {
	go rt.healthLoop()
} else {
	close(rt.doneCh)
}
```

> 备选方案（不重建、原地起停）：为 `eppRuntime` 增加 `probeCtl chan struct{}` / atomic 标志，在 `updateConf` 内控制 `healthLoop` 起停。本设计选"纳入签名重建"，与 TLS 处理统一、实现更简单；重建时用 `inheritStateFrom` 保留状态。

### 4.6 修复 D4：`EPPTimeout.Connect` 热更新

`connectTimeout` 纳入签名 → 变化时重建 runtime，`NewGrpcConn`（`epp_client.go:80`）以新 `MinConnectTimeout` 建连（D4 修复）；probe 亦读新值。地址表相同时 `inheritStateFrom` 保留 `active`/`health`。

### 4.7 修复 D5：`eppBreaker.updateConf` 未变化直接返回

`eppBreakerConf` 字段均为可比较类型（bool/int/time.Duration），归一化后直接比较：

```go
// bfe_balance/bal_gslb/epp_breaker.go
func (b *eppBreaker) updateConf(conf eppBreakerConf) {
	b.mu.Lock()
	defer b.mu.Unlock()

	conf = normalizeEppBreakerConf(conf) // 与 newEppBreaker 共用同一归一化
	if b.conf == conf {
		return // ★ 未变化：保留窗口、计数与状态，避免每次 reload 被重置
	}

	b.conf = conf
	b.results = make([]bool, conf.windowSize)
	b.count = 0
	b.idx = 0
	// 保持 state / openedAt / halfOpenInflight 现状
}

// 抽出与 newEppBreaker 相同的归一化（避免逻辑分叉）
func normalizeEppBreakerConf(conf eppBreakerConf) eppBreakerConf {
	if conf.windowSize < 1 { conf.windowSize = 1 }
	if conf.minVolume < 1 { conf.minVolume = 1 }
	if conf.errorRatePercent < 1 || conf.errorRatePercent > 100 { conf.errorRatePercent = 50 }
	if conf.openTimeout <= 0 { conf.openTimeout = 30 * time.Second }
	return conf
}
```

`newEppBreaker`（`epp_breaker.go:61`）也改用 `normalizeEppBreakerConf`（保持与 `updateConf` 判据一致）。这样 `BalTable.SetGslbBasic` 每次 reload 全量遍历时，未变更的集群不会重置熔断窗口。

> 可选增强：若**仅 `Disabled` 变化**，可只更新 `b.conf` 而不清空窗口，或重置 `halfOpenInflight`。本期不做，保持最小改动。

## 5. 行为变化与兼容性

| 场景 | 变更前 | 变更后 |
| --- | --- | --- |
| 仅改 `EPPTimeout.Call` / `EPPCheck` 其余字段 | 原地 `updateConf` | 不变（仍原地，下个请求/下轮探测生效） |
| 改 `EPPTLS` / `EPPTimeout.Connect` / `EPPCheck.Disabled`（地址不变） | **不生效**（D1-D4） | **重建 runtime 生效**；`active`/`health` 继承（地址不变） |
| 改 `EPPAddr` | 重建 runtime（active 归零） | 不变（重建、active 归零，地址不同不继承） |
| reload 但配置未变 | 熔断窗口被重置（D5） | **窗口保留**（`updateConf` 早返回） |
| EPP/非 EPP 模式切换 | `initEPP`/`closeEPP` | 不变 |

**兼容性**：

- 签名包含地址串，地址顺序变化视为变化（与现有 `sameAddrs` 语义一致）。
- `EPPTLS.CAFile` 的**内容**变化但路径不变时，签名无法感知（签名只含路径字符串）。因此内容变更需通过"路径版本化"（CA 文件路径带版本目录/文件名，内容变化即路径变化）来触发签名变化；若上游使用固定路径，则内容更新需另行触发（例如轮换路径或重启节点）。
- 重建 runtime 会新建 conns 并 grace-retire 旧 conns（`eppRetireGrace`，默认 60s）；在途请求在 grace 内继续用旧连接，行为与现有地址变更一致。
- 本变更只影响 BFE 二进制行为，不改变配置格式；旧 `cluster_conf.data`（无新字段）加载行为不变。

## 6. 灰度与升级顺序

- 由于**旧 BFE 二进制遇到含 `Plaintext` 的 `EPPTLS` 会因未知字段退化为 `{Insecure:false, CAFile:""}` 并被旧校验拒绝**，本 BFE 版本应**先于控制面导出 `Plaintext`** 上线（即"先升级 BFE、再下发明文"）。
- 滚动升级兼容：升级后遇到不含新字段的旧 `cluster_conf.data` 时，签名与现状一致，行为不变。

## 7. 测试计划

| 用例 | 断言 |
| --- | --- |
| `TestEPPRuntimeHotUpdate_TLSToPlaintext` | 二次 `SetGslbBasic` 改 `EPPTLS.Plaintext`（地址不变）→ runtime 被替换、新 conn 以明文建连、probe 与 data 均明文 |
| `TestEPPRuntimeHotUpdate_PlaintextToTLS` | `plaintext=true → false`（地址不变）→ 新 runtime 的 `tlsConf != nil`，probe TLS 路径正常（覆盖 D2） |
| `TestEPPRuntimeHotUpdate_ConnectTimeout` | 改 `EPPTimeout.Connect` → runtime 替换、新 conn 的 `MinConnectTimeout` 为新值（覆盖 D4） |
| `TestEPPRuntimeHotUpdate_CheckDisabled` | 切换 `EPPCheck.Disabled` → 新 runtime 的 `healthLoop` 起/停符合新值；`active`/`health` 被继承（覆盖 D3） |
| `TestEPPRuntimeHotUpdate_ReadTypeInPlace` | 改 `EPPTimeout.Call` / check 滞回 → `assert.Same(rt, bal.getEPPRt())`（不重建） |
| `TestEPPRuntimeHotUpdate_AddrChange` | 改 `EPPAddr` → 重建、不继承状态、旧 runtime 进入 `eppRetired` |
| `TestEppBreaker_UpdateConfUnchanged` | 相同 conf 连续 `updateConf` → 窗口/计数不变（覆盖 D5） |
| `TestEppBreaker_UpdateConfChanged` | conf 变化 → 窗口按新 `windowSize` 重建 |
| 既有回归 | `TestEPPRuntimeFailoverFailback`、`TestEPPRuntimeAllDownStaysOnActive`、`TestSetGslbBasicEPPHotUpdate` 保持通过 |

> **注意**：改造后"相同地址表"不再等价于"复用 runtime"——只有**签名字段全部不变**时才复用。因此 `TestSetGslbBasicEPPHotUpdate`（`epp_runtime_test.go:496`）中"相同地址表 → runtime 复用（`assert.Same`）"的断言需按新语义更新：现用例仅改 `EPPTimeout.Call`（读取型），仍应复用；如用例改 `EPPTLS`/`connectTimeout`/`Disabled` 则应断言重建。`TestEppBreakerUpdateConf`（`epp_breaker_test.go:136`）也需补充"相同 conf 不重置窗口"的断言。

验证：`go build ./...` + `make test`（`go test -cover ./...` 与 `go vet ./...`）。

## 8. 代码变更清单

**`bfe_balance/bal_gslb/epp_runtime.go`**
- [x] 新增 `eppRuntimeSignature` 与 `runtimeSignature()`
- [x] `eppRuntime` 增 `sig` 字段；`newEPPRuntime` 设置 `sig`
- [x] 新增 `sameSignature()`、`inheritStateFrom()`
- [x] `probe` TLS 分支加防御性 nil 检查（D2）

**`bfe_balance/bal_gslb/bal_gslb.go`**
- [x] `initEPP` 判据由 `sameAddrs` 改为 `sameSignature`；签名变化时重建 + `inheritStateFrom`

**`bfe_balance/bal_gslb/epp_breaker.go`**
- [x] `updateConf` 归一化后未变化直接返回
- [x] 抽出 `normalizeEppBreakerConf`，`newEppBreaker` 复用

**测试**（`bfe_balance/bal_gslb/epp_runtime_test.go`、`epp_breaker_test.go`）
- [x] 新增/更新 §7 用例

**文档**
- [x] 本文档

> `bfe_util/epp/epp_client.go` 的 `NewGrpcConn` / `BuildTLSConfig` 本身不改（签名中已含其输入，重建即重建新连接）；仅在测试中按需使用。

## 9. 验收标准（热更新生效矩阵）

| 参数 | 变更后是否生效 | 生效时机 | 机制 |
| --- | --- | --- | --- |
| `EPPTLS.{Insecure,CAFile,Plaintext}` | 是 | 本次 reload | 签名重建（新 conns/tlsConf，data+probe 一致） |
| `EPPTimeout.Connect` | 是 | 本次 reload | 签名重建 |
| `EPPCheck.Disabled` | 是 | 本次 reload | 签名重建（loop 起停） |
| `EPPAddr` | 是 | 本次 reload | 签名重建（active 归零） |
| `EPPTimeout.Call` | 是 | 下个请求 | `updateConf` 原地 |
| `EPPCheck.{CheckInterval,FailThreshold,Cooldown,SuccessThreshold}` | 是 | 下轮探测 | `updateConf` 原地 |
| `EPPBreaker.*` | 是 | 下次调用 | `updateConf`（未变化不重置窗口） |
| `BalanceMode` | 是 | 本次 reload | `initEPP`/`closeEPP`（既有） |

**端到端验收（EPP 明文双通道 / A-dual-channel）**：控制面只改 `EPPTLS.Plaintext=true` 并 reload（地址不变），断言 BFE 的 data 连接与 health 探针**均**以明文建连（无需地址变更或重启）。

## 10. 一句话总结

把"是否重建 runtime"的判据从"只看地址"升级为 **`runtimeSignature`**（含 `EPPTLS`/`EPPTimeout.Connect`/`EPPCheck.Disabled`/`EPPAddr`），签名变化即重建 runtime（复用现有 grace-retire，并在地址不变时继承 `active`/`health`），其余参数仍原地 `updateConf`，从而一并修复 D1（TLS 不生效）、D2（plaintext→TLS 时 `tlsConf` 为 nil）、D3（Disabled 不生效）、D4（connectTimeout 不生效）；同时让 `eppBreaker.updateConf` 在配置未变化时直接返回，避免每次 reload 清空熔断窗口（D5）。

## 11. 参考资料

- issue：`yxy-note/rainway-ai-gateway/issues/218/218-BFE-EPP非调度参数热更新修复设计文档.md`
- `bfe_balance/bal_gslb/bal_gslb.go`（`SetGslbBasic` / `initEPP` / `retireEPPLocked` / `buildEPPRuntimeConf` / `buildEPPBreakerConf`）
- `bfe_balance/bal_gslb/epp_runtime.go`（`newEPPRuntime` / `sameAddrs` / `updateConf` / `probe` / `checkRound`）
- `bfe_balance/bal_gslb/epp_breaker.go`（`newEppBreaker` / `updateConf`）
- `bfe_util/epp/epp_client.go`（`BuildTLSConfig` / `NewGrpcConn`）
- `bfe/docs/zh_cn/modifications/2026-09-06-epp-ai-gateway-integration/`、`2026-09-14-epp-plaintext-dial/`

