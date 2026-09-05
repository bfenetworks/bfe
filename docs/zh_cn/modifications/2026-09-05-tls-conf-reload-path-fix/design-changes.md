# 修复 TLS 热加载 client CA/CRL 路径未随版本目录切换（conf-agent#19）

## 1. 背景

线上收到 [rainway-ai-gateway/conf-agent#19](https://github.com/rainway-ai-gateway/conf-agent/issues/19) 反馈：`bfe/conf` 下 `tls_conf` 是普通目录（mtime 7月16日）而不是符号链接，未指向最新版本目录 `tls_conf_20260904205703`；同时 `tls_conf_*` 版本目录从 8/1 堆积到 9/4 共 14 个无人清理。

影响：BFE 实际使用的 TLS 配置是 7月16日的旧内容，期间所有 TLS 配置变更（证书、规则）均未生效，且 conf-agent 每轮 reload 都在报错。

conf-agent 侧的分析与恢复手册见 `conf-agent/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/`（change-summary.md / design-changes.md）。本文档只覆盖 BFE 侧修复。

---

## 2. BFE 侧根因分析

### 2.1 死锁结构

conf-agent 的 reload 流程为 `Probe → StoreFile2TmpDir → TriggerBFEReload → UpdateDefaultConfDir`：新配置（含 CopyFiles 复制的 `client_ca`、`client_crl` 目录）写入版本目录后，先请求 BFE 热加载，**成功后才切换符号链接**；任何一步失败即中断本轮。

### 2.2 `TLSConfReload` 路径处理不一致（本仓库的根因）

`bfe/bfe_server/bfe_confdata_load.go` 的 `TLSConfReload`：

```go
certConfFile := srv.Config.HttpsBasic.ServerCertConf   // 启动时已 ConfPathProc 绝对化
tlsRuleFile := srv.Config.HttpsBasic.TlsRuleConf
if path := query.Get("path"); path != "" {
    certConfFile = joinPath(path, certConfFile)        // 重定位到版本目录 ✓
    tlsRuleFile = joinPath(path, tlsRuleFile)          // 重定位到版本目录 ✓
}
return srv.tlsConfLoad(certConfFile, tlsRuleFile)
```

而 `tlsConfLoad` 内部读取 client CA / CRL 时：

```go
clientCABaseDir := srv.Config.HttpsBasic.ClientCABaseDir    // <confRoot>/tls_conf/client_ca
clientCRLBaseDir := srv.Config.HttpsBasic.ClientCRLBaseDir  // <confRoot>/tls_conf/client_crl
clientCAMap, err := tls_rule_conf.ClientCALoad(tlsRule.Config, clientCABaseDir)
clientCRLPoolMap, err := tls_rule_conf.ClientCRLLoad(clientCAMap, clientCRLBaseDir)
```

`ClientCABaseDir` / `ClientCRLBaseDir` 在启动时经 `ConfPathProc` 绝对化（`bfe/bfe_config/bfe_conf/conf_https_basic.go`），**reload 时不受 `path` 参数影响**，永远指向当前激活目录。

### 2.3 死锁闭环

1. 新 `tls_rule_conf.data` 引用了新的 client CA（`ClientCAName.crt` 只存在于版本目录的 `client_ca/` 中）。
2. BFE reload：`server_cert_conf.data` / `tls_rule_conf.data` 从版本目录读取成功，`ClientCALoad` 在旧 `tls_conf/client_ca` 下找不到 `.crt` → 报错 → monitor 返回错误 JSON。
3. conf-agent 看到错误后中断本轮，`UpdateDefaultConfDir` 不执行 → 符号链接不切换。
4. 回到 1，无限循环：新文件永远进不了 `tls_conf/` 目录，每次 reload 以同样原因失败。

> `ServerCertParse` / `CheckTlsConf` 校验失败也会造成同样的停滞，但那属于"配置本身有问题，正确地不切换"；client CA/CRL 是"文件已就位、BFE 看错了目录"，属于本仓库的缺陷。

---

## 3. 变更目标

带 `path` 参数调用 `/reload/tls_conf` 时，client CA / CRL 基目录与 cert / tls_rule 一样重定位到版本目录，使版本目录成为自包含的完整 TLS 配置单元，打破死锁。

---

## 4. 变更总览

| 模块 | 主要改动 |
|------|----------|
| `bfe_server` | `TLSConfReload` / `tlsConfLoad` 支持按 `path` 重定位 client CA / CRL 基目录 |
| 测试 | `bfe_server` 补充路径重定向与回归单元测试 |

---

## 5. 详细设计

### 5.1 修改 `TLSConfReload`

`bfe/bfe_server/bfe_confdata_load.go`：

```go
// reload tls conf
certConfFile := srv.Config.HttpsBasic.ServerCertConf
tlsRuleFile := srv.Config.HttpsBasic.TlsRuleConf
clientCABaseDir := srv.Config.HttpsBasic.ClientCABaseDir
clientCRLBaseDir := srv.Config.HttpsBasic.ClientCRLBaseDir
if p := query.Get("path"); p != "" {
    certConfFile = joinPath(p, certConfFile)
    tlsRuleFile = joinPath(p, tlsRuleFile)

    // NEW: client CA / CRL 目录与 cert/tls_rule 同样重定位到版本目录
    tlsConfRoot := filepath.Join(srv.ConfRoot, "tls_conf")
    if strings.HasPrefix(clientCABaseDir, tlsConfRoot+string(filepath.Separator)) {
        clientCABaseDir = joinPath(p, clientCABaseDir)
    }
    if strings.HasPrefix(clientCRLBaseDir, tlsConfRoot+string(filepath.Separator)) {
        clientCRLBaseDir = joinPath(p, clientCRLBaseDir)
    }
}

return srv.tlsConfLoad(certConfFile, tlsRuleFile, clientCABaseDir, clientCRLBaseDir)
```

配套：`tlsConfLoad` 增加 `clientCABaseDir`、`clientCRLBaseDir` 两个入参，内部不再从 `srv.Config` 读取；启动加载路径（`bfe_basic` 调用点）同步改为传入 `srv.Config.HttpsBasic.ClientCABaseDir/ClientCRLBaseDir`，语义不变。

### 5.2 关键说明

- **`joinPath` 语义**：现有 `joinPath(path, suffix)` 取 `suffix` 最后一段拼接，即 `joinPath("tls_conf_v1", "/root/conf/tls_conf/client_ca")` → `tls_conf_v1/client_ca`。版本目录经 conf-agent CopyFiles 已包含 `client_ca`、`client_crl` 完整副本，重定位后文件必然存在。与 cert/tls_rule 的既有处理完全一致，不引入新机制。
- **自定义绝对路径保护**：`ClientCABaseDir` 配置为 `tls_conf` 之外的绝对路径（如 `/mnt/ca`）时，前缀判断不命中，保持从原位置读取，行为不变。
- **不带 `path` 的请求**：所有新逻辑都在 `path != ""` 分支内，手工调用 `/reload/tls_conf`（无 conf-agent 的场景）行为完全不变。

---

## 6. 边界情况

| 场景 | 处理 |
|------|------|
| 带 `path`，版本目录含完整 `client_ca` / `client_crl` | 正常从版本目录加载（修复后核心场景） |
| 带 `path`，版本目录缺少引用的 CA 文件 | `ClientCALoad` 报错，reload 失败（校验逻辑不变，只是目录换了） |
| 带 `path`，CA/CRL 目录配置在 tls_conf 之外 | 不重定向，从配置的绝对路径读取 |
| 不带 `path` | 从 `<confRoot>/tls_conf/client_ca|crl` 读取，行为不变 |
| BFE 直接启动加载 | 传入启动解析的绝对路径，语义不变 |

---

## 7. 测试计划

### 7.1 单元测试（`bfe/bfe_server/bfe_confdata_load_test.go`，新增或扩展）

1. `TestTLSConfReload_ClientCARelocatedWithPath`：构造含新版本 client CA 的临时目录，带 `path` 调用 reload 验证 CA 从版本目录加载成功。
2. `TestTLSConfReload_ClientCAMissingInVersionDir`：版本目录缺少引用的 CA 文件时返回错误。
3. `TestTLSConfReload_CustomAbsoluteCABaseDirNotRelocated`：`ClientCABaseDir` 配为 tls_conf 之外的绝对路径时，带 `path` 也不重定向。
4. `TestTLSConfReload_WithoutPathUsesActivatedDir`：不带 `path` 时从激活目录读取，行为与现状一致。

### 7.2 端到端验证

1. 搭建 conf-agent + BFE 环境，制造"新增 client CA"的配置变更。
2. 验证 `/reload/tls_conf?path=tls_conf_<v>` 返回 `{"error":null}`，conf-agent 打印 `UpdateDefaultConfDir succ`，符号链接切换到新版本目录。
3. 验证旧版本目录按 `VersionKeepCount` 正常清理。

---

## 8. 影响范围

| 文件 | 影响 |
|------|------|
| `bfe/bfe_server/bfe_confdata_load.go` | `TLSConfReload` / `tlsConfLoad` 签名与路径处理 |
| `bfe/bfe_server/*_test.go` | 新增/补充测试 |
| `bfe/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/design-changes.md` | 本文档 |

---

## 9. 兼容性与风险

### 9.1 兼容性

- `/reload/tls_conf` 接口签名与返回格式不变。
- 不带 `path` 的调用、BFE 启动加载路径行为完全不变。
- 版本目录结构、符号链接机制、`VersionKeepCount` 语义均不变。

### 9.2 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 自定义 CA/CRL 绝对路径被错误重定向 | 仅当配置值位于 `<confRoot>/tls_conf` 下才重定向，并有单测覆盖 |
| 版本目录 `client_ca` 副本不完整导致 reload 失败 | CopyFiles 机制保证副本完整；缺失时 reload 正确报错，conf-agent 保持不切换，安全语义不变 |
| 旧 conf-agent + 新 BFE 组合 | 兼容，conf-agent 请求格式不变 |

---

## 10. 实施步骤建议

1. 修改 `TLSConfReload` / `tlsConfLoad`，实现 client CA / CRL 目录随 `path` 重定位。
2. 补充 7.1 所列单元测试。
3. 发布 BFE 新版本（**必须先于存量环境恢复操作部署**）。
4. 存量机器按 `conf-agent/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/change-summary.md` 第 7 节恢复：手工把 `tls_conf` 符号链接指向最新版本目录后即可自愈。
5. 建议 conf-agent 同步升级到 v0.0.6+（目录备份与过期清理），并应用其连续失败日志增强。

---

## 11. 参考资料

- [rainway-ai-gateway/conf-agent#19](https://github.com/rainway-ai-gateway/conf-agent/issues/19)
- `bfe/bfe_server/bfe_confdata_load.go`（`TLSConfReload` / `tlsConfLoad` / `joinPath`）
- `bfe/bfe_config/bfe_conf/conf_https_basic.go`（`ClientCABaseDir` / `ClientCRLBaseDir` 默认值与 `ConfPathProc`）
- `bfe/bfe_config/bfe_tls_conf/tls_rule_conf/tls_rule_conf_load.go`（`ClientCALoad` / `ClientCRLLoad`）
- `bfe/docs/zh_cn/sys_design/tls_conf_reload_path.md`（本修复的系统设计文档）
- `conf-agent/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/`（conf-agent 侧方案与恢复手册）
