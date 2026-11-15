# mod_access_pb3 动态配置（mod_access_pb.data）与请求/响应头白名单采集设计

## 1. 背景与目标

### 1.1 背景

`bfe-access_pb` 协议的 `RequestLog` 自发布起即预留了通用 KV 列表字段：

- `req_headers`（字段 55，`repeated HttpHeader{key,value}`）：关注的请求头列表；
- `res_headers`（字段 158）：关注的响应头列表。

但 `mod_access_pb3` 从未实现采集逻辑——Access Log 经 log-reader → Kafka → Doris 后两列恒空（Doris 明细表对应列早已存在）。业务需要通过 HTTP Header（如 `business_type`、`x-request-id`、`x-trace-id`）做流量分析、按业务维度聚合与问题排查（需求来源：[bfenetworks/bfe#1506](https://github.com/bfenetworks/bfe/issues/1506)）。

### 1.2 目标

1. 支持按**白名单**采集指定的请求头/响应头，写入既有 proto 字段——proto schema、下游 Doris 表、log-reader JSON 契约全部零改动；
2. 白名单**默认不采集**（行为与升级前完全一致），名称匹配**大小写不敏感**；
3. 白名单可**热更新**：调整采集范围无需重启 BFE，且可被上层配置管理（控制面 + conf-agent）统一下发；
4. 数据文件是**模块级通用动态配置载体**：按段（section）组织，本期仅 `access_header` 一个段，未来动态配置以新段加入而不新增文件。

## 2. 方案选型

| 方案 | 结论 | 理由 |
|------|------|------|
| 白名单写 `mod_access_pb3.conf` 静态配置（`[Headers]` 段） | 不采纳 | 模块 conf 仅 Init 时读一次，调整需重启；conf-agent 对模块 conf 只做 CopyFiles 延续、不作为动态配置下发通道 |
| 每类动态配置各建一个 `.data` 文件 | 不采纳 | 文件碎片化；reload 需按文件逐个触发；conf-agent 需为每个文件配 reloader |
| **静态登记 + 动态数据文件（mod_ai_route 模式），数据文件多段化** | **采纳** | `mod_access_pb3.conf` 只登记路径（`DataConfPath`），动态内容在 `mod_access_pb.data`；照搬 mod_ai_route 的加载/热加载/reload 注册五件套（见 `bfe_modules/mod_ai_route/`，conf_load.go / data_load.go / mod_ai_route.go:63-148 / route_table.go:45-63）；多段式使后续扩展（采样率、按 product 差异化等）以新顶层 key 加入同一文件 |

与 mod_ai_route 参照的两点**有意为之的差异**：

1. **数据文件多段化**：`ai_route.data` 单用途；`mod_access_pb.data` 顶层按配置类别分 key，加载框架按"逐段解析、逐段校验、整份原子替换"设计，段间无耦合；
2. **Init 缺失容忍**：`ai_route.data` 缺失会启动失败；`mod_access_pb.data` 缺失按全段缺省初始化 + Warn——存量部署不加文件直接升级行为不变（路由规则缺失是致命配置错误，采集白名单缺失等价于"不启用该能力"）。

## 3. 配置契约

### 3.1 静态配置（`mod_access_pb3.conf`）

```ini
[BasicConf]
OpenDebug = false
DataConfPath = ../conf/mod_access_pb3/mod_access_pb.data
```

`DataConfPath`：**可选**——非空时相对路径基于 conf root 解析（`bfe_util.ConfPathProc`，与 `RouteRulePath` 同规则），缺省不启用动态配置（等价全段缺省）；仅 Init 生效，热加载只针对数据文件。

### 3.2 动态配置（`mod_access_pb.data`，多段式 JSON）

```json
{
    "Version": "20261115090000",
    "access_header": {
        "req_headers": ["business_type", "x-request-id"],
        "res_headers": ["x-trace-id"]
    }
}
```

| 顶层 key | 说明 |
|----------|------|
| `Version` | 版本标识；控制面导出时由数据签名生成，conf-agent 据此比对变更；BFE 不校验内容仅透传 |
| `access_header` | 本期唯一段；缺省/null = 空白名单，不采集任何 header |
| *（未来段）* | 新动态配置以新顶层 key 加入；**未知段忽略 + Warn**（前向兼容：新版控制面写给旧版 BFE 不报错） |

## 4. 热加载机制

```
Init:  loadModAccessPbConf(nil) ──文件缺失──► 全段缺省 + Warn（不失败）
              │存在
              ▼
       锁外 ModAccessPbDataLoad：JSON 解析 → 逐段校验（未知段 Warn）
              │
       锁内 holder.Update：整份引用交换（无中间态）
              ▼
运行时: 读路径 RLock 取快照（每请求一次，无拷贝）

Reload（web monitor 接口，handler 名 "mod_access_pb3"，支持 ?path= 覆盖）:
  加载/校验失败 ──► 返回 error，整份旧配置继续生效（不允许半新半旧）
  成功           ──► 锁内原子替换，即时生效
```

要点：

- **锁外校验、锁内交换**：校验（JSON 结构、token 字符、归一、去重、长度）在锁外完成，锁内只做引用交换；读路径要么见旧配置要么见新配置，无中间态（范式同 `route_table.go:45-63`）；
- **整份失败语义**：任一段校验失败则整份 reload 失败——某段配错会阻塞其它段更新，运维需整份修正，换取运行期无中间态；
- **手工运维路径**：未接入控制面的部署可手工编辑数据文件后调 `/reload/mod_access_pb3` 生效。

## 5. 采集语义

| 决策点 | 语义 |
|--------|------|
| 匹配方式 | 白名单加载时统一归一为 HTTP 规范化形式（`http.CanonicalHeaderKey`），运行时直接 map 查找，零额外开销、**大小写不敏感** |
| 写入键名 | proto `key` 一律写规范化形式（如 `Business-Type`），保证 Doris 聚合键稳定 |
| 多值头 | 同键多值按 `,` 拼接（沿用既有独立字段 `strings.Join(values, ",")` 惯例） |
| 值长限制 | value 截断至 2048 字节（对齐 Doris `VARCHAR(2048)` 列宽），截断不打日志 |
| 响应头来源 | BFE 发往客户端的最终响应头（`reqResponseInfoGen` 的 `res`）；`res == nil` 早退天然跳过 |
| 与独立字段重叠 | 白名单可含已有独立 proto 字段的头（如 `User-Agent`），两者都写、互不影响 |
| 空值/缺头 | 头存在但值为空照常写 `{key, ""}`；头不存在不写条目 |

## 6. 安全设计（红线 bfenetworks/bfe#1357）

1. **运行时全消息兜底**：`mod_access_pb3` 输出前经 `maskSensitiveCredentials`（`credential_mask.go:41`）对整个 `RequestLog` 做递归反射扫描，凡字符串值含原始 API Key 一律替换为 `key_id`（未鉴权置空）——新增字段自动覆盖，零改动；
2. **白名单内容不做敏感头限制**：`Authorization`/`Proxy-Authorization`/`Cookie` 等在数据文件中与其它头同等处理，BFE 加载侧不拒绝；若需限制，由上层配置管理（OpenAPI 校验）承担；
3. **白名单最小化**：默认空、按需开启，避免全量采集的存储开销与敏感信息扩散。

## 7. 边界与兼容性

- **默认行为不变**：不建文件 / `access_header` 缺失 / 空白名单 → proto 字段不序列化，日志体积零增长；
- **协议兼容**：字段 55/158 早已发布，新旧 BFE 与新旧链路任意组合无交叉影响；回滚 = 删数据文件或回滚二进制，下游无 DDL；
- **前后向兼容**：未知段忽略 + Warn（新控制面 → 旧 BFE）；已知段缺失按缺省处理（旧数据文件 → 新 BFE）；
- **热加载边界**：`mod_access_pb3.conf` 本体（`[Log]`、`DataConfPath`）仅 Init 生效。

## 8. 参考文档

- `bfe/docs/zh_cn/modifications/2026-11-15-issue-1506-mod-access-pb3-header-whitelist/design-changes.md`（变更说明）
- `bfe/docs/zh_cn/configuration/mod_access_pb3/mod_access_pb3.conf.md` / `mod_access_pb.data.md`（配置文档）
- `bfe/docs/zh_cn/sys_design/ai_access_log_fields.md`（AI 可观测字段 701-900 设计，与本设计的通用字段互补）
- `bfe_modules/mod_ai_route/`（动态数据文件加载参照实现）
- `bfe-access-pb/docs/protobuf.md`（字段 55/158 与 `HttpHeader` 定义）
