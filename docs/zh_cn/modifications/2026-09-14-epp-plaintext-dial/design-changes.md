# BFE EPP client 明文拨号支持

## 1. 背景与目标

BFE 连接 EPP（ext-proc）当前**只有 TLS 拨号**：数据连接（`bfe_util/epp/epp_client.go` `NewGrpcConn`）与健康检查探针（`bfe_balance/bal_gslb/epp_runtime.go` `probe`）均以 `credentials.NewTLS(...)` 建立，TLS 校验强度由 cluster_conf 的 `EPPTLS`（`Insecure` / `CAFile`）控制；`EPPTLS` 缺省时保持 legacy 行为（TLS + 跳过证书校验 + 加载告警）。

而 EPP 服务端（ai-gateway-epp）的 `-grpc-tls-cert` / `-grpc-tls-key` **缺省为明文**（"empty = plaintext"，两者同配生效、只配其一 fail-fast）。这导致部署形态被单方面锁死：只要 BFE 侧没有明文拨号能力，EPP 就必须开启 TLS——内网/测试环境想要"双边明文"目前无法实现。

**目标**：

1. 给 BFE 的 EPP client 增加明文拨号选项，与 EPP 服务端明文形态对接；
2. 将 `EPPTLS == nil` 的 legacy 行为形式化为**显式默认值** `{"Insecure": true}`（线上行为不变，语义显性）；
3. 默认行为整体不变（仍为 TLS），明文必须显式配置。

## 2. 现状盘点

| 位置 | 内容 | 与本次的关系 |
|---|---|---|
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:438` | `EPPTLSConf{Insecure, CAFile}`，`EPPTLSConfCheck` 校验；nil → 告警 + legacy 跳过校验 | 新增 `Plaintext` 字段及互斥校验；nil 改为自动填充默认值 |
| `bfe_config/.../cluster_conf_load.go:861` | `GslbBasicConfCheck` EPP 分支调用 `EPPTLSConfCheck` | 填充默认值的落点 |
| `bfe_balance/bal_gslb/bal_gslb.go:162-169` | `buildEPPRuntimeConf`：nil EPPTLS → legacy TLS 跳过校验 | nil 分支删除（EPPTLS 加载后必非 nil），透传 `Plaintext` |
| `bfe_balance/bal_gslb/epp_runtime.go:37-47` | `eppRuntimeConf{tlsInsecure, tlsCAFile, ...}` | 新增 `plaintext` 字段 |
| `bfe_balance/bal_gslb/epp_runtime.go:79,99` | `newEPPRuntime`：`BuildTLSConfig` + `NewGrpcConn`（数据连接） | plaintext 时跳过 TLS 配置、明文拨号 |
| `bfe_balance/bal_gslb/epp_runtime.go:219-222` | `probe`：健康检查短连接，`credentials.NewTLS(rt.tlsConf)` | plaintext 分支：`insecure.NewCredentials()` |
| `bfe_util/epp/epp_client.go:56-94` | `BuildTLSConfig` / `NewGrpcConn`（仅 `epp_runtime.go` 及测试调用） | `NewGrpcConn` 增加 `plaintext` 参数 |

## 3. 设计

### 3.1 配置：`EPPTLSConf` 新增 `Plaintext` 字段 + 显式默认值

```go
type EPPTLSConf struct {
	Insecure  bool
	CAFile    string // CA certificate file for verifying EPP server, required if Insecure is false
	Plaintext bool   // dial EPP without TLS (EPP serves plaintext gRPC; mutually exclusive with Insecure/CAFile)
}
```

**默认值形式化（做法 A）**：`GslbBasicConfCheck` 的 EPP 分支中，`EPPTLS == nil` 时自动填充 `&EPPTLSConf{Insecure: true}`，随后再进入 `EPPTLSConfCheck`：

- 线上行为与现状完全一致（TLS + 跳过校验），但配置语义显性——导出的 cluster_conf 中能看到明确的 `{"Insecure": true}`；
- **加载告警保留**：原 nil 分支的告警移至填充点（"EPPTLS not configured, default to Insecure=true (skip certificate verification), please configure EPPTLS explicitly"），运维提示不丢失；
- **填充与校验的顺序**：填充仅针对 nil；非 nil 的配置不再填充。互斥校验（见下）只对运维显式提供的字段生效，填充值不参与互斥判定。

**互斥校验**（`EPPTLSConfCheck` 内 fail-fast）：`Plaintext` 与 `Insecure` / `CAFile` 同时被显式配置 → 配置加载报错，避免"既明文又配证书"的含糊语义。

### 3.2 拨号路径改造

1. `eppRuntimeConf` 增加 `plaintext bool`；`buildEPPRuntimeConf` 直接从 `gslbBasic.EPPTLS` 读取（nil 分支删除：加载完成后 EPPTLS 必非 nil）。
2. `bfe_util/epp/epp_client.go` `NewGrpcConn(addr, connectTimeout, insecureSkip, caFile string, plaintext bool)`：`plaintext=true` 时以 `insecure.NewCredentials()` 作为 transport credentials（懒连接语义不变）；`BuildTLSConfig` 不变，仅 TLS 路径调用。
3. `epp_runtime.go`：
   - `newEPPRuntime`：`plaintext` 时跳过 `BuildTLSConfig`（`rt.tlsConf` 置 nil，probe 亦不再依赖它）；
   - `probe`：按 `conf.plaintext` 分支选择 `insecure.NewCredentials()` 或 `credentials.NewTLS(rt.tlsConf)`。

### 3.3 部署形态对照

| EPP 服务端形态 | BFE `EPPTLS` 配置 |
|---|---|
| 配 `-grpc-tls-cert`/`-grpc-tls-key`（TLS） | 缺省（自动填充 `Insecure=true`，告警提示）/ `Insecure=true` / `Insecure=false` + `CAFile` |
| 不配（明文，"empty = plaintext"） | `Plaintext=true`（本次新增） |

### 3.4 配置落点与 server_data_conf 下发

`EPPTLSConf` 定义在 `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:438`，是 `GslbBasicConf` 的字段（与 `BalanceMode` / `EPPAddr` / `EPPCheck` / `EPPTimeout` / `EPPBreaker` 同级），随 per-cluster 的 `GslbBasic` 段配置在 **`conf/server_data_conf/cluster_conf.data`**（默认路径见 `bfe_config/bfe_conf/conf_basic.go:106`，由 `bfe_route/cluster_table.go` 的 `ClusterConfLoad` 加载）。JSON 形态：

```json
{
  "Version": "...",
  "Config": {
    "<cluster名>": {
      "GslbBasic": {
        "BalanceMode": "EPP",
        "EPPAddr": ["10.0.0.1:9002", "10.0.0.2:9002"],
        "EPPTLS": { "Insecure": true }
      }
    }
  }
}
```

注意区分：`conf/cluster_conf/cluster_table.data`（后端实例表）与 `conf/cluster_conf/gslb.data`（子集群权重）**不含** `GslbBasic`。

新增 `Plaintext` 字段为布尔零值（false = 现状），BFE 侧 JSON 反序列化兼容旧文件。**但需注意 ai-gateway-api 托管部署的形态**：api 的 server_data_conf 导出当前只下发 `GslbBasic.BalanceMode` 与 `GslbBasic.EPPAddr`（`model/iroute_conf`，不含 `EPPCheck`/`EPPTimeout`/`EPPBreaker`/`EPPTLS`），即托管生成的 `cluster_conf.data` 中 `EPPTLS` 恒为缺省（nil → 自动填充 `Insecure=true`）。因此：

- 手工/conf-agent 自管 `cluster_conf.data` 的部署：本次 BFE 改动即可启用明文链路；
- api 全量生成 `cluster_conf.data` 的部署：`Plaintext` 需 api 导出侧后续补充下发（本次范围外，另行变更）。

## 4. 兼容性

- **线上行为不变**：nil `EPPTLS` 自动等价于 `{"Insecure": true}`（TLS + 跳过校验），仅多一条加载告警；显式配置语义全部不变。
- **旧配置文件**：无 `Plaintext` 字段 → false → 原 TLS 路径，行为不变。
- **滚动升级顺序（重要）**：旧 BFE 二进制遇到含 `Plaintext` 的 `EPPTLS` 时，未知 JSON 字段被忽略 → 退化为 `{Insecure: false, CAFile: ""}` → 被旧版 `EPPTLSConfCheck` 拒绝加载。因此灰度顺序必须为：**先全量升级 BFE 二进制（新二进制完全兼容旧配置），再下发含 `Plaintext` 的 cluster_conf**。
- 不改动 `conf/` 样例（现有样例无 EPP 配置段）；不改动 EPP / ai-gateway-api 侧任何代码。

## 5. 测试

| 测试文件 | 新增/调整用例 |
|---|---|
| `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load_epp_test.go` | nil `EPPTLS` → 自动填充 `{Insecure: true}`（原 "nil keeps legacy behavior" 用例改为断言填充值）；`Plaintext=true` 合法加载；`Plaintext` 与 `Insecure`/`CAFile` 显式同配报错；JSON round-trip 携带 `Plaintext` |
| `bfe_util/epp/epp_client_test.go` | `NewGrpcConn` plaintext 参数下对明文 gRPC server 建连成功（仿现有 fake server，去掉服务端 TLS creds） |
| `bfe_balance/bal_gslb/epp_conn_test.go` / `epp_runtime_test.go` | plaintext 数据连接复用；plaintext 健康检查探针通过、failover 基本流程 |

验证：`go build ./...` + `make test`（`go test -cover ./...` 与 `go vet ./...`）。
