# 下发配置文件敏感字段落盘加密（BFE 加载点解密）

> 完整技术方案见《下发链路加密技术方案（场景二：BFE 磁盘落盘）》（下称《方案》）。
> 本文是**该方案在 BFE 仓的改造点说明**：控制面导出加密、密钥体系论证、威胁模型、
> 轮换 runbook 等全局内容以《方案》为准，此处仅摘要，不重开论证。

## 1. 背景

需求来源：**密钥加密落盘与密钥管理平台集成**（核心必备功能）在数据面侧的
落地——"全链路无任何明文落盘"。控制面数据库落盘加密另有独立方案解决（与本方案
密钥相互独立）；本方案解决 BFE 数据面磁盘落盘。

现状链路（关键性质：**conf-agent 字节透传**）：

```text
控制面 InnerAPI（/configs/mod-api-key、/configs/tls_conf/server_data_conf）
  │ ConfigExport 生成文件内容（当前明文 JSON）
  ▼
conf-agent：拉取响应字节 → 写盘 conf/<version>/（不解析内容）
  ▼
触发 BFE /reload/<mod> → 模块加载文件 → 解析 → 原子换表
```

涉密钥的导出文件共两个，均明文落盘（代码实证）：

| 文件 | 密钥材质 | BFE 侧证据 |
|---|---|---|
| `mod_api_key.data` | 下游虚拟密钥全量（BFE 按值鉴权） | `bfe_modules/mod_ai_token_auth/token_rule_load.go:38-42`（`tokenFileMap map[string]*TokenFile`，api_key 值是 map 键） |
| `server_data_conf.data` | 上游 provider 密钥（BFE 做上游 key 轮换） | `bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:133-138`（`AIKey.Key // API key value`）、`:223-227`（`AIConf.Keys`）、`bfe_route/bfe_cluster/bfe_cluster.go:35,65` |

**本质认知**：数据面进程内持有机密明文是鉴权机制的物理必然（注入上游
`Authorization: Bearer <key>` 的那一刻必须有明文，Higress/Envoy 同样内存明文）。
加密落盘保护的是存储介质（盘/备份/误打包外泄），不是运行中的数据面。

**目标**（一期）：

1. 下发文件敏感字段磁盘密文化，BFE 加载时解密进内存；
2. 文件保持合法 JSON、非敏感字段人类可读，保留线上 `cat` 排障能力（评审决议）；
3. 灰度可回滚：明文/密文天然兼容，新旧版本 BFE 并存期间行为可预期；
4. 文件密钥独立可轮换（与场景一 DB 主密钥分离，六条论证见《方案》§4.2-b）。

**非目标**：控制面 DB 落盘加密（场景一）；进程内存加密/防内存 dump；防文件篡改
（定位是防泄漏不防篡改）；传输通道 mTLS（另行立项）；方案 A（取消磁盘文件的
拉取式架构，对齐 Envoy xDS，中期演进）。

## 2. 变更总览

加密动作内嵌在控制面（ai-gateway-api 仓）两个 InnerAPI 导出端点的 ConfigExport
过程中，按敏感字段清单加密后序列化；**conf-agent 零改动**（字节透传，密文字节流
自然落到 BFE 磁盘）。BFE 侧只做"加载时解密"。

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 加解密工具 | 新增 `bfe_util/crypto`：信封解密 + keyring（约 150 行，独立实现，格式以《方案》§3.3 为准） | `bfe/bfe_util/crypto/`（新包） |
| mod_ai_token_auth 加载 | tokens map **键**解密，重建明文索引 | `bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go:258`（`tokenMapConvert` 处） |
| server_data_conf 加载 | 遍历解密 `AIConf.Keys[].Key` | `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:1665`（`ClusterConfLoad` 侧） |
| 主配置 | `conf/bfe.conf` 新增 `[Security] KeyFile` 配置项（keyring 文件路径，与 conf 目录异路径） | `bfe/bfe_config/bfe_conf/conf_security.go`（新增，参照 `conf_ai_key_affinity.go` 先例）、`bfe/conf/bfe.conf` 样例 |
| 测试 | 单测（marker 识别/错误密钥/双 keyID 并存/键重建） | `bfe/bfe_util/crypto/*_test.go`、`bfe/bfe_modules/mod_ai_token_auth/*_test.go` |
| 文档同步 | configuration 文档新增 KeyFile 配置说明 | `bfe/docs/zh_cn/configuration/` |

**不改动的部分**（回归锚点）：模块主逻辑零改动——解密完成后内存结构与今天完全
一致（token map 以明文 api_key 为键、`AIConf.Keys[].Key` 为明文）；conf-agent 零
改动；控制面改动不在本仓（接口约定见 §3.1）。

## 3. 详细改动

### 3.1 加密形态与信封格式（接口约定，BFE 侧只消费）

字段级加密（评审决议：不做整文件加密——整文件密文失去 `cat` 排障能力；不加 AAD
——定位防泄漏不防篡改，GCM 单字段完整性校验保留防磁盘位翻转）。加密后形态：

```json
// mod_api_key.data：api_key 值是 tokens 对象的键
{"product1":{"tokens":{"enc$v1$9mJz...":{"key_id":"k1","enable":true}}}}

// server_data_conf.data 的 AIConf 段：只加密 Key 字段值
"keys":[{"name":"k1","key":"enc$v1$Ab3x...","weight":100}]
```

信封格式（与场景一同一项目规范，两场景仅密钥材料不同）：

```text
enc$v1$<base64( keyID(1B) | nonce(12B) | AES-256-GCM( fieldPlaintext ) )>
```

- 无 `enc$v1$` 前缀按明文直通（灰度/回滚/新旧并存天然安全）；
- 选钥**按密文自带 keyID 查 keyring**，无需字段级密钥映射；
- 膨胀仅密钥值 ≈1.36×+44B；加载时 N 次解密（N=密钥条数，千级 ≈ 微秒×N），性能可忽略。

### 3.2 `bfe_util/crypto`：信封解密 + keyring（新增包）

职责：

- `HasMarker(s string) bool`：识别 `enc$v1$` 前缀；
- `DecryptWithKeyring(ciphertext string, keyring) (plaintext string, error)`：
  解析 keyID → keyring 查表选钥 → AES-256-GCM 解密 → 返回明文；
- keyring 加载：从 `[Security] KeyFile` 指定的 keyring 文件读取多 keyID 并存
  （轮换期新旧钥同挂），文件缺失/格式错误按 §3.5 失败语义处理；
- 密文**不落日志**：失败日志只记文件/keyID/长度。

一期各仓独立实现（与 ai-gateway-api 侧 `lib/xcrypto` 规范统一，不共享依赖），
二期再收敛为 go-lib 共享包（《方案》§8.2-Q2）。

### 3.3 `mod_ai_token_auth`：tokens map 键解密（`token_rule_load.go`）

文件结构 `product → tokens(map[api_key]Token)`，**api_key 值是 map 键**，Token 内
key_id/quota/subnet 等非敏感不加密。改动点在 `tokenMapConvert`
（`token_rule_load.go:258`）：

```text
现状：for key, tokenFile := range *tokenFileMap → tokenMap[key] = &token
新：  遍历 *tokenFileMap：
        ├─ HasMarker(key)? 无 → tokenMap[key] 原样（明文直通）
        └─ 有 → keyring 选钥（按密文 keyID）→ Decrypt → 明文键 newKey
              → tokenMap[newKey] = &token
```

要点：map 键解密后**重建明文索引**，解密失败整次加载报错（reload 失败、旧配置
保留）；不允许跳过单条继续（半解密状态会静默丢鉴权覆盖）。

### 3.4 `server_data_conf`：`AIConf.Keys[].Key` 解密（cluster_conf 加载侧）

`ClusterConfLoad`（`cluster_conf_load.go:1665`）解析完成后、
`AIConfCheck`（`:1177`）校验前，遍历 `AIConf.Keys`：

```text
for i := range conf.AIConf.Keys:
    HasMarker(Keys[i].Key)? 无 → 直通
    有 → keyring 选钥 → Decrypt → 写回 Keys[i].Key（明文）
```

同条目 Name/Weight、模型映射、协议路径、亲和参数均非敏感，不加密；解密后内存
结构与今天一致，上游 key 轮换/亲和逻辑零改动。

### 3.5 失败语义

| 场景 | 语义 | 依据 |
|---|---|---|
| 无 marker 的明文文件 | 直通解析 | marker 兼容设计 |
| 有 marker、密钥缺失/解密失败/GCM 认证失败 | **reload 失败，旧生效配置保留**；日志记文件/keyID/长度，密文不落日志 | BFE 模块 reload 范式（`mod_ai_token_auth.go:382` `loadProductRuleConf`） |
| 首次启动（非 reload）即解密失败 | 模块 Init 失败 → BFE 拒绝启动（fail-fast） | 容忍启动会静默丢失全部鉴权配置，风险更大（《方案》§8.2-Q4） |
| 全明文文件 + 无密钥 | 正常启动 | 渐进启用 |

### 3.6 密钥注入与配置

评审已决议（《方案》§8.2-Q1）：文件或 K8s Secret 挂载，**不提供 env**（/proc 与
core dump 泄漏面、单一代码路径）。

**配置落点：BFE 主配置 `conf/bfe.conf` 新增 `[Security]` 段**（不放模块 conf）：

```ini
# conf/bfe.conf：KeyFile 为 keyring 文件（多 keyID 并存），轮换期新旧钥同挂
[Security]
KeyFile = "/etc/ai-gateway/keys/export.keys"
```

不放模块 conf 的理由（两个解密点的归属不同）：

- `mod_api_key.data` 由 `mod_ai_token_auth` 模块加载，但 `server_data_conf.data`
  由 route/server 层加载（`bfe_route/cluster_table.go:50` `ClusterConfLoad`，
  经 `bfe_server/bfe_confdata_load.go:127` `LoadServerDataConf` 触发），**不归属
  任何模块**；
- 启动顺序上 `InitDataLoad()` 先于 `InitModules()`（`bfe_server_init.go:50-84`），
  route 层加载时模块尚未初始化，也读不到模块 conf；
- 主配置 `bfe.conf` 由 `bfe_conf.BfeConfigLoad` 解析为 `BfeConfig`（`bfe.go:103`），
  server 层经 `srv.Config` 可达，模块可按 confRoot 重载——两个解密点均可见；
  有 `[AIKeyAffinity]` 同层段先例（`bfe_config/bfe_conf/conf_ai_key_affinity.go`）；
  `bfe.conf` 只含密钥文件路径、不含密钥材质，控制面/conf-agent 下发无泄漏问题。

**热加载语义：KeyFile 路径静态，keyring 内容热加载。**

- 路径无热更需求：`bfe.conf` 本就只在启动时加载，而轮换只改 keyring 内容（加新钥/
  摘旧钥），路径终身不变，不值得为静态路径引入主配置热更机制；
- keyring 内容热加载：两个加载点**每次加载 data 文件时重读 keyring 文件**（不在
  Init 时缓存），轮换 = 更新 keyring 文件 → 触发既有 reload 端点（
  `/reload/mod_ai_token_auth`、`/reload/server_data_conf`，
  `mod_ai_token_auth.go:382`、`web_server.go:130`）→ 新钥生效，**无需滚动重启**；
- K8s 例外：`subPath` 只读挂载不随 Secret 更新刷新（启动时 bind mount），热加载
  需改用**整目录 Secret 只读挂载**（kubelet 同步后原子刷新）；坚持用 `subPath` 则
  维持"改 Secret + 滚动重启"；
- 失败语义兜底：误操作"先摘旧钥、密文未收敛"时 reload 解密失败 → 旧配置保留，
  不会半解密（§3.5）；配合 runbook"先加新钥 → 切加密端 → 收敛后摘旧钥"；
- 可观测：monitor handler 暴露当前已加载 keyID 列表，支撑轮换收敛判定。

部署纪律（前提条件，否则防护归零）：

- 密钥文件 0600、独立属主，**与 conf 目录分目录存放**（否则"conf 目录误打包"把
  密钥连同密文一起外发）；
- 裸机/虚机：密钥文件为唯一注入方式，轮换 = 更新文件 + 触发 reload（热加载）；
- K8s：优先整目录 Secret 只读挂载（支持热加载）；如用 `subPath`，轮换 = 改 Secret
  + 滚动重启；
- 严禁进容器镜像、ansible playbook 明文变量、conf 目录、配置中心明文。

文件密钥独立可轮换：泄漏后果有限（控制面重导出即恢复，**可恢复事件**），不要求
轮换 DB 主密钥——这是与场景一密钥分离的意义所在。

## 4. 行为变化与兼容性

**版本错配矩阵（上线顺序硬约束）**：

| 控制面 `EncryptExports` | BFE 版本 | 行为 | 结论 |
|---|---|---|---|
| 关（明文） | 旧版 | 现状 | 基线 |
| 关 | 新版（支持解密） | 正常 | **第一步：全量升级 BFE 新版** |
| 开（密文） | 旧版 | reload 解析失败 → **旧配置保留但不再更新**（危险滞留） | **禁止：必须先完成第一步** |
| 开 | 新版 | 密文加载，磁盘零明文 | 目标态 |

开关 `[Security].EncryptExports = false`（控制面，默认关闭）开启后仅上述两 topic
导出加密；其余 topic（tls、路由规则等无密钥文件）不加密，保持可读可排障。

**回滚**：关 `EncryptExports` → 下一版本文件回到明文 → 旧版 BFE 亦恢复可加载
（双向安全）。

**不变量清单**（回归测试锚点）：解密后内存结构（token map 明文键、
`AIConf.Keys[].Key` 明文）、鉴权匹配逻辑、上游 key 轮换/亲和逻辑、原子换表语义、
reload 失败旧配置保留。

## 5. 测试

1. **单测**（`make test`）：
   - marker 识别（密文/明文/非法串）；
   - 错误密钥/未知 keyID 解密失败；GCM 认证失败；
   - `tokenMapConvert` 键解密重建（含混合明文/密文 map）；
   - `AIConf.Keys` 遍历解密（含混合明文/密文条目）；
   - 双 keyID 并存解密；
2. **集成测试**（本仓 `tests/integration/implementation/scenario-SC26-encrypted-config-fields/`，
   设计文档见 `tests/integration/测试设计文档/scenario-SC26-下发配置字段级加密落盘/`）：
   9 个用例覆盖——密文 token 加载鉴权（含磁盘直读断言）、密文上游 Key 注入、
   明文/密文混合共存、无 keyring reload 失败旧配置保留、错误 keyring 首启 fail-fast
   （错误不含密文）、keyring 热加载轮换、双 keyID 并存、全明文无 keyring 基线；
3. **灰度验证**：§4 矩阵四组合各一例。

## 6. 灰度与回滚

| 阶段 | 动作 | 观察 |
|---|---|---|
| 1 | 全量升级 BFE 新版（`EncryptExports` 仍关） | 行为与基线一致 |
| 2 | 控制面开 `EncryptExports`，灰度实例 | 直读 conf 目录断言无明文；鉴权/上游调用正常 |
| 3 | 全量滚动更新 | 磁盘明文归零；reload 失败率 |

回滚：控制面关 `EncryptExports` → 下一导出周期回到明文 → 任意版本 BFE 可加载
（双向安全）。

## 7. 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| 控制面先开加密、BFE 未全量升级（版本错配） | 旧版 BFE reload 失败、配置滞留 | 上线顺序硬约束（§4 矩阵）：第一步必须全量升级 BFE；`EncryptExports` 默认关 |
| 密钥文件权限未收敛 / 与 conf 目录同路径 | conf 目录误打包把密钥连同密文外发，防护归零 | §3.6 部署纪律：0600/异属主/异路径；发布检查清单；K8s Secret 只读挂载 |
| 首启解密失败 fail-fast | BFE 拒启动 | 发布检查清单前置校验（密钥落位/权限/明文文件兼容启动） |
| 解密失败日志泄漏密文 | 密文外泄（可被离线爆破尝试） | 失败日志只记文件/keyID/长度，密文不落日志 |
| 文件内密文被换位粘贴 | 用错密钥致上游 401（可用性层面） | 威胁模型边界内（防泄漏不防篡改，前提已是磁盘写权限）；GCM 校验防位翻转 |

## 8. 参考路径

- 链路证据：`bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go:38-42,258`、
  `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:382`、
  `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:133-138,223-227,1177,1665`、
  `bfe/bfe_route/bfe_cluster/bfe_cluster.go:35,65`
