# 下发配置文件敏感字段加密落盘设计（BFE 加载点解密）

## 1. 背景与目标

### 1.1 背景

AI 网关部署形态中，涉密钥的下发配置文件有两个，均经控制面 InnerAPI 导出、
conf-agent **字节透传**写盘（conf-agent 不解析内容），再由 BFE 加载：

| 文件 | 密钥材质 | BFE 加载点 |
|------|----------|------------|
| `mod_ai_token_auth/token_rule.data`（下发名 `mod_api_key.data`） | 下游虚拟密钥全量（BFE 按值鉴权），api-key 值是 Tokens map 的外层键 | `token_rule_load.go` `tokenMapConvert` |
| `server_data_conf/cluster_conf.data` | 上游 provider 密钥（`AIConf.Keys[].Key`，BFE 做上游 key 轮换） | `cluster_conf_load.go` `ClusterConfLoad` |

链路形态：

```
控制面 InnerAPI（ConfigExport 生成文件内容）
  ▼
conf-agent：拉取响应字节 → 写盘 conf/<version>/（字节透传，不解析内容）
  ▼
触发 BFE /reload/<mod> → 读文件 → 解析 JSON → 校验 → 原子换表
```

在导出侧加密之前，这两个文件**明文落盘**：磁盘整体拷贝、备份介质外泄、conf 目录
误打包任一发生即密钥泄漏。

### 1.2 本质认知

数据面进程内持有机密明文是鉴权机制的物理必然——网关在向上游注入
`Authorization: Bearer <key>` 的那一刻必须有明文（Higress/Envoy 的内存中同样明文
存放 apiToken）。**加密落盘保护的是存储介质（盘/备份/误打包外泄），不是运行中的
数据面。**

### 1.3 目标

1. 下发文件敏感字段磁盘密文化，BFE 加载时解密进内存；
2. 文件保持合法 JSON、非敏感字段人类可读，保留线上 `cat` 排障能力；
3. 灰度可回滚：明文/密文天然兼容，新旧版本 BFE 并存期间行为可预期；
4. 文件密钥独立可轮换，与 DB 落盘加密的主密钥分离（暴露面、分发域、轮换成本
   均不同，分离论证见变更文档）。

### 1.4 非目标

控制面数据库落盘加密（另有独立方案）；进程内存加密/防内存 dump；防文件篡改
（定位是防泄漏不防篡改，文件完整性由主机安全与文件权限负责）；下发通道 mTLS
（另行立项）；取消磁盘文件的拉取式架构（对齐 Envoy xDS 的中期演进方向）。

## 2. 设计原则

- **字段级加密，不做整文件加密**：整文件密文会失去 `cat` 排障能力（路由表、配额
  绑定、权重全部不可读）；字段级只加密敏感字段值，文件保持合法 JSON。
- **marker 明文直通**：无 `enc$v1$` 前缀的字段按明文处理——灰度、回滚、新旧版本
  并存天然安全，不需要版本协商。
- **防泄漏不防篡改**：定位是"密钥不落明文盘"。GCM 自带单字段完整性校验（防磁盘
  位翻转产生"可用但错误"的密钥），但不加 AAD、不做文件级防篡改。
- **加密点在控制面导出口**：conf-agent 零改动，密文字节流自然落到 BFE 磁盘；
  BFE 只做"加载时解密"。
- **keyring 多 keyID 并存，加载点每次重读**：轮换期新旧钥同挂；keyring 文件内容
  随每次 data 文件加载重读（热加载），路径配置仅在启动时加载。

## 3. 信封格式与敏感字段

信封格式（字段级，与 DB 落盘加密同一项目规范，仅密钥材料不同）：

```text
enc$v1$<base64( keyID(1B) | nonce(12B) | AES-256-GCM( fieldPlaintext ) )>
```

- 选钥按密文自带 keyID 查 keyring，无需字段级密钥映射；
- 膨胀仅密钥值 ≈1.36×+44B；加载时 N 次解密（N=密钥条数，千级 ≈ 微秒×N），性能
  可忽略。

敏感字段清单与加密后形态：

```json
// token_rule.data：api-key 值是 Tokens 外层键
{"product1":{"tokens":{"enc$v1$9mJz...":{"key_id":"k1","enable":true}}}}

// cluster_conf.data 的 AIConf 段：只加密 Key 字段值
"keys":[{"name":"k1","key":"enc$v1$Ab3x...","weight":100}]
```

Token 内 key_id/quota/subnet、`AIKey` 的 Name/Weight、模型映射、协议路径、亲和
参数均非敏感，不加密。

## 4. 总体流程

```
[控制面] ConfigExport 生成明文结构 → 按敏感字段清单加密字段值（keyID=n）
         → 序列化 → InnerAPI 响应
[conf-agent] 拉取 → 字节写盘（字段级密文）→ 触发 BFE /reload
[BFE] 读文件 → 解析 JSON（文件恒为合法 JSON）
        → 遍历敏感字段：HasMarker?
            ├─ 无 marker → 字段明文直通（兼容/回滚态）
            └─ 有 marker → keyring 按密文 keyID 选钥 → Decrypt
                          → 写回内存结构对应字段
        → 校验 → 原子换表
```

关键点：**先解析后遍历解密**（与整文件方案的"先解密后解析"不同）；解密完成后
内存结构与明文配置完全一致——token map 以明文 api-key 为键、`AIConf.Keys[].Key`
为明文，鉴权匹配、上游 key 轮换/亲和等主逻辑零改动。

## 5. 详细设计

### 5.1 密钥配置：bfe.conf `[Security]` 段

```ini
[Security]
KeyFile = "/etc/ai-gateway/keys/export.keys"   # keyring 文件，多 keyID 并存
```

放在主配置而不放模块 conf 的理由：`cluster_conf.data` 由 route/server 层加载
（`bfe_route/cluster_table.go` `ClusterConfLoad`，经 `bfe_server` `LoadServerDataConf`
触发），不归属任何模块；且启动顺序上 `InitDataLoad()` 先于 `InitModules()`，模块
conf 对该加载点不可达。主配置对两个解密点均可见（有 `[AIKeyAffinity]` 同层段
先例）。默认值空（不解密）；bfe.conf 只含路径、不含密钥材质。

### 5.2 两个加载点

- **token_rule.data**（`mod_ai_token_auth`）：`tokenMapConvert` 遍历 Tokens map，
  外层键带 marker 则解密并以明文键重建索引；解密失败整次加载报错（不允许跳过
  单条继续——半解密状态会静默丢鉴权覆盖）。
- **cluster_conf.data**（route/server 层）：`ClusterConfLoad` 解析完成后、
  `AIConfCheck` 校验前，遍历 `AIConf.Keys` 解密 `Key` 字段。

### 5.3 keyring 与热加载

- keyring 文件支持多 keyID 并存（轮换期新旧钥同挂），格式与 DB 落盘加密的
  keyring 一致；
- 两个加载点**每次加载 data 文件时重读 keyring**（不在 Init 缓存）：
  轮换 = 更新 keyring 文件 → 触发既有 `/reload/mod_ai_token_auth`、
  `/reload/server_data_conf` 端点 → 新钥生效，无需滚动重启；
- K8s 例外：`subPath` 只读挂载不随 Secret 更新刷新，热加载需改用整目录 Secret
  只读挂载（kubelet 同步后原子刷新）；坚持 `subPath` 则维持滚动重启；
- 部署纪律：密钥文件 0600、独立属主、**与 conf 目录分目录存放**（否则 conf 目录
  误打包把密钥连同密文一起外发，防护归零）；严禁进容器镜像/配置中心明文/env。

### 5.4 失败语义

| 场景 | 语义 |
|------|------|
| 无 marker 的明文文件 | 直通解析（渐进启用/回滚态） |
| 有 marker、密钥缺失/解密失败/GCM 认证失败 | reload 失败，旧生效配置保留；日志记文件/keyID/长度，**密文不落日志** |
| 首次启动（非 reload）即解密失败 | 模块 Init 失败 → BFE 拒绝启动（fail-fast；容忍启动会静默丢失全部鉴权配置） |
| 全明文文件 + 无密钥 | 正常启动 |

### 5.5 文件密钥轮换

1. 生成新钥（keyID=n+1），控制面 keyring 追加新钥引用；
2. **BFE 侧先行**：全量实例 keyring 追加新钥（热加载或滚动重启）——先让全部解密
   端具备新钥能力，再让加密端产出新密文；
3. 控制面切 `ActiveExportKeyID = n+1` → 触发一次全量导出；导出文件每次全量重生成，
   下一导出周期即完成全量重加密（自然收敛，无需强制 sweep）；
4. 统计连续 M 个导出周期中旧 keyID 密文数为零后，BFE keyring 摘旧钥、旧钥销毁；
5. 应急轮换（泄漏）：跳过观察期，双钥并存窗口压缩到分发所需最短时间。
   文件钥泄漏 = 控制面重导出即恢复（可恢复事件），不要求轮换 DB 主密钥——这是
   两密钥分离的意义所在。

## 6. 边界情况

| 场景 | 处理 |
|------|------|
| 明文/密文混合文件（灰度中期） | 无 marker 字段直通、有 marker 字段解密，单文件内可混合 |
| 密文字段使用未知 keyID | keyring 查表失败 → 按解密失败处理（reload 失败、旧配置保留） |
| 密文被换位粘贴到其它字段 | GCM 解密失败 → reload 失败；即便误用也是用错密钥致上游 401（可用性层面，前提已是磁盘写权限） |
| 密钥文件被替换为错误内容 | 同解密失败语义，旧配置保留 |
| 控制面开加密、BFE 未升级（版本错配） | 旧版 BFE 解析失败 → reload 失败、旧配置滞留（危险）——上线顺序硬约束：先全量升级 BFE 再开加密 |

## 7. 兼容性

| 控制面导出加密开关 | BFE 版本 | 行为 |
|--------------------|----------|------|
| 关（明文） | 旧版 | 现状（基线） |
| 关 | 新版（支持解密） | 正常（第一步：全量升级 BFE） |
| 开（密文） | 旧版 | reload 失败、旧配置滞留（禁止） |
| 开 | 新版 | 密文加载，磁盘零明文（目标态） |

回滚：控制面关导出加密开关 → 下一导出周期回到明文 → 任意版本 BFE 可加载
（双向安全）。开关默认关闭；开启后仅上述两 topic 加密，其余下发文件不加密。

## 8. 参考资料

- `bfe/docs/zh_cn/modifications/2026-10-05-export-config-field-encryption/design-changes.md`（本次变更记录，含威胁模型与测试计划）
- `bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go`（`tokenMapConvert`）
- `bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`（`AIKey` / `AIConf` / `ClusterConfLoad`）
- `bfe/bfe_route/cluster_table.go`（`ClusterConfLoad` 调用点）
- `bfe/docs/zh_cn/configuration/bfe.conf.md`（`[Security]` 配置说明）
- `bfe/docs/zh_cn/configuration/mod_ai_token_auth/token_rule.data.md`、`bfe/docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md`（敏感字段密文形态说明）
