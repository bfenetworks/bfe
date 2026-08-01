# mod_ai_rate_limit 基础配置

## 配置简介

`mod_ai_rate_limit.conf` 是 `mod_ai_rate_limit` 模块的基础配置文件，用于指定限流规则文件路径、Redis 连接参数等。

## 配置描述

| 配置项                     | 类型    | 参数含义                         | 必填 | 补充描述 | 合法性条件 |
| -------------------------- | ------- | -------------------------------- | ---- | -------- | ---------- |
| Basic.ProductRulePath      | String  | 限流规则文件路径                 | Y    | -        | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.IsRejectOnRedisError | Boolean | Redis 出错时是否拒绝请求         | N    | 默认值为 false | - |
| Redis.Bns                  | String  | Redis 代理 BNS 地址              | N    | -        | 非空字符串 |
| Redis.ConnectTimeout       | Integer | 连接 Redis 超时时间              | N    | 单位：毫秒 | 必须大于 0 |
| Redis.ReadTimeout          | Integer | 读取 Redis 超时时间              | N    | 单位：毫秒 | 必须大于 0 |
| Redis.WriteTimeout         | Integer | 写入 Redis 超时时间              | N    | 单位：毫秒 | 必须大于 0 |
| Redis.MaxIdle              | Integer | Redis 连接池最大空闲连接数       | N    | -        | 非负整数 |
| Redis.MaxActive            | Integer | Redis 连接池最大活跃连接数       | N    | 0 表示无限制 | 非负整数 |
| Redis.Password             | String  | Redis 密码                       | N    | 未设置时忽略 | - |
| Log.OpenDebug              | Boolean | 是否开启 debug 日志              | N    | -        | - |

## 配置示例

```ini
[basic]
ProductRulePath = ../conf/mod_ai_rate_limit/ai_rate_limit.data
IsRejectOnRedisError = true

[redis]
bns = BLB.ALB-redis
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = true
```
