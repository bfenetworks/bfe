# BFE TLS 配置版本目录热加载设计

## 1. 背景与目标

### 1.1 背景

在 AI 网关部署形态中，TLS 配置（`tls_conf`）由 conf-agent 以**版本目录 + 符号链接**的方式下发：

1. conf-agent 从 ai-gateway-api InnerAPI 导出 `server_cert_conf.data` 等配置，经 extra_files 下载证书/私钥文件，连同 CopyFiles 复制的 `client_ca`、`client_crl`、`tls_rule_conf.data` 等，写入**版本目录** `tls_conf_<version>`；
2. conf-agent 请求 BFE `GET /reload/tls_conf?path=tls_conf_<version>` 做热加载校验；
3. 仅在 BFE 返回成功后，conf-agent 才把 `tls_conf` 符号链接切换到新版本目录，并清理过期版本。

`path` 参数的设计意图是：让 BFE 在符号链接切换**之前**，先对**新版本目录这份自包含的配置单元**做一次完整加载校验——校验通过的配置才会被激活。

### 1.2 问题

BFE 原有的 `TLSConfReload` 只把 `server_cert_conf` / `tls_rule_conf` 两个文件按 `path` 重定位到版本目录，client CA / CRL 基目录仍然固定读激活目录（`<confRoot>/tls_conf/client_ca|crl`）。当变更涉及"激活目录中不存在、只随版本目录下发"的 client CA 时：

- BFE reload 在 `ClientCALoad` 阶段报"找不到 CA 文件"，返回错误；
- conf-agent 因此永不切换符号链接，也永不清理版本目录；
- 下一轮 reload 以同样原因再次失败，形成**永久死锁**（线上事故见 [conf-agent#19](https://github.com/rainway-ai-gateway/conf-agent/issues/19)：TLS 配置停留在一个多月前的旧版本，版本目录堆积 14 个）。

### 1.3 目标

带 `path` 调用 `/reload/tls_conf` 时，client CA / CRL 基目录与 cert / tls_rule 一样重定位到版本目录，使版本目录成为真正自包含的完整 TLS 配置单元，打破死锁；不带 `path` 的调用与 BFE 启动加载路径行为完全不变。

## 2. 设计原则

- **版本目录自包含**：`path` 重定向的前提是版本目录内含完整副本。该前提由 conf-agent 的 CopyFiles 机制保证（`client_ca`、`client_crl` 等目录随每个版本目录复制）。
- **不引入新机制**：复用既有 `joinPath(path, suffix)`（取 suffix 最后一段拼接）的处理方式，与 cert/tls_rule 完全一致。
- **自定义路径保护**：`ClientCABaseDir` / `ClientCRLBaseDir` 配置为 `tls_conf` 之外的绝对路径（如独立挂载的 `/mnt/ca`）时，重定向不生效，从配置的绝对路径读取。
- **校验语义不变**：`ClientCALoad` / `ClientCRLLoad` 的校验逻辑一行不改，只是查找目录随 `path` 变化；版本目录副本缺失时 reload 正确报错，conf-agent 保持不切换，安全语义不变。

## 3. 总体流程

```
conf-agent                                     BFE
   │                                            │
   │  ① 导出配置 + extra_files 下载证书/私钥      │
   │  ② 写入版本目录 tls_conf_<v>                │
   │     （CopyFiles 复制 client_ca/client_crl   │
   │      /tls_rule_conf.data 等完整副本）        │
   │                                            │
   │  ③ GET /reload/tls_conf?path=tls_conf_<v>  ▼
   │                                  TLSConfReload
   │                                  ├─ server_cert_conf ← 版本目录 ✓
   │                                  ├─ tls_rule_conf    ← 版本目录 ✓
   │                                  ├─ client_ca 基目录 ← 版本目录（本设计）
   │                                  ├─ client_crl 基目录← 版本目录（本设计）
   │                                  └─ 全量校验（CheckTlsConf 等）
   │  ④ {"error":null}  ←────────────────────────┤
   │                                            │
   │  ⑤ 切换 tls_conf 符号链接 → tls_conf_<v>     │
   │  ⑥ 按 VersionKeepCount 清理过期版本目录      │
```

关键点：`path` 重定向覆盖**全部四类配置输入**后，第 ③ 步加载的就是第 ⑤ 步将要激活的同一份内容，"先校验后切换"的语义成立。

## 4. 详细设计

### 4.1 路径重定向规则

`bfe/bfe_server/bfe_confdata_load.go` 的 `TLSConfReload`：

```go
certConfFile := srv.Config.HttpsBasic.ServerCertConf   // 启动时已 ConfPathProc 绝对化
tlsRuleFile := srv.Config.HttpsBasic.TlsRuleConf
clientCABaseDir := srv.Config.HttpsBasic.ClientCABaseDir
clientCRLBaseDir := srv.Config.HttpsBasic.ClientCRLBaseDir
if p := query.Get("path"); p != "" {
    certConfFile = joinPath(p, certConfFile)
    tlsRuleFile = joinPath(p, tlsRuleFile)

    // client CA / CRL 目录与 cert/tls_rule 同样重定位到版本目录
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

配套：`tlsConfLoad` 增加 `clientCABaseDir`、`clientCRLBaseDir` 两个入参，内部不再从 `srv.Config` 读取；启动加载路径（`bfe_server.go` 调用点）同步改为传入启动解析的绝对路径，语义不变。

### 4.2 重定向判定

- `joinPath(path, suffix)` 取 `suffix` 最后一段拼接，如 `joinPath("tls_conf_v1", "/root/conf/tls_conf/client_ca")` → `tls_conf_v1/client_ca`。
- client CA / CRL 仅在配置值位于 `<confRoot>/tls_conf` 之下时重定向；`tls_conf` 之外的自定义绝对路径不命中前缀判断，保持原行为。
- 所有新逻辑都在 `path != ""` 分支内，手工调用 `/reload/tls_conf`（无 conf-agent 的场景）行为完全不变。

### 4.3 版本目录自包含性保证

`path` 重定向正确工作的前提是版本目录包含 `client_ca`、`client_crl` 等目录的完整副本，由 conf-agent CopyFiles 机制提供。修复 #19 的过程中发现并修复了 conf-agent 侧的两个相关缺陷：

- CopyFiles 的目录项被拍平到版本目录根（应为 `tls_conf_<v>/client_ca/...`）；
- 空目录（如未配置吊销列表时的 `client_crl/`）复制失败。

详见 `conf-agent/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/`。

## 5. 边界情况

| 场景 | 处理 |
|------|------|
| 带 `path`，版本目录含完整 `client_ca` / `client_crl` | 从版本目录加载（修复后核心场景） |
| 带 `path`，版本目录缺少引用的 CA 文件 | `ClientCALoad` 报错，reload 失败（校验逻辑不变，只是目录换了） |
| 带 `path`，CA/CRL 目录配置在 tls_conf 之外 | 不重定向，从配置的绝对路径读取 |
| 不带 `path` | 从 `<confRoot>/tls_conf/client_ca|crl` 读取，行为不变 |
| BFE 直接启动加载 | 传入启动解析的绝对路径，语义不变 |

## 6. 兼容性

- `/reload/tls_conf` 接口签名与返回格式不变。
- 不带 `path` 的调用、BFE 启动加载路径行为完全不变。
- 版本目录结构、符号链接机制、`VersionKeepCount` 语义均不变。
- 旧 conf-agent + 新 BFE 组合兼容：conf-agent 请求格式不变；反过来新 conf-agent（版本目录自包含）+ 旧 BFE 仍会触发 #19 死锁，**存量环境必须先升级 BFE**。

## 7. 参考资料

- [rainway-ai-gateway/conf-agent#19](https://github.com/rainway-ai-gateway/conf-agent/issues/19)
- `bfe/bfe_server/bfe_confdata_load.go`（`TLSConfReload` / `tlsConfLoad` / `joinPath`）
- `bfe/bfe_config/bfe_conf/conf_https_basic.go`（`ClientCABaseDir` / `ClientCRLBaseDir` 默认值与 `ConfPathProc`）
- `bfe/bfe_config/bfe_tls_conf/tls_rule_conf/tls_rule_conf_load.go`（`ClientCALoad` / `ClientCRLLoad`）
- `bfe/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/design-changes.md`（本次变更记录）
- `conf-agent/docs/zh_cn/modifications/2026-09-05-tls-conf-reload-path-fix/`（conf-agent 侧方案与线上恢复手册）
