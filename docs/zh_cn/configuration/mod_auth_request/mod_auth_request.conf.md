# mod_auth_request 基础配置

## 配置简介

`mod_auth_request.conf` 是 `mod_auth_request` 模块的基础配置文件，用于指定认证规则文件路径、认证服务地址及超时时间等。

## 配置描述

| 配置项            | 类型    | 参数含义             | 必填 | 补充描述              | 合法性条件                                                   |
| ----------------- | ------- | -------------------- | ---- | --------------------- | ------------------------------------------------------------ |
| Basic.DataPath    | String  | 规则配置文件路径     | N    | 默认值为 `mod_auth_request/auth_request_rule.data` | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.AuthAddress | String  | 认证服务的地址       | Y    | 如 `http://127.0.0.1` | 须为合法的 URL                                               |
| Basic.AuthTimeout | Integer | 认证超时时间         | Y    | 单位：毫秒            | 必须大于 0                                                   |
| Log.OpenDebug     | Boolean | 是否开启调试日志     | N    | 默认值为 `False`      | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = http://127.0.0.1
AuthTimeout = 100

[Log]
OpenDebug = false
```
