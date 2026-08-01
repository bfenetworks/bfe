# mod_session_sticky 基础配置

## 配置简介

`mod_session_sticky.conf` 是 `mod_session_sticky` 模块的基础配置文件，用于指定规则配置文件路径、缓存类型及 Redis 相关配置等。

## 配置描述

### 基础配置

| 配置项          | 类型    | 参数含义                   | 必填 | 补充描述                                                     | 合法性条件                  |
| --------------- | ------- | -------------------------- | ---- | ------------------------------------------------------------ | --------------------------- |
| Basic.DataPath  | String  | 规则配置文件路径           | Y    | 默认值为 `mod_session_sticky/session_sticky.data`            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.CacheSize | Integer | Sticky 模式下本地缓存的大小 | N    | 默认值为 `10000`；仅当 `CacheType` 为 `local` 时生效         | 非负整数                    |
| Basic.CacheType | String  | 缓存类型                   | N    | 默认值为 `local`；可选值为 `local`（本地 LRU 缓存）或 `redis`（Redis 分布式缓存） | 取值范围为 `local`、`redis` |
| Log.OpenDebug   | Boolean | 是否启用模块调试日志开关   | N    | -                                                            | -                           |

### Redis 配置

当 `CacheType` 设置为 `redis` 时，需要配置以下 Redis 相关参数：

| 配置项               | 类型    | 参数含义         | 必填 | 补充描述                                          | 合法性条件 |
| -------------------- | ------- | ---------------- | ---- | ------------------------------------------------- | ---------- |
| Redis.Bns            | String  | BNS 服务名称     | 条件 | `CacheType` 为 `redis` 时必填                     | -          |
| Redis.ConnectTimeout | Integer | 连接超时时间     | 条件 | `CacheType` 为 `redis` 时必填；单位为毫秒         | 正整数     |
| Redis.ReadTimeout    | Integer | 读取超时时间     | 条件 | `CacheType` 为 `redis` 时必填；单位为毫秒         | 正整数     |
| Redis.WriteTimeout   | Integer | 写入超时时间     | 条件 | `CacheType` 为 `redis` 时必填；单位为毫秒         | 正整数     |
| Redis.MaxIdle        | Integer | 最大空闲连接数   | 条件 | `CacheType` 为 `redis` 时必填                     | 非负整数   |
| Redis.MaxActive      | Integer | 最大活跃连接数   | 条件 | `CacheType` 为 `redis` 时必填；`0` 表示不限       | 非负整数   |
| Redis.Password       | String  | Redis 密码       | 条件 | `CacheType` 为 `redis` 时必填；空字符串表示无密码 | -          |
| Redis.ExpireSeconds  | Integer | 缓存过期时间     | 条件 | `CacheType` 为 `redis` 时必填；单位为秒           | 非负整数   |

## 配置示例

### 本地缓存模式

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheSize = 10000
CacheType = local

[Log]
OpenDebug = true
```

### Redis 缓存模式

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheType = redis

[Redis]
Bns = redis.service
ConnectTimeout = 1000
ReadTimeout = 1000
WriteTimeout = 1000
MaxIdle = 10
MaxActive = 100
Password = 
ExpireSeconds = 3600

[Log]
OpenDebug = true
```
