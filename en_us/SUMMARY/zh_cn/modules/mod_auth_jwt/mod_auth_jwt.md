# mod_auth_jwt

## 模块简介

mod_auth_jwt支持JWT([JSON Web Token](https://tools.ietf.org/html/rfc7519))认证

## 基础配置

### 配置描述

模块配置文件: conf/mod_auth_jwt/mod_auth_jwt.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>规则配置的的文件路径 |
| Log.OpenDebug | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
DataPath = mod_auth_jwt/auth_jwt_rule.data
```

## 规则配置

### 配置描述

conf/mod_auth_jwt/auth_jwt_rule.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>配置文件版本 |
| Config      | Struct<br>所有产品线的JWT认证规则配置 |
| Config{k}   | String<br>产品线名称 |
| Config{v}   | Object<br>产品线下 JWT认证规则列表|
| Config{v}[] | Object<br>JWT认证规则 |
| Config{v}[].Cond | String<br>匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].KeyFile | String<br>JWK配置文件 |
| Config{v}[].Realm | String<br>安全域名称<br>默认值"Restricted" |

JWK配置文件说明

* 配置文件必须遵守[JSON Web Key规范](https://tools.ietf.org/html/rfc7517)
* 生成示例Key：

```
echo -n jwt_example | base64 | tr '+/' '-_' | tr -d '='
```

* JWK配置文件示例：

```json
[
    {
        "k": "and0X2V4YW1wbGU",
        "kty": "oct",
        "kid": "0001"
    }
]
```

### 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "mod_auth_jwt/key_file",
                "Realm": "Restricted"
            }
        ]
    }
}
```
