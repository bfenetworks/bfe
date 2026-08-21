# 简化 `token_rule.data` 配置字段

## 1. 背景

`bfe/docs/zh_cn/configuration/mod_ai_token_auth/token_rule.data.md` 已完成字段精简：

- **QuotaPlan**：删除 `CreateTime`、`ResetMode`。
- **Token**：删除 `status`、`update_time`，由 `enabled`/`expired_time` 表达启用与过期状态。

当前 BFE 代码中这些字段仍然存在，且 `status` 在 `mod_ai_token_auth` 的认证逻辑中被实际使用。为使代码与文档保持一致，需要同步清理相关代码，并将 `status` 承载的状态判断能力迁移到 `enabled` 与 `expired_time`。

---

## 2. 目标

1. 从 `QuotaPlan` 和 `Token` 数据结构中删除已废弃字段。
2. 从配置校验、测试代码、测试数据中删除已废弃字段。
3. 将 Token 状态判断（禁用、过期、配额耗尽）从 `status` 迁移到 `enabled`/`expired_time`/实时配额检查。
4. 保持现有认证、配额检查行为不变。
5. 更新相关设计文档与测试设计文档。

---

## 3. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 数据结构 | 删除 `QuotaPlan.CreateTime`、`QuotaPlan.ResetMode`、`Token.Status`、`Token.UpdateTime`、`TokenFile.Status`、`TokenFile.UpdateTime`；`Token` 新增 `Enabled` | `bfe/bfe_modules/mod_ai_token_auth/token.go` |
| 配置校验 | 删除 `ResetMode` 与 `Status` 校验 | `bfe/bfe_modules/mod_ai_token_auth/token.go`、`token_rule_load.go` |
| 认证逻辑 | `ValidateUserToken`/`ValidateUserTokenByReq` 用 `enabled`/`expired_time` 替代 `status` | `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` |
| 状态常量 | 删除 `TokenStatus*` 常量或仅保留内部使用 | `bfe/bfe_modules/mod_ai_token_auth/token.go` |
| 标签结构 | `ApikeyTag` 新增 `TagLevel` 字段 | `bfe/bfe_basic/request_ai_basic.go` |
| 单元测试 | 删除/调整涉及 `Status`、`UpdateTime`、`CreateTime`、`ResetMode` 的用例 | `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` |
| 测试数据 | 删除测试配置文件中的废弃字段 | `bfe/bfe_modules/mod_ai_token_auth/testdata/mod_ai_token_auth/token_rule.data` |
| 集成测试公共库 | 删除 `BFEConfigBuilder` 中的废弃字段 | `bfe/tests/integration/common/bfe_config_builder.go` |
| 集成测试实现 | 删除 SC03/SC05 测试中的废弃字段 | `bfe/tests/integration/implementation/scenario-SC03-rmb-quota/...`、`scenario-SC05-access-log-ai-fields/...` |
| 设计文档 | 更新 `rmb_quota.md` 中的 `QuotaPlan` 结构说明 | `bfe/docs/zh_cn/sys_design/rmb_quota.md` |
| 测试设计文档 | 删除场景说明中的废弃字段 | `bfe/tests/integration/测试设计文档/...` |

---

## 4. 详细设计

### 4.1 QuotaPlan 字段删除

当前 `QuotaPlan` 中 `CreateTime` 与 `ResetMode` 仅被解析和校验，`ResetMode` 未参与任何配额扣减逻辑，`CreateTime` 完全未被读取。

**删除后结构（`bfe/bfe_modules/mod_ai_token_auth/token.go`）**

```go
type QuotaPlan struct {
    Id          string
    Unlimited   bool
    PassNoQuota bool
    RedisKey    string
    ExpiredTime int64  // -1 means never expired
    Quota       int64  // 配额总量，固定点整数：total_token 时为 Token 数；RMB 时为 1e-8 元
    Unit        string // "total_token" or "RMB"
}
```

**配置校验（`bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go`）**

删除 `quotaPlanCheck` 中的 `ResetMode` 校验：

```go
// 删除以下代码
if conf.ResetMode < 0 || conf.ResetMode > 1 {
    return fmt.Errorf("invalid ResetMode: %d", conf.ResetMode)
}
```

### 4.2 Token 字段删除与状态迁移

当前 Token 状态由 `status` 字段表达，取值：

| 状态码 | 含义 |
|--------|------|
| 1 | 启用 |
| 2 | 禁用 |
| 3 | 过期 |
| 4 | 配额耗尽 |

删除 `status` 后，需迁移为：

| 原状态 | 新判断方式 | 说明 |
|--------|-----------|------|
| 禁用（2） | `!enabled` | `enabled` 字段已存在，表达是否启用 |
| 过期（3） | `expired_time != -1 && expired_time < now` | 与当前 `expired_time` 判断一致 |
| 配额耗尽（4） | 认证阶段实时检查 Redis 配额余额 | 当前代码已在 `ValidateUserTokenByReq` 中执行 `plan.HasBalance`，可完全替代 |

**删除后结构（`bfe/bfe_modules/mod_ai_token_auth/token.go`）**

```go
type Token struct {
    Key            string
    KeyId          string
    Enabled        bool
    ExpiredTime    int64
    UnlimitedQuota bool
    Models         []string
    BlockModels    []string
    Subnet         []*net.IPNet
    Tags           []bfe_basic.ApikeyTag
    QuotaPlans     []*QuotaPlan
}
```

```go
type TokenFile struct {
    Key            string  `json:"key"`
    KeyId          string  `json:"key_id"`
    Enabled        bool     `json:"enabled"`
    ExpiredTime    int64   `json:"expired_time"` // -1 means never expired
    UnlimitedQuota bool    `json:"unlimited_quota"`
    Models         *string `json:"allow_models"`
    BlockModels    *string `json:"block_models"`
    Subnet         *string `json:"subnet"`
    Tags           []bfe_basic.ApikeyTag
    QuotaPlans     []string `json:"quota_plans"`
    models         []string
    blockModels    []string
    subnet         []*net.IPNet
}
```

**配置校验（`bfe/bfe_modules/mod_ai_token_auth/token.go`）**

删除 `tokenCheck` 中的 `Status` 校验：

```go
// 删除以下代码
if conf.Status < TokenStatusEnabled || conf.Status > TokenStatusExhausted {
    return fmt.Errorf("invalid Status: %d", conf.Status)
}
```

`tokenConvert` 需将 `Enabled` 从 `TokenFile` 复制到 `Token`：

```go
return Token{
    Key:            tokenFile.Key,
    KeyId:          tokenFile.KeyId,
    Enabled:        tokenFile.Enabled,
    ExpiredTime:    tokenFile.ExpiredTime,
    UnlimitedQuota: tokenFile.UnlimitedQuota,
    Models:         tokenFile.models,
    BlockModels:    tokenFile.blockModels,
    Subnet:         tokenFile.subnet,
    Tags:           tokenFile.Tags,
    QuotaPlans:     quotaPlans,
}, nil
```



### 4.3 认证逻辑改造

`bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` 中 `ValidateUserToken` 与 `ValidateUserTokenByReq` 当前通过 `token.Status` 判断状态。

#### 4.3.1 `ValidateUserToken` 改造

当前逻辑：

```go
switch token.Status {
case TokenStatusExhausted:
    return nil, fmt.Errorf("token %s quota exhausted", token.KeyId)
case TokenStatusExpired:
    return nil, fmt.Errorf("token %s expired", token.KeyId)
case TokenStatusDisabled:
    return nil, fmt.Errorf("token %s disabled", token.KeyId)
}

if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
    token.Status = TokenStatusExpired
    return nil, fmt.Errorf("token %s expired", token.KeyId)
}
```

改造后：

```go
if !token.Enabled {
    return nil, fmt.Errorf("token %s disabled", token.KeyId)
}

if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
    return nil, fmt.Errorf("token %s expired", token.KeyId)
}
```

> `TokenStatusExhausted` 不再通过配置表达，认证阶段会在后续 `QuotaPlans` 余额检查中统一处理。

#### 4.3.2 `ValidateUserTokenByReq` 改造

当前逻辑：

```go
switch token.Status {
case TokenStatusExhausted:
    SetApiAuthInfo(req, bfe_basic.CodeInvalidApiKey, nil)
    return nil, bfe_basic.NewAiErrorWithDetails(...)
case TokenStatusExpired:
    ...
case TokenStatusDisabled:
    ...
}

if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
    token.Status = TokenStatusExpired
    ...
}
```

改造后：

```go
if !token.Enabled {
    SetApiAuthInfo(req, bfe_basic.CodeKeyDisabled, nil)
    return nil, bfe_basic.NewAiErrorWithDetails(
        bfe_basic.CodeKeyDisabled,
        bfe_basic.TypeAuthenticationError,
        fmt.Sprintf("Invalid API key: %s. disabled.", key),
        &bfe_basic.AiErrorDetail{ApiKey: key, KeyId: token.KeyId},
    )
}

if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
    return nil, bfe_basic.NewAiErrorWithDetails(
        bfe_basic.CodeKeyExpired,
        ...,
    )
}
```

### 4.4 状态常量处理

`bfe/bfe_modules/mod_ai_token_auth/token.go` 中当前定义：

```go
const (
    TokenStatusEnabled   = 1
    TokenStatusDisabled  = 2
    TokenStatusExpired   = 3
    TokenStatusExhausted = 4
)
```

由于 `status` 字段删除，这些常量应同步删除。若其他模块通过包引用使用了这些常量，需一并清理。

### 4.5 ApikeyTag 结构增加 TagLevel

`token_rule.data.md` 4.1 节已新增 `TagLevel` 字段，用于表达标签级别（1~5）。需要在 `bfe_basic.ApikeyTag` 结构中添加对应字段：

**当前结构（`bfe/bfe_basic/request_ai_basic.go`）**

```go
type ApikeyTag struct {
    TagName  string //eg entity.type
    TagValue string //eg entity.name
}
```

**改造后结构**

```go
type ApikeyTag struct {
    TagName  string //eg entity.type
    TagValue string //eg entity.name
    TagLevel int    // 标签级别，取值为 1~5 的整数
}
```

> `TagLevel` 当前版本仅作为保留/透传字段，BFE 认证与配额逻辑可不依赖其取值。但结构定义需与配置文档保持一致，以便 JSON 反序列化正确填充。

#### 4.5.1 访问日志序列化（依赖 `bfe-access-pb`）

`bfe/bfe_modules/mod_access_pb3/request_log.go` 将 `AiBasicInfo.ApikeyTags` 序列化为访问日志 protobuf。`bfe-access-pb/bfe_access_pb/bfe_access.proto` 中的 `ApikeyTag` 消息需要同步新增 `tag_level` 字段：

```protobuf
message ApikeyTag {
    optional string tagname  = 1;
    optional string tagvalue = 2;
    optional int32  taglevel = 3;  // 新增
}
```

然后执行 `bfe-access-pb/build.sh` 重新生成 `bfe_access.pb.go`。

生成后，`request_log.go` 中序列化逻辑需补充 `TagLevel`：

```go
reqLog.AiApikeytags = append(reqLog.AiApikeytags, &bfe_access_pb3.ApikeyTag{
    Tagname:  proto.String(tag.TagName),
    Tagvalue: proto.String(tag.TagValue),
    Taglevel: proto.Int32(int32(tag.TagLevel)),
})
```

> 注：`bfe-access-pb/bfe_access.pb.go` 已通过 `build.sh` 重新生成，`request_log.go` 已补充 `TagLevel` 序列化。

---

## 5. 关键代码变更示例

### 5.1 `token.go`

```go
type Token struct {
    Key            string
    KeyId          string
    Enabled        bool
    ExpiredTime    int64
    UnlimitedQuota bool
    Models         []string
    BlockModels    []string
    Subnet         []*net.IPNet
    Tags           []bfe_basic.ApikeyTag
    QuotaPlans     []*QuotaPlan
}

type TokenFile struct {
    Key            string  `json:"key"`
    KeyId          string  `json:"key_id"`
    Enabled        bool     `json:"enabled"`
    ExpiredTime    int64   `json:"expired_time"`
    UnlimitedQuota bool    `json:"unlimited_quota"`
    Models         *string `json:"allow_models"`
    BlockModels    *string `json:"block_models"`
    Subnet         *string `json:"subnet"`
    Tags           []bfe_basic.ApikeyTag
    QuotaPlans     []string `json:"quota_plans"`
    models         []string
    blockModels    []string
    subnet         []*net.IPNet
}

type QuotaPlan struct {
    Id          string
    Unlimited   bool
    PassNoQuota bool
    RedisKey    string
    ExpiredTime int64
    Quota       int64
    Unit        string
}
```

### 5.2 `token_rule_table.go` 中的 `ValidateUserTokenByReq`

```go
if !token.Enabled {
    SetApiAuthInfo(req, bfe_basic.CodeKeyDisabled, nil)
    return nil, bfe_basic.NewAiErrorWithDetails(
        bfe_basic.CodeKeyDisabled,
        bfe_basic.TypeAuthenticationError,
        fmt.Sprintf("Invalid API key: %s. disabled.", key),
        &bfe_basic.AiErrorDetail{ApiKey: key, KeyId: token.KeyId},
    )
}

if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
    SetApiAuthInfo(req, bfe_basic.CodeKeyExpired, nil)
    return nil, bfe_basic.NewAiErrorWithDetails(
        bfe_basic.CodeKeyExpired,
        bfe_basic.TypeAuthenticationError,
        fmt.Sprintf("Invalid API key: %s. expired.", key),
        &bfe_basic.AiErrorDetail{ApiKey: key, KeyId: token.KeyId},
    )
}
```

### 5.3 `token_rule_load.go`

```go
func quotaPlanCheck(conf *QuotaPlan) error {
    if conf.Id == "" {
        return errors.New("no Id")
    }
    if conf.ExpiredTime < -1 {
        return fmt.Errorf("invalid ExpiredTime: %d", conf.ExpiredTime)
    }

    if conf.Unit == "" {
        conf.Unit = quota.UnitTotalToken
    }
    if conf.Unit != quota.UnitTotalToken && conf.Unit != quota.UnitRMB {
        return fmt.Errorf("invalid Unit: %s", conf.Unit)
    }

    if !conf.Unlimited {
        if conf.Unit == quota.UnitRMB {
            if conf.Quota < 0 {
                return fmt.Errorf("invalid Quota for RMB: %d", conf.Quota)
            }
        } else {
            if conf.Quota <= 0 {
                return fmt.Errorf("invalid Quota: %d", conf.Quota)
            }
        }
    }

    return nil
}
```

---

## 6. 测试计划

### 6.1 单元测试

修改 `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go`：

1. 删除/调整 `TestQuotaPlanCheck` 中 `ResetMode` 相关用例。
2. 删除/调整 `TestTokenCheck` 中 `Status` 相关用例，新增 `Enabled` 非法值用例。
3. 删除 `Token` 测试构造中的 `Status`、`UpdateTime` 字段。
4. 调整 `ValidateUserToken`/`ValidateUserTokenByReq` 测试：
   - 禁用场景：构造 `Enabled = false` 的 Token，期望 `CodeKeyDisabled`。
   - 过期场景：构造 `expired_time < now` 的 Token，期望 `CodeKeyExpired`。
   - 配额耗尽场景：保持现有 `plan.HasBalance` 相关测试，确认仍返回 `CodeQuotaExhausted`。

### 6.2 集成测试

1. 更新 `bfe/tests/integration/common/bfe_config_builder.go` 中的 `QuotaPlan` 与 `Token` 结构定义。
2. 更新 SC03、SC05 测试代码中的 Token/QuotaPlan 构造。
3. 更新 `bfe/bfe_modules/mod_ai_token_auth/testdata/mod_ai_token_auth/token_rule.data` 测试数据。
4. 全量运行：

```bash
cd bfe
go test ./bfe_modules/mod_ai_token_auth/...
go test ./tests/integration/...
```

### 6.3 回归测试

- `go test ./bfe_modules/mod_ai_token_auth/...`
- `go test ./tests/integration/...`
- 针对禁用、过期、配额耗尽三种拒绝场景的端到端验证。

---

## 7. 文档更新

1. **`bfe/docs/zh_cn/configuration/mod_ai_token_auth/token_rule.data.md`**
   - 已完成字段精简。

2. **`bfe/docs/zh_cn/sys_design/rmb_quota.md`**
   - 删除 6.2 节 `QuotaPlan` 结构说明中的 `CreateTime`、`ResetMode`。

3. **`bfe/tests/integration/测试设计文档/scenario-SC03-RMB配额扣减/场景说明.md`**
   - 删除示例配置中的 `CreateTime`、`ResetMode`。

4. **`bfe/tests/integration/测试设计文档/scenario-SC05-AI访问日志字段校验/场景说明.md`**
   - 删除示例配置中的 `CreateTime`、`ResetMode`。

5. **`bfe/docs/zh_cn/modifications/2026-08-21-token-rule-data-field-simplification/design-changes.md`**
   - 本设计变更文档（即本文档）。

---

## 8. 影响范围

| 模块/文件 | 影响 |
|-----------|------|
| `bfe/bfe_modules/mod_ai_token_auth/token.go` | 删除字段、调整 `tokenCheck` |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go` | 删除 `ResetMode` 校验 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` | 状态判断逻辑迁移 |
| `bfe/bfe_basic/request_ai_basic.go` | `ApikeyTag` 新增 `TagLevel` |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 单元测试调整 |
| `bfe/bfe_modules/mod_ai_token_auth/testdata/...` | 测试数据更新 |
| `bfe/tests/integration/common/bfe_config_builder.go` | 公共结构定义更新 |
| `bfe/tests/integration/implementation/scenario-SC03/SC05/...` | 集成测试更新 |
| `bfe/docs/zh_cn/sys_design/rmb_quota.md` | 设计文档更新 |
| `bfe/tests/integration/测试设计文档/...` | 测试设计文档更新 |

---

## 9. 兼容性与风险

### 9.1 兼容性

- **配置格式变更**：`token_rule.data` 不再接受 `status`、`update_time`、`CreateTime`、`ResetMode`。旧配置若继续包含这些字段，Go 的 `encoding/json` 会忽略未知字段（不报错），但实际逻辑不再依赖它们。
- **外部系统**：ai-gateway-api 等导出系统需要同步停止导出 `status`、`update_time`、`CreateTime`、`ResetMode`。
- **Redis/配额行为**：删除 `ResetMode` 不影响当前配额扣减，因为该字段从未参与运行时逻辑。

### 9.2 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| `status` 删除后，外部系统依赖 `status=4` 表达配额耗尽 | 改为由 BFE 认证阶段实时检查 Redis 余额；外部系统可在配额耗尽时设置 `enabled=false` 禁用 Key |
| `enabled` 字段语义变化 | 文档明确 `enabled=true` 启用，`false` 禁用 |
| 状态码常量删除影响其他模块 | 全局搜索 `TokenStatus*` 引用，同步清理 |
| 集成测试配置未同步更新 | 全量跑 `bfe/tests/integration/...` 验证 |

**回滚**：若发现兼容性问题，可回滚代码与文档。本次变更不涉及 Redis key 结构或网络协议变更，回滚影响面可控。

---

## 10. 关键代码索引

| 文件 | 行号范围 | 说明 |
|---|---|---|
| `bfe/bfe_modules/mod_ai_token_auth/token.go` | 41-83 | `Token`、`TokenFile`、`QuotaPlan` 结构定义 |
| `bfe/bfe_modules/mod_ai_token_auth/token.go` | 177-224 | `tokenCheck` 校验逻辑 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_load.go` | 119-151 | `quotaPlanCheck` 校验逻辑 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` | 71-96 | `ValidateUserToken` 状态判断 |
| `bfe/bfe_modules/mod_ai_token_auth/token_rule_table.go` | 106-167 | `ValidateUserTokenByReq` 状态判断 |
| `bfe/bfe_modules/mod_ai_token_auth/mod_ai_token_auth_test.go` | 340-450 | Token/QuotaPlan 单元测试 |
