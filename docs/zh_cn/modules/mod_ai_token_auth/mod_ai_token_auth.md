# mod_ai_token_auth

## 模块简介

mod_ai_token_auth 支持大模型 api-key(token) 鉴权。一个 api-key 代表一个对某些大模型服务拥有一定访问权限和配额的令牌。在此模块中根据规则对请求中携带的 api-key 进行检查，决定该请求是否允许访问大模型服务。

请求 header 携带 api-key:
```
Authorization: Bearer <api-key>
```

## 基础配置

### 配置描述

模块配置文件: conf/mod_ai_token_auth/mod_ai_token_auth.conf

| 配置项              | 描述                                        |
| ------------------- | ------------------------------------------- |
| Basic.ProductRulePath      | String<br>api-key声明和规则配置的文件路径 |
| redis.bns | String<br>redis服务的bns名。redis用于存储api-key的配额使用量。 |
| redis.connectTimeout | Integer<br>连接超时时间（毫秒）|
| redis.readTimeout | Integer<br>读取超时时间（毫秒）|
| redis.writeTimeout | Integer<br>写入超时时间（毫秒）|
| redis.maxIdle | Integer<br>最大空闲连接数 |
| redis.maxActive | Integer<br>最大活跃连接数（0表示不限） |
| redis.password | String<br>Redis密码（可选） |
| Log.OpenDebug       | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
ProductRulePath = mod_ai_token_auth/token_rule.data

[Redis]
# bns addr
bns = BLB.ALB-redis

# timeout in ms
connectTimeout = 20
readTimeout = 20
writeTimeout = 20

# max idle connections
maxIdle = 20

# max active connections
maxActive = 100

# redis password (optional)
password = 

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Version | String<br>配置文件版本 |
| QuotaPlans | Object<br>所有产品线的配额计划声明 |
| QuotaPlans{k} | String<br>产品线名称|
| QuotaPlans{v} | Array<br>产品线下的配额计划列表 |
| QuotaPlans{v}[] | Object<br>配额计划，数据结构见下 |
| Tokens | Object<br>所有产品线的 api-key 声明 |
| Tokens{k} | String<br>产品线名称|
| Tokens{v} | Object<br> 产品线下的所以 api-key |
| Tokens{v}{k} | String<br> 一个 api-key |
| Tokens{v}{v} | Object<br> 一个 api-key 声明，数据结构见下。 |
| Config | Object<br>所有产品线的 api-key 鉴权规则配置 |
| Config{k} | String<br>产品线名称|
| Config{v} | Array<br> 产品线下 api-key 鉴权规则列表 |
| Config{v}[] | Object<br> api-key 鉴权规则 |
| Config{v}[].Cond | String<br>匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Action | Object<br>动作。只支持一种动作：{ "cmd": "CHECK_TOKEN" } |

配额计划数据结构：
```
struct {
	Id          string           // 配额计划ID
	Unlimited   bool             // 是否无限配额
	PassNoQuota bool             // 配额不足时是否放行
	RedisKey    string           // Redis中存储配额的key
	CreateTime  int64            // 创建时间 (Unix Time)
	ExpiredTime int64            // 过期时间 (Unix Time)。 -1 - 永不过期
	Quota       int64            // 配额总量 (单位：token)
	ResetMode   int              // 重置模式：0 - 非周期性；1 - 周期性的配额包
}
```

api-key 声明的数据结构：
```
struct {
	Key            string           // api-key
	Status         int              // api-key的状态：1 - Enabled; 2 - Disabled; 3 - Expired; 4 - Exhausted
	Name           string           // 名字
	UpdateTime     int64            // 更新时间 (Unix Time)。改变意味着开启一个新的配额消费周期，重新开始计算UsedQuota。
	ExpiredTime    int64            // 过期时间 (Unix Time)。 -1 - 永不过期
	UnlimitedQuota bool             // 是否无限配额
	Models         *string          // 允许的模型列表，多个模型名由逗号分开
	BlockModels    *string          // 禁止的模型列表，多个模型名由逗号分开
	Subnet         *string          // 允许的源ip子网
	Tags           []ApikeyTag      // api-key标签列表
	QuotaPlans     []string         // 关联的配额计划ID列表
}
```

### 配置示例

```json
{
    "Version": "20190101000000",
    "QuotaPlans": {
        "example_product": [
            {
                "id": "daily_quota",
                "unlimited": false,
                "pass_no_quota": false,
                "redis_key": "ai:quota:daily_quota",
                "create_time": 1672531200,
                "expired_time": -1,
                "quota": 100000,
                "reset_mode": 1
            }
        ]
    },
    "Tokens": {
        "example_product": {
            "TESTKEY": {
                "key": "TESTKEY",
                "status": 1,
                "name": "test",
                "expired_time": -1,
                "unlimited_quota": false,
                "allow_models": "model_a,model_b",
                "block_models": "model_c",
                "subnet": "192.168.0.0/24",
                "tags": [
                    {"key": "department", "value": "engineering"}
                ],
                "quota_plans": ["daily_quota"]
            }
        }
    },
    "Config": {
        "example_product": [
            {
                "cond": "default_t()",
                "action": {
                    "cmd": "CHECK_TOKEN"
                }
            }
        ]
    }
}
```

## 工作原理

### Token 鉴权流程

1. **请求进入**：当请求到达时，模块检查请求是否匹配鉴权规则
2. **Token 验证**：从请求的 `Authorization` Header 中提取 api-key，验证其有效性（状态、过期时间等）
3. **模型权限检查**：验证请求访问的模型是否在允许列表中，且不在禁止列表中
4. **IP 子网检查**：验证请求来源 IP 是否在允许的子网范围内
5. **配额检查**：检查关联的配额计划是否有足够的配额
6. **配额扣除**：请求完成后，从响应体中提取 token 使用量，扣除相应配额

### 监控指标

| 指标名称 | 类型 | 描述 |
| -------- | ---- | ---- |
| REQ_TOTAL | Counter | 总请求数 |
| REQ_AUTH | Counter | 触发鉴权的请求数 |
| REQ_AUTH_FAIL | Counter | 鉴权失败的请求数 |
