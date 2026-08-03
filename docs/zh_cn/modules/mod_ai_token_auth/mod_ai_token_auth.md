# mod_ai_token_auth

## 模块简介

mod_ai_token_auth 支持大模型 api-key(token) 鉴权。一个 api-key 代表一个对某些大模型服务拥有一定访问权限和配额的令牌。在此模块中根据规则对请求中携带的 api-key 进行检查，决定该请求是否允许访问大模型服务。

请求 header 携带 api-key:
```
Authorization: Bearer <api-key>
```

## 基础配置

模块基础配置文件说明详见 [mod_ai_token_auth.conf](../../configuration/mod_ai_token_auth/mod_ai_token_auth.conf.md)。

## 规则配置

模块规则配置文件说明详见 [token_rule.data](../../configuration/mod_ai_token_auth/token_rule.data.md)。

## 核心概念

### Token

Token 是模块在内存中使用的 api-key 运行时对象，由配置文件中的 `Tokens` 转换而来。一个 Token 包含：

- `Key`：api-key 字符串
- `Status`：api-key 状态
- `Name`：名称
- `ExpiredTime`：过期时间，`-1` 表示永不过期
- `UnlimitedQuota`：是否无限配额
- `Models` / `BlockModels`：允许 / 禁止访问的模型列表
- `Subnet`：允许的源 IP 子网列表
- `Tags`：api-key 标签，用于日志或扩展
- `QuotaPlans`：关联的配额计划列表

### Token 状态

| 状态值 | 常量 | 含义 |
| ------ | ---- | ---- |
| 1 | `TokenStatusEnabled` | 启用 |
| 2 | `TokenStatusDisabled` | 禁用 |
| 3 | `TokenStatusExpired` | 已过期 |
| 4 | `TokenStatusExhausted` | 配额已耗尽 |

`TokenStatusExhausted` 由外部系统或配置生成，模块内部主要在日志/监控中使用；实际请求阶段仍通过 Redis 实时判断配额余额。

### QuotaPlan（配额计划）

配额计划描述 api-key 可用的 token 配额，运行时结构如下：

| 字段 | 含义 |
| ---- | ---- |
| `Id` | 配额计划 ID |
| `Unlimited` | 是否为无限配额。`true` 时不读取 Redis，始终放行 |
| `PassNoQuota` | 配额不足时是否放行。`true` 时即使余额为 0 也允许请求通过 |
| `RedisKey` | Redis 中存储该配额余额的 key |
| `CreateTime` | 创建时间 |
| `ExpiredTime` | 过期时间，`-1` 表示永不过期 |
| `Quota` | 配额总量，单位 token |
| `ResetMode` | `0` 非周期性；`1` 周期性配额包 |

> 注意：`Unlimited` 与 `PassNoQuota` 的区别：
> - `Unlimited=true`：不限制配额，不读取 Redis，请求始终有配额。
> - `PassNoQuota=true`：仍然从 Redis 读取余额并扣减，但余额为 0 时不拒绝请求。

## 工作原理

### Token 鉴权流程

1. **请求进入**：当请求到达时，模块检查请求是否匹配鉴权规则
2. **Token 验证**：从请求的 `Authorization` Header 中提取 api-key，验证其有效性（状态、过期时间等）
3. **模型权限检查**：验证请求访问的模型是否在允许列表中，且不在禁止列表中
4. **IP 子网检查**：验证请求来源 IP 是否在允许的子网范围内
5. **配额检查**：检查关联的配额计划是否有足够的配额
6. **配额扣除**：请求完成后，从响应体中提取 token 使用量，扣除相应配额

### 配额检查（HasBalance）

对于每个非 Unlimited 的配额计划，模块在请求阶段调用 `QuotaPlan.HasBalance`：

1. 若 `Unlimited=true`，直接返回有配额。
2. 若 `RedisKey` 为空，返回错误。
3. 通过 Redis `GET` 读取余额：
   - 若 key 不存在（Redis 返回 `nil`），视为首次使用，按满额 `Quota` 处理，返回有配额。
   - 若读取成功，余额 `> 0` 则有配额，否则无配额。
   - 若发生其它 Redis 错误，返回 `INTERNAL_QUOTA_ERROR`。

### 配额扣减（Deduct）

请求成功返回后，模块根据响应中的 token 使用量调用 `QuotaPlan.Deduct`：

1. 若 `Unlimited=true` 或扣减数量 `<= 0`，直接返回。
2. 若 `RedisKey` 为空，返回错误。
3. 通过 Lua 脚本原子执行：
   - 若 Redis key 不存在，先初始化为 `Quota`。
   - 从当前余额中扣减 `min(当前余额, 使用量)`。
   - 返回扣减后的剩余配额。

Lua 脚本逻辑大致如下：

```lua
local raw = redis.call('GET', KEYS[1])
local current
if raw == false then
    current = tonumber(ARGV[2])
    redis.call('SET', KEYS[1], current)
else
    current = tonumber(raw)
end
local amount = tonumber(ARGV[1])
local deduct = math.min(current, amount)
if deduct > 0 then
    redis.call('DECRBY', KEYS[1], deduct)
end
return math.max(0, current - deduct)
```

### Redis key 初始化说明

配额余额 key 由外部系统（如 ai-gateway-api）在创建/重置配额时写入 Redis。若出现以下情况，BFE 会自行兜底：

- 请求阶段 key 不存在：`HasBalance` 按满额处理，允许请求通过。
- 扣减阶段 key 不存在：`Deduct` 先初始化 key 为 `Quota`，再扣减。

这避免了因 Redis key 未及时初始化导致的 `INTERNAL_QUOTA_ERROR`。

### 错误码

模块可能返回以下配额相关错误：

| 错误码 | HTTP 状态码 | 含义 |
| ------ | ----------- | ---- |
| `INTERNAL_QUOTA_ERROR` | 500 | 读取 Redis 配额时发生非 nil 错误 |
| `QUOTA_EXHAUSTED` | 429 | 配额余额不足 |
| `QUOTA_EXPIRED` | 429 | 配额计划已过期 |

其它通用错误码定义见 `bfe_basic/request_ai_basic.go`。

## 与 mod_ai_route 的关系

- `mod_ai_token_auth` 负责 api-key 鉴权、模型权限、子网、配额。
- `mod_ai_route` 负责在鉴权通过后，根据 AI 路由规则把请求转发到目标集群和模型。
- 两者通过 BFE 的模块执行顺序协同工作，通常 `mod_ai_token_auth` 先于 `mod_ai_route` 执行。

## 监控指标

| 指标名称 | 类型 | 描述 |
| -------- | ---- | ---- |
| REQ_TOTAL | Counter | 总请求数 |
| REQ_AUTH | Counter | 触发鉴权的请求数 |
| REQ_AUTH_FAIL | Counter | 鉴权失败的请求数 |
| REQ_AUTH_SUCC | Counter | 鉴权成功的请求数 |
| REQ_QUOTA_EXHAUST | Counter | 因配额不足被拒绝的请求数 |
| REQ_INTERNAL_QUOTA_ERROR | Counter | 因 Redis 配额读取错误被拒绝的请求数 |

> 具体监控项以代码实现为准。
