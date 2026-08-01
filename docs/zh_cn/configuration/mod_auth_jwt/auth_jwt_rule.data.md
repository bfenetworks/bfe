# mod_auth_jwt 规则配置

## 配置简介

`auth_jwt_rule.data` 是 `mod_auth_jwt` 模块的规则配置文件，用于配置各产品线的 JWT 认证规则。

## 配置描述

| 配置项                 | 类型    | 参数含义                     | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------- | ------- | ---------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                | String  | 配置文件版本                 | Y    | -                                                            | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                 | Object  | 所有产品线的 JWT 认证规则配置 | Y    | -                                                            | -                                                            |
| Config{k}              | String  | 产品线名称                   | Y    | -                                                            | -                                                            |
| Config{v}              | Object  | 产品线下 JWT 认证规则列表    | Y    | -                                                            | -                                                            |
| Config{v}[]            | Object  | JWT 认证规则                 | Y    | -                                                            | -                                                            |
| Config{v}[].Cond       | String  | 匹配条件                     | Y    | 语法详见[Condition](../../condition/condition_grammar.md)    | -                                                            |
| Config{v}[].KeyFile    | String  | JWK 配置文件路径             | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Config{v}[].Realm      | String  | 安全域名称                   | N    | 默认值`"Restricted"`                                         | -                                                            |

## JWK 配置文件

配置文件必须遵守 [JSON Web Key 规范](https://tools.ietf.org/html/rfc7517)。

生成示例 Key：

```
echo -n jwt_example | base64 | tr '+/' '-_' | tr -d '='
```

JWK 配置文件示例：

```json
[
    {
        "k": "and0X2V4YW1wbGU",
        "kty": "oct",
        "kid": "0001"
    }
]
```

## 配置示例

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
