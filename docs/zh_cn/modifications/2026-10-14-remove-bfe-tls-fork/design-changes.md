# 移除 bfe_tls fork，改用标准库 crypto/tls

## 1. 背景

`bfe/bfe_tls` 是对 Go 标准库 `crypto/tls`（约 Go 1.9 时代、仅 TLS 1.2）的完整 fork，历史上为支持以下定制而引入：

- `MultiCert`：按 VIP/SNI 动态选择证书
- `ServerRule`：按 VIP/SNI 匹配 TLS 规则（NextProtos 灰度、全局 h2/spdy 开关、ClientAuth、Grade 密码套件）
- Redis 服务端会话缓存（`ServerSessionCache`）
- JA3 指纹、握手内部指标、SSLv2 ClientHello 兼容、DynamicRecord、握手耗时统计
- 直接从 `ConnectionState` 暴露 `ClientRandom/MasterSecret` 供 mod_key_log 使用

fork 的代价是：无法获得标准库多年累积的安全修复与 TLS 1.3 支持，升级 Go 版本时存在代码漂移风险，且包类型已渗透到 `bfe_basic.Session` / `bfe_http2` / `bfe_spdy` / `bfe_stream` 等基础设施层，维护面约 70 个文件。

经评估，核心生产特性（MultiCert、ServerRule）均可由标准库 `GetCertificate` / `GetConfigForClient` 回调完整表达，fork 不再必要。

## 2. 变更目标

1. **删除 `bfe_tls/` 整个包**，全部改为直接依赖标准库 `crypto/tls`。
2. **保持配置规则不变**：`server_cert_conf.data`、`tls_rule_conf.data` 文件格式与匹配语义（VIP 优先、SNI 次之、默认规则兜底）不变。
3. 支持 TLS 1.3。
4. 以下特性随 fork 一并移除（已确认非必要）：JA3、SSLv2 ClientHello、Redis 会话缓存、DynamicRecord、握手内部指标、Grade 密码套件等级。
5. 配置文件中与 fork 强耦合的字段**保留但废弃**（解析忽略），保证旧配置可直接启动。

## 3. 变更总览

| 模块 | 主要改动 |
|------|----------|
| `bfe_tls` | 整体删除 |
| `bfe_server` | `https_listener`/`tls_multi_cert`/`tls_server_rule`/`http_conn`/`bfe_confdata_load`/`bfe_server.go` 重写为 `crypto/tls` + 回调 |
| `bfe_basic` | `Session.TlsState` 类型改为 `*tls.ConnectionState` |
| `bfe_http`/`bfe_http2`/`bfe_websocket`/`bfe_spdy`/`bfe_stream`/`bfe_balance` | 类型签名从 `bfe_tls.Conn/Config` 换为 `tls.Conn/Config` |
| `bfe_util` | 新增 `CipherSuiteTextForOpenSSL` / `VersionTextForOpenSSL` 映射（从 fork 迁入） |
| `mod_header` | 删除 `tls_ja3_raw`/`tls_ja3_hash` 内置变量 |
| `mod_access` | TLS 日志字段适配标准库 `ConnectionState` |
| `mod_key_log` | 改为向 bfe_server 注册 `tls.Config.KeyLogWriter` |
| `bfe_config` | `bfe.conf`/`tls_rule_conf` 废弃字段解析忽略 |

## 4. 详细设计

### 4.1 fork 机制 → 标准库映射

| fork 机制 | 标准库替代 | 说明 |
|-----------|-----------|------|
| `Config.MultiCert`（握手中选证书） | `Config.GetCertificate(hello *tls.ClientHelloInfo)` | VIP 用 `bfe_util.GetVip(hello.Conn)` 取——解开 `*tls.Conn` 后查底层连接的 `AddressFetcher.VirtualAddr()`（L4 LB 经 PROXY 协议注入），并非 `LocalAddr()`；SNI 从 `hello.ServerName` 取，与 fork 中 `MultiCertMap.Get(conn)` 的查找顺序一致 |
| `Config.ServerRule`（握手中匹配规则） | `Config.GetConfigForClient(hello)` 返回规则对应的 `*tls.Config` | 规则在 `Update()` 时预构建为不可变的 `*tls.Config` 模板，`GetConfigForClient` 只做查表，不在握手热路径上分配 |
| `NextProtoConf.Get(conn)`（灰度 + 全局开关） | `GetConfigForClient` 内计算 `NextProtos` | murmur3 hash 远端 IP 的灰度逻辑、全局 h2/spdy 开关（`EnableNextProto`）、`PROTO_OPTIONAL/MANDATORY` 语义原样保留 |
| `ClientCRLPool` 吊销检查 | `Config.VerifyConnection` | 标准库完成链校验后，在 `VerifyConnection(cs tls.ConnectionState)` 中用 `cs.VerifiedChains` 做 CRL 吊销检查（CRLPool 逻辑迁入 bfe_server） |
| Redis `ServerSessionCache` | 删除 | 用标准库内置 session ticket，`SessionTicketKey` 配置保留；`SessionTicketKeyName` 废弃 |
| `JA3/SSLv2/握手指标/HandshakeTime/DynamicRecord` | 删除 | 握手延迟统计在 `http_conn.go` 外层用 `time.Since` 包住 `Handshake()` 调用，继续写入现有 `ProxyHandshakeDelay` 指标 |
| Grade → cipher suites | 废弃 | TLS1.2 套件用标准库默认安全子集；TLS1.3 套件不可配（标准库行为） |
| `CipherSuitesPriority` + `PreferServerCipherSuites` 协商 | `Config.CipherSuites` 白名单 | 配置行序与 `|` 分组不再影响优先级，详见 4.7 |
| mod_key_log 读 `ClientRandom/MasterSecret` | `Config.KeyLogWriter` | 模块 init 时向 bfe_server 注册 writer，标准库握手时按 NSS keylog 格式写出 |

### 4.2 规则匹配的双通道

TLS 规则（`TLSServerRuleMap`）需要在两个时机被查询：

1. **握手时**（`GetConfigForClient`/`GetCertificate`）：输入 `ClientHelloInfo`，VIP 用 `bfe_util.GetVip(hello.Conn)` 取（解开 `*tls.Conn` → `NetConn()` → `AddressFetcher.VirtualAddr()`，即 PROXY 协议注入的虚地址），SNI 取 `hello.ServerName`。
2. **握手后**（h2/spdy/stream 参数）：输入 `*tls.Conn`，VIP 同样用 `bfe_util.GetVip(tlsConn)` 取，SNI 取 `tlsConn.ConnectionState().ServerName`。

匹配顺序保持不变：VIP 精确命中 → SNI 命中 → 默认规则。`getDefaultRule` 语义不变（NextProtos 用默认值、ClientAuth=false、h2/stream 参数为零值）。

### 4.3 bfe_server 改造要点

- `https_listener.go`：`bfe_tls.NewListener` → `tls.NewListener`，base config 上挂 `GetConfigForClient`/`GetCertificate`。
- `tls_multi_cert.go`：`MultiCertMap.Get(c *bfe_tls.Conn)` → `GetCertificate(hello *tls.ClientHelloInfo)`；VIP 无 SNI、SNI 无 VIP、默认证书三级查找与 metrics 计数保留。
- `tls_server_rule.go`：`Update()` 时为每条规则预构建 `*tls.Config` 模板（MinVersion/MaxVersion、ClientAuth/ClientCAs、VerifyConnection、NextProtos 占位）；`GetConfigForClient` 按 hello 查表并补算灰度 NextProtos。`EnableNextProto`（全局 h2/spdy 开关）语义保留。
- `http_conn.go`：`*bfe_tls.Conn` 断言改为 `*tls.Conn`；`getMandatoryProtocol` 改用 `NegotiatedProtocolIsMutual`/`NegotiatedProtocol`；握手耗时用外层计时。
- `bfe_server.go`：删除 `SessionCache`/`MultiCert`/`TLSServerRule` 中 fork 相关接线；TLSConfig 改为标准库配置 + 回调。

### 4.4 下游适配

- `bfe_basic/session.go`：`TlsState *tls.ConnectionState`；`GetVip()` 逻辑不变（从连接取）。
- `bfe_util`：迁入 `CipherSuiteTextForOpenSSL` / `VersionTextForOpenSSL`（mod_header/mod_access 依赖）。
- `bfe_http/transport.go`、`bfe_balance/backend/health_check.go`：客户端 `bfe_tls.Client` → `tls.Client`，`VerifyHostname` 语义由标准库提供（出站侧顺带获得 TLS1.3）。
- `bfe_http2/bfe_websocket/bfe_spdy/bfe_stream`：连接类型换 `*tls.Conn`；`bfe_http2.SetServerRule`/`bfe_stream.SetServerRule` 签名改为基于 `*tls.Conn` 的规则查询。
- `bfe_util/get_l4lb_info.go`、`get_net_info.go`：类型断言改为 `*tls.Conn`。

### 4.5 模块改动

- **mod_header**：删除 `tls_ja3_raw`/`tls_ja3_hash` 两个内置变量；`tls_version`/`tls_cipher`/会话复用等变量保留，映射函数改从 `bfe_util` 取。
- **mod_access**：TLS 相关日志字段（版本、cipher、复用）适配标准库 `ConnectionState`。
- **mod_key_log**：不再从 session 读 MasterSecret；init 时调用 bfe_server 暴露的注册接口设置 `KeyLogWriter`，输出格式保持 NSS keylog 兼容。

### 4.6 配置废弃字段（解析保留、行为忽略）

| 配置文件 | 字段 | 处理 |
|----------|------|------|
| `bfe.conf` `[HttpsBasic]` | `EnableSslv2ClientHello` | 废弃忽略 |
| `bfe.conf` `[SessionCache]` | `Servers` 等 | 废弃忽略（Redis 会话缓存移除） |
| `bfe.conf` | `SessionTicketKeyName` 相关 | 废弃，仅使用 `SessionTicketKey` |
| `tls_rule_conf.data` | `Grade` | 废弃，TLS1.2 套件用标准库默认 |
| `tls_rule_conf.data` | `Chacha20`/`DynamicRecord`（含默认值 `DefaultChacha20`/`DefaultDynamicRecord`） | 废弃忽略 |

### 4.7 CipherSuites 优先级语义变化

fork 时代 `bfe.conf` 中 `HttpsBasic.CipherSuites` 的**配置行序**参与握手协商；改用标准库后该语义丢失，仅保留"白名单（可用套件集合）"语义。

#### 4.7.1 旧实现（fork）

`bfe_conf.GetCipherSuites` 返回两个平行数组 `(cipherSuites, cipherSuitesPriority)`：

- `cipherSuites`：把 `|` 展开后的扁平套件 ID 列表；
- `cipherSuitesPriority`：与之一一对应，值为该套件所属的**配置行序号**（同一行内 `|` 分隔的套件共享同一行号）。

两者分别装配到 `TLSConfig.CipherSuites` 与 `TLSConfig.CipherSuitesPriority`。握手时因 `PreferServerCipherSuites=true` 且两数组长度必然相等，总是走 `negotiateEquivalentCipherSuites`：

- 先比较 `serverOrder`（行号，小者优先）→ **配置行序即服务端优先级**；
- 行号相同时再比较 `clientOrder` → **同一行（`|` 分组）内由客户端偏好决定**。

因此默认配置把 `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256` 写在一行，表示为"等效档"：有 AES-GCM 硬件加速的客户端选 GCM，移动端选 ChaCha20。

#### 4.7.2 新实现（标准库）

`bfe_conf.GetCipherSuites` 只返回扁平列表，`bfe_server` 仅赋值 `TLSConfig.CipherSuites`。标准库 `Config.cipherSuites()` 以固定的 `cipherSuitesPreferenceOrder` 为基准、用 `slices.DeleteFunc` 按 `Config.CipherSuites` 过滤，其注释明确：

> Cipher suites are filtered but **not reordered** based on the application and peer's preferences.

即 `TLSConfig.CipherSuites` 退化为**纯白名单（集合语义）**，实际协商顺序由标准库固定表决定（AEAD+ECDHE 优先、AES-128 优先于 AES-256、ECDSA 优先于 RSA、3DES/RC4 垫底）。`bfe_server.go` 中仍赋值的 `PreferServerCipherSuites = true` 自 Go 1.17 起 deprecated 且被忽略；服务端仍是"自行定序"的一方，但用的是标准库顺序而非配置顺序。

#### 4.7.3 影响

| 维度 | 影响 |
|------|------|
| 可用套件**集合** | 不变。白名单仍生效；显式列出的 RC4/RSA-kex/3DES 仍会被重新启用（`disabledCipherSuites`/`rsaKexCiphers`/`tdesCiphers` 仅在 `Config.CipherSuites == nil` 时才自动禁用） |
| 套件**顺序/优先级** | 丢失。改为标准库固定顺序，配置行序失效 |
| `|` 等效分组 | 失效。仍能解析与校验，但不再影响协商结果 |
| `PreferServerCipherSuites` | 空操作（Go 1.17+ 忽略） |
| TLS1.3 套件 | 本就不可配置（标准库行为），无影响 |

#### 4.7.4 配置兼容性

`bfe.conf` 的 `HttpsBasic.CipherSuites` 格式与解析逻辑不变，旧配置可直接启动、无需修改。但需注意：

- 若历史配置以**行序强制某套件优先**，该意图已不再生效；
- 若配置目的是**裁剪可用套件**（禁用弱套件），仍然有效；
- 文档中"等效套件可用 `|` 分隔"的表述已同步更新：`docs/zh_cn/configuration/bfe.conf.md`、`docs/en_us/configuration/bfe.conf.md`、`conf/bfe.conf`。

## 5. 参考方案分析：lsegroup fork 的适配层改造

社区存在另一种迁移路线（lsegroup/bfe fork）：**不删除 `bfe_tls`，而是把它从"深度 fork"降级为"薄适配层"**——删除整个握手实现，用标准库 `crypto/tls` 做底层，`bfe_tls` 只保留与原 API 同形的类型和回调接线，约 70 个调用方文件基本不用改。

### 5.1 该方案的改造方式

1. **删除 fork 的握手核心**（约一半代码量）：`handshake_client/server/messages.go`、`key_agreement.go`、`prf.go`、`ticket.go`、`alert.go` 及全部握手测试。
2. **Conn 变为包装 + "合成 Conn"**：`bfe_tls.Conn` 内嵌 `*crypto_tls.Conn`；握手回调中标准库只提供 `ClientHelloInfo.Conn`（原始 net.Conn），于是构造一个内嵌指针为 nil 的"合成 Conn"，VIP 取 `LocalAddr()`、SNI 取 `hello.ServerName`，喂给原有 `ServerRule.Get(c *bfe_tls.Conn)` / `MultiCert.Get` 接口——MultiCert 和 ServerRule 的配置规则完全不用动。
3. **fork 机制 → 标准库回调映射**：MultiCert → `GetCertificate`；ServerRule → `GetConfigForClient`（命中规则后 `Clone()` 基础配置再应用规则）；Grade → `MinVersion`/`CipherSuites`；listener 内置 RWMutex + `UpdateListener` 支持热加载；`MaxVersion = 0` 放开 TLS 1.3。
4. **类型/常量全部再导出**：`ConnectionState` 保留扩展字段（JA3、HandshakeTime、ClientRandom/MasterSecret 等），`Config` 保留全部旧字段，调用方零改动。

### 5.2 该方案的缺陷（本方案明确规避）

该 fork 是"机械保留字段"式迁移，产生大量**字段还在、功能已死**的静默损坏：

| 项 | 该 fork 的问题 | 本方案的处理 |
|----|---------------|--------------|
| JA3 | `ja3Raw/ja3Hash` 无任何赋值点，mod_header 变量恒返回空串 | 删除 JA3 变量（4.5 节） |
| mod_key_log | `ClientRandom/MasterSecret` 不可填充，仍输出 `CLIENT_RANDOM <空> <空>` 垃圾日志 | 改用 `Config.KeyLogWriter`（4.1 节） |
| 握手耗时 | `HandshakeTime` 恒为 0，`ProxyHandshakeDelay` 指标失真 | `http_conn.go` 外层计时（4.3 节） |
| CRL 吊销 | Rule 保留 `ClientCRLPool` 但未用 `VerifyConnection` 接线，检查丢失 | `VerifyConnection` 接线（4.1 节） |
| Redis SessionCache | bfe_server 仍构造 cache，但 `toCryptoTLS` 忽略该字段，纯死代码 | 整体删除，字段废弃忽略（4.6 节） |
| ALPN 灰度 | 规则的 `NextProtos`（含 rate 灰度）未接进握手 ALPN，灰度能力丢失 | `GetConfigForClient` 内补算灰度 NextProtos，语义保留（4.1 节） |
| Grade 语义 | `gradeMinVersion` 对所有等级返回 TLS1.2，等级名存实亡 | Grade 整体废弃，TLS1.2 套件用标准库默认子集（4.6 节） |
| TlsState 指标 | 计数器无任何自增点，监控全零 | 随 fork 删除，保留项在外层统计 |

### 5.3 对本方案的启示

- 合成 Conn + `GetConfigForClient`/`GetCertificate` 的双通道（握手前/握手后）设计**验证可行**，与本方案 4.1/4.2 节的技术路线一致。
- 可借鉴实现：`UpdateListener` 的 RWMutex 热加载封装、`CipherSuiteTextForOpenSSL` 映射表实现。
- 不建议长期保留 bfe_tls 薄壳作为终态：双重配置翻译是永久维护成本。本方案选择完成调用点迁移后整体删除。

## 6. 行为变化

| 项 | 旧行为（fork） | 新行为（标准库） |
|----|---------------|------------------|
| TLS 版本 | 最高 TLS 1.2 | 支持 TLS 1.3；TLS 1.0/1.1 被标准库移除，最低 TLS 1.2 |
| TLS1.2 cipher | 按 Grade（A+/A/B/C）裁剪 | 标准库默认安全子集，不可按 Grade 裁剪 |
| TLS1.2 cipher 优先级 | 按配置行序，`|` 分组为等效档 | 标准库固定顺序；配置行序与 `|` 分组不再影响优先级，`CipherSuites` 退化为白名单（见 4.7） |
| 会话恢复 | 支持 Redis 跨实例共享 | 仅本进程 session ticket |
| 灰度发布 | NextProtos 按远端 IP hash 灰度 | 保留，逻辑不变 |
| 证书选择 | VIP/SNI 三级查找 | 保留，逻辑不变 |
| ClientAuth + CRL | fork 内校验 | 标准库校验 + VerifyConnection CRL 检查 |
| 密钥日志 | 请求结束时写 | 握手时写（NSS 格式） |
| mod_access session 日志 | 含 `ses_tls_client_random`/`ses_tls_server_random` | 两项删除（标准库 `ConnectionState` 不暴露握手随机数） |
| mod_access_pb3 会话日志 | `OcspStaple` 取 fork 布尔字段；含 `HandshakeTime` | `OcspStaple` 改为 `len(OCSPResponse) > 0`；`HandshakeTime` 删除 |

## 7. 边界情况

| 场景 | 处理 |
|------|------|
| 无 SNI 客户端 | VIP 匹配规则/证书，与现状一致 |
| VIP 未配置任何规则 | 走默认规则，NextProtos 为 `http/1.1` |
| 灰度 rate 过滤后 NextProtos 为空 | 回退 `DefaultNextProtos`（`http/1.1`），语义不变 |
| 全局 h2/spdy 关闭（`EnableNextProto`） | 握手中过滤 optional 协议，语义不变 |
| `tls_rule_conf.data` 仍含废弃字段 | 正常解析，不产生效果 |
| TLS1.3 客户端 | 正常协商；ClientAuth/CRL 由标准库 `VerifyConnection` 处理 |
| OCSP Stapling | `tls.Certificate.OCSPStaple` 标准库原生支持，保留 |

## 8. 测试计划

### 8.1 单元测试

- `bfe_server`：MultiCert 三级查找、TLSServerRuleMap VIP/SNI/默认匹配、NextProtosConf 灰度与全局开关、废弃字段解析。
- `bfe_util`：cipher/version 文本映射。
- `mod_header`/`mod_access`/`mod_key_log`：适配后测试。

### 8.2 集成测试

- 现有涉及 HTTPS/H2 的集成场景全量回归（SC01~SC19；SC13 的 TLS 1.3 客户端认证断言已适配）。
- 新增 **SC20 标准库 crypto/tls 适配**（8 个 TC）：TLS 1.3/1.2 版本边界、SNI 选证书、ALPN/h2、启动态 ClientAuth、CRL 吊销、session ticket 恢复与密钥轮换、废弃字段兼容、mod_key_log NSS 日志。设计文档见 `tests/integration/测试设计文档/scenario-SC20-标准库TLS适配/`。

## 9. 影响范围

| 文件/目录 | 影响 |
|-----------|------|
| `bfe/bfe_tls/` | 删除 |
| `bfe/bfe_server/` | TLS 监听、证书、规则、连接处理重写 |
| `bfe/bfe_basic/session.go` | TlsState 类型 |
| `bfe/bfe_http/`、`bfe/bfe_http2/`、`bfe/bfe_websocket/`、`bfe/bfe_spdy/`、`bfe/bfe_stream/`、`bfe/bfe_balance/` | 类型签名 |
| `bfe/bfe_util/` | 新增两个映射函数 |
| `bfe/bfe_modules/mod_header/`、`mod_access/`、`mod_key_log/` | 功能适配 |
| `bfe/bfe_config/bfe_conf/`、`bfe_config/bfe_tls_conf/` | 废弃字段标记 |

## 10. 兼容性与风险

### 10.1 兼容性

- `server_cert_conf.data`、`tls_rule_conf.data`、`bfe.conf` 文件格式不变，旧配置可直接启动。
- VIP/SNI 规则匹配、证书选择、灰度发布语义不变。
- `/reload/tls_conf` 接口与热加载语义不变。

### 10.2 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| TLS1.3 协商导致下游对 cipher/版本的日志/校验逻辑变化 | 文档说明；mod_access/mod_header 变量继续提供版本与 cipher 文本 |
| Grade 废弃后 TLS1.2 套件集合变化 | 标准库默认子集为安全基线；如后续需要裁剪，通过 `CipherSuites` 配置项扩展（非本次范围） |
| Redis 会话缓存移除导致多实例握手率上升 | 由 LB 会话亲和或后续按需重新引入（标准库侧已无 hook，届时需另议方案） |
| fork 中未识别的隐性行为差异 | 以集成测试（SNI/VIP/ClientAuth/热加载）兜底 |

## 11. 参考资料

- `bfe/bfe_tls/`（本次删除对象）
- `bfe/bfe_server/tls_server_rule.go`、`tls_multi_cert.go`、`https_listener.go`、`http_conn.go`
- `bfe/bfe_config/bfe_tls_conf/tls_rule_conf/tls_rule_conf_load.go`
- Go 标准库 `crypto/tls`：`Config.GetConfigForClient`、`Config.GetCertificate`、`Config.VerifyConnection`、`Config.KeyLogWriter`
- 参考方案：lsegroup/bfe fork 的 bfe_tls 适配层改造（见第 5 节分析）
