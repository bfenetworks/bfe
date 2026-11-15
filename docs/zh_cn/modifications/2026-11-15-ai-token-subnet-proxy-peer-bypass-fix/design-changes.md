# BFE AI API Key 子网限制可被代理对端 IP 绕过修复方案（mod_ai_token_auth subnet 校验 OR 回落缺陷）

## 1. 背景

安全研究者反馈（本地私密研究稿，未公开提交；finder：Liam Giannini / webcracy.ch，credit as finder）：

> `mod_ai_token_auth` 的 API Key `subnet` 限制在「可信反向代理部署 + 代理对端 IP 位于允许子网内 + 真实客户端 IP 在子网外」场景下可被绕过。BFE 已知真实客户端地址（`Request.ClientAddr`），但子网校验对 `ClientAddr`、`RemoteAddr` 做 **OR** 判定，代理对端命中即放行。

- 影响版本：BFE **1.9.0**（tag commit `cb99da2`）及当前 master（本地 HEAD `c3ddeb18` 代码路径一致，已逐行核实）
- CWE：CWE-863（Incorrect Authorization）
- 建议严重度：Moderate（有条件触发，见 §1.3）
- 该缺陷**不绕过 API Key 校验本身、不泄露 Key**；绕过的是已有效 Key 的附加网络限制，影响被该 Key 的既有权限、模型范围、配额限死

### 1.1 涉及代码路径（源码实锤）

**① 子网校验的双重地址判定** — `bfe_modules/mod_ai_token_auth/token_rule_table.go:196-215`：

```go
if len(token.Subnet) > 0 {
    inSubnet := false
    for _, subnet := range token.Subnet {
        if req.ClientAddr != nil && subnet.Contains(req.ClientAddr.IP) {
            inSubnet = true
            break
        } else if req.RemoteAddr != nil && subnet.Contains(req.RemoteAddr.IP) {
            inSubnet = true      // ← 客户端 IP 不在子网时，回落到 TCP 对端（代理）IP
            break
        }
    }
    ...
}
```

**② 两地址的语义分离** — `bfe_basic/request.go:68-70`：

```go
RemoteAddr *net.TCPAddr // address of remote peer
ClientAddr *net.TCPAddr // address of real client. Maybe nil if request is from
// upstream proxy but without a valid Clientip header
```

**③ 可信代理标记** — `bfe_modules/mod_trust_clientip/mod_trust_clientip.go:146-153`：按 TCP 对端 IP 查信任表，`session.SetTrustSource(trusted)`。

**④ 真实客户端地址推导** — `bfe_server/set_client_addr.go:28-44`：

- 非可信来源：`req.ClientAddr = req.RemoteAddr`（**直连时两地址恒等**）；
- 可信来源：`ClientAddr` 取自 `X-Real-IP`，缺省取第一个 `X-Forwarded-For` 值；`RemoteAddr` 保持代理 TCP 地址；头无效时 `ClientAddr` 为 **nil**。

### 1.2 为什么这是缺陷而非有意设计

`subnet` 的文档语义是「允许的**源 IP** 子网」（`docs/zh_cn/configuration/mod_ai_token_auth/token_rule.data.md:93`：「允许的源 IP 子网」，模块文档：`docs/en_us/modules/mod_ai_token_auth/mod_ai_token_auth.md:24`：「whether the request **source IP** is within the allowed subnet range」），指真实客户端来源。

关键在于：**直连场景下 `set_client_addr.go:31` 已令 `ClientAddr == RemoteAddr`**，回落分支对直连毫无作用；它**唯一**生效的场景就是可信代理场景——不会是「为直连兜底」的设计，是未考虑代理部署的疏漏。

### 1.3 触发条件与影响边界

前置条件（三者同时满足才可绕过）：

1. 攻击者持有一把**有效** API Key，且该 Key 配置了 `subnet`；
2. 部署为可信反向代理形态（`mod_trust_clientip` 将代理对端标记为 trusted，BFE 前面挂 Nginx/LB 的典型内网形态）；
3. 代理对端 IP 位于 Key 允许子网内，真实客户端 IP 在子网外。

影响：Key 外泄后，本应作为第二道访问条件的子网限制失效——外网持有者经由子网内代理即可调用受保护 AI 路由。不提升 Key 自身权限。

### 1.4 复现矩阵（研究者本地 Go 测试，1.9.0 源码包内运行原方法）

Key 子网配置为 `10.0.0.0/8`：

| 真实客户端（`ClientAddr`） | 代理对端（`RemoteAddr`） | 修复前结果 | 期望结果 |
|------|------|------|------|
| `203.0.113.10`（子网外） | `198.51.100.5`（子网外） | 拒绝 ✅ | 拒绝 |
| `203.0.113.10`（子网外） | `10.0.0.5`（子网内） | **接受 ❌** | 拒绝 |
| `10.1.2.3`（子网内） | `198.51.100.5`（子网外） | 接受 ✅ | 接受 |

## 2. 目标

1. **修复判定逻辑**：存在可用的真实客户端地址（`ClientAddr != nil`）时，`subnet` 只按 `ClientAddr` 判定；`ClientAddr == nil`（直连，或可信代理未携带有效客户端头）时才回落到 `RemoteAddr`（TCP 对端）。
2. **行为语义对齐文档**：「允许的源 IP 子网」= 真实客户端来源；代理对端地址不参与判定。
3. **回归防护**：新增「子网外客户端 / 子网内代理对端」拒绝用例，及 `ClientAddr == nil` 回落用例。

## 3. 变更总览

| 项目 | 位置 | 现状 | 变更后 |
|------|------|------|--------|
| 子网判定分支 | `bfe_modules/mod_ai_token_auth/token_rule_table.go:196-206` | `ClientAddr` 或 `RemoteAddr` 任一命中即放行（OR） | `ClientAddr != nil` 时仅按 `ClientAddr` 判定；`ClientAddr == nil` 时按 `RemoteAddr` 判定 |
| 错误信息 | `token_rule_table.go:209` | "Client IP not in subnet of key %s" | 不变（语义恰好吻合新行为） |
| 单元测试 | `bfe_modules/mod_ai_token_auth/token_rule_table_test.go` | 无代理场景子网用例 | 新增子网判定三矩阵用例 + nil-ClientAddr 回落用例 |

> 明确**不修改**：`bfe_server/set_client_addr.go`、`mod_trust_clientip`、`bfe_basic/request.go`——地址推导与信任机制本身无缺陷，缺陷仅在判定端的 OR 回落。

## 4. 详细设计

### 4.1 判定逻辑改写

`bfe_modules/mod_ai_token_auth/token_rule_table.go`：

```go
if len(token.Subnet) > 0 {
    inSubnet := false
    // 优先按真实客户端地址判定；无可用客户端地址（直连或可信代理
    // 未携带有效客户端头）时回落到 TCP 对端地址。
    srcAddr := req.ClientAddr
    if srcAddr == nil {
        srcAddr = req.RemoteAddr
    }
    for _, subnet := range token.Subnet {
        if srcAddr != nil && subnet.Contains(srcAddr.IP) {
            inSubnet = true
            break
        }
    }
    if !inSubnet {
        ... // SUBNET_NOT_ALLOWED 拒绝路径不变
    }
}
```

### 4.2 设计要点

1. **两种部署形态天然覆盖**：直连时 `ClientAddr == RemoteAddr`（`set_client_addr.go:31`），选谁结果相同；可信代理时只信 `ClientAddr`——同一套代码同时正确服务两种形态，无需按 `TrustSource()` 特判。
2. **`ClientAddr == nil` 的回落**：可信代理未携带有效 `X-Real-IP` / `X-Forwarded-For` 时回落到代理对端地址。这与修复前行为一致（修复前 nil 时同样走 `RemoteAddr` 分支），本变更不引入新的失败模式。是否对「配置了 subnet 的 Key + 可信代理未带头」改为 fail closed（直接拒绝）属于可选加固，见 §8。
3. **错误码与日志不变**：仍返回 `SUBNET_NOT_ALLOWED`（403，`authentication_error`），`SetAiAuthInfo` 写入 712 的语义不受影响；错误文案 "Client IP not in subnet of key" 在新行为下语义更精确（此前客户端在子网外却报 "Client IP"，本就名不副实）。

## 5. 涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` | `ValidateUserTokenByReq` 子网判定分支改写（§4.1），拒绝路径不动 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table_test.go` | 新增子网判定测试：`TestValidateUserTokenByReqSubnetUsesClientAddrOnly`（外客户端/内代理拒绝、内客户端接受、nil-ClientAddr 回落 RemoteAddr 三个子用例） |
| `bfe/tests/integration/implementation/scenario-SC39-auth-and-token-quota/sc39_auth_and_token_quota_test.go` | 新增 `TestTC14_SubnetGateBehindTrustedProxy` 三子用例（§6.3）；`testOptions` 增加 `enableTrustClientIP`；`sendRequest` 支持可选 `X-Real-Ip` |
| `bfe/tests/integration/implementation/scenario-SC39-auth-and-token-quota/testdata/mod_trust_clientip/`（新增） | 可信代理样例配置：`mod_trust_clientip.conf` + `trust_client_ip.data`（信任 127.0.0.0/8，测试客户端经 loopback 扮演子网内代理） |
| `bfe/tests/integration/测试设计文档/scenario-SC39-鉴权错误矩阵与Token配额/TC-14-子网限制可信代理绕行.md`（新增） | TC-14 测试设计文档 |
| `bfe/tests/integration/测试设计文档/scenario-SC39-鉴权错误矩阵与Token配额/场景说明.md` | §1/§3/§6 登记 TC-14 与 mod_trust_clientip 配置 |

## 6. 测试计划

### 6.1 单元测试

`token_rule_table_test.go` 新增用例，参照既有 `TestSubnetValidation` / `TestValidateUserTokenByReqRejectionKeepsIdentity` 的构造方式（token 设 `Subnet`，构造 `req.ClientAddr` / `req.RemoteAddr`）：

1. **外客户端 + 内代理对端**：`ClientAddr=203.0.113.10`、`RemoteAddr=10.0.0.5`、子网 `10.0.0.0/8` ⇒ 拒绝，`RejectReason == SUBNET_NOT_ALLOWED`（修复前会接受——本用例即回归防线）；
2. **内客户端 + 外代理对端**：`ClientAddr=10.1.2.3`、`RemoteAddr=198.51.100.5` ⇒ 接受；
3. **nil-ClientAddr 回落**：`ClientAddr=nil`、`RemoteAddr=10.0.0.5` ⇒ 接受（保留对直连/无头代理的兼容行为）；`RemoteAddr=198.51.100.5` ⇒ 拒绝；
4. **无子网配置**：`token.Subnet` 为空 ⇒ 不进入判定，行为不变（负面对照）。

### 6.2 编译与回归

```bash
cd bfe
go build ./...
go test ./bfe_modules/mod_ai_token_auth/... -count=1 -v
go test ./bfe_modules/mod_trust_clientip/... ./bfe_server/... -count=1   # 确认未波及地址推导
```

### 6.3 集成验证（SC39 TC-14）

`tests/integration/implementation/scenario-SC39-auth-and-token-quota/` 新增 `TestTC14_SubnetGateBehindTrustedProxy`：经 `enableTrustClientIP` 启用 `mod_trust_clientip`（信任 loopback，测试客户端扮演子网内可信代理），Key `subnet=127.0.0.0/8`（覆盖代理对端、不覆盖伪装公网地址），三个子用例：

1. `X-Real-Ip: 203.0.113.10`（子网外客户端）⇒ 403 `SUBNET_NOT_ALLOWED`，后端命中 0（修复前返回 200——已用 stash 回退验证该用例在旧代码上必 FAIL）；
2. `X-Real-Ip: 127.0.0.2`（子网内客户端）⇒ 200；
3. 不带 `X-Real-Ip` ⇒ 200（`ClientAddr` 为 nil，回落 TCP 对端 loopback，验证兼容行为）。

```bash
cd bfe
go test ./tests/integration/implementation/scenario-SC39-auth-and-token-quota/ -count=1
```

测试设计文档：`tests/integration/测试设计文档/scenario-SC39-鉴权错误矩阵与Token配额/TC-14-子网限制可信代理绕行.md`。

## 7. 兼容性说明

1. **行为收窄**：修复后「子网外客户端经子网内代理访问」从接受变为拒绝——这正是缺陷本身，属安全修复的预期行为变化。正常客户端（直连或在子网内经代理）不受影响。
2. **误伤风险**：若有存量部署**依赖**「代理对端在子网内即可放行」的隐性行为（即有意把子网配在代理上而非客户端上），升级后其外网客户端将被拒绝。缓解：此类部署应把子网改为覆盖真实客户端来源，或保持不配置 `subnet`。
3. 错误码、日志字段、配置格式、token 导出格式均无变化。

## 8. 可选加固（不在本变更范围）

研究者建议中「可信代理未携带可用客户端地址时对 subnet-Key fail closed」未纳入本次最小修复——`ClientAddr == nil` 回落 `RemoteAddr` 属既有行为，贸然收紧可能切断正当的直连/无头代理流量。如后续需要，可在 `token_rule.data` 增加显式开关（如 `subnet_strict: true`），由运营按部署形态选择。

---

*文档生成日期：2026-11-15*
