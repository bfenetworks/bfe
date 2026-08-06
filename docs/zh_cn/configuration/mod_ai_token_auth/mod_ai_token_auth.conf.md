# mod_ai_token_auth 基础配置

## 配置简介

`mod_ai_token_auth.conf` 是 `mod_ai_token_auth` 模块的基础配置文件，用于指定 api-key 规则配置文件路径、Redis 连接信息以及日志开关等。

## 配置描述

| 配置项                  | 类型    | 参数含义                                 | 必填 | 补充描述                         | 合法性条件                                                   |
| ----------------------- | ------- | ---------------------------------------- | ---- | -------------------------------- | ------------------------------------------------------------ |
| Basic.ProductRulePath   | String  | api-key 声明和规则配置的文件路径         | N    | 默认值为 `mod_ai_token_auth/token_rule.data` | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Redis.Bns               | String  | Redis 服务的 bns 名                      | Y    | Redis 用于存储 api-key 的配额使用量 | 须为有效的 Redis 服务地址                                    |
| Redis.ConnectTimeout    | Integer | 连接超时时间（毫秒）                     | Y    | -                                | 必须大于 0                                                   |
| Redis.ReadTimeout       | Integer | 读取超时时间（毫秒）                     | Y    | -                                | 必须大于 0                                                   |
| Redis.WriteTimeout      | Integer | 写入超时时间（毫秒）                     | Y    | -                                | 必须大于 0                                                   |
| Redis.MaxIdle           | Integer | 最大空闲连接数                           | Y    | -                                | 必须大于等于 0                                               |
| Redis.MaxActive         | Integer | 最大活跃连接数                           | Y    | 0 表示不限                       | 必须大于等于 0                                               |
| Redis.Password          | String  | Redis 密码                               | N    | 未配置时不启用认证               | -                                                            |
| Log.OpenDebug           | Boolean | 是否开启 debug 日志                      | N    | 默认值为 `False`                 | -                                                            |

## 配置示例

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
