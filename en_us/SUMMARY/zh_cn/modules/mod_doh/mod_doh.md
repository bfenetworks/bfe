# mod_doh

## 模块简介

mod_doh支持DNS over HTTPS。

## 基础配置

### 配置描述

模块配置文件: conf/mod_doh/mod_doh.conf

| 配置项               | 描述                                        |
| ------------------- | ------------------------------------------ |
| Basic.Cond          | String<br>指定DoH请求的条件，详见[Condition](../../condition/condition_grammar.md) |
| Dns.Address         | String<br>DNS服务器地址 |
| Dns.RetryMax        | Int<br>访问DNS最大重试次数<br>默认值0，表示无重试 |
| Dns.Timeout         | Int<br>访问DNS超时时间，单位毫秒 |
| Log.OpenDebug       | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
Cond = "default_t()"

[Dns]
Address = "127.0.0.1:53"
Timeout = 1000

[Log]
OpenDebug = false
```
