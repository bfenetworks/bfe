# mod_doh 基础配置

## 配置简介

`mod_doh.conf` 是 `mod_doh` 模块的基础配置文件，用于指定 DoH 请求的匹配条件、DNS 服务器地址及日志选项。

## 配置描述

| 配置项          | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.Cond      | String  | 指定 DoH 请求的条件      | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | 须为合法的 Condition 表达式                                  |
| Dns.Address     | String  | DNS 服务器地址           | Y    | 示例：`127.0.0.1:53`                                         | 须为可解析的 UDP 地址                                        |
| Dns.RetryMax    | Integer | 访问 DNS 最大重试次数    | N    | 默认值 `0`，表示无重试                                       | 取值须 `>= 0`                                                |
| Dns.Timeout     | Integer | 访问 DNS 超时时间，单位毫秒 | Y  | -                                                            | 取值须 `> 0`                                                 |
| Log.OpenDebug   | Boolean | 是否开启 debug 日志      | N    | 默认值 `False`                                               | -                                                            |

## 配置示例

```ini
[Basic]
Cond = "default_t()"

[Dns]
Address = "127.0.0.1:53"
Timeout = 1000

[Log]
OpenDebug = false
```
