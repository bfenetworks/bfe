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
| Log.OpenDebug       | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[basic]
ProductRulePath = mod_ai_token_auth/token_rule.data

[redis]
# bns addr
bns = BLB.ALB-redis

# timeout in ms
connectTimeout = 20
readTimeout = 20
writeTimeout = 20

# max idle connections
maxIdle = 20

[log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Version | String<br>配置文件版本 |
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

api-key 声明的数据结构：
```
struct {
	Key            string           // api-key
	Status         int              // api-key的状态：1 - Enabled; 2 - Disabled; 3 - Expired; 4 - Exhausted
	Name           string           // 名字
	UpdateTime     int64            // 更新时间 (Unix Time)。改变意味着开启一个新的配额消费周期，重新开始计算UsedQuota。
	ExpiredTime    int64            // 过期时间 (Unix Time)。 -1 - 永不过期
	RemainQuota    int64            // 总可用配额 (单位： token)
	UnlimitedQuota bool             // 是否无限配额
	Models         *string          // 允许的模型列表，多个模型名由逗号分开
	Subnet         *string          // 允许的源ip子网
}
```

### 配置示例

```json
{
    "Config": {
        "example_product" :[
            {
                "cond": "default_t()",
                "action": {
                    "cmd": "CHECK_TOKEN"
                }
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
                "unlimited_quota": true
            }
        }
    },
    Version": "20190101000000"
}
```
