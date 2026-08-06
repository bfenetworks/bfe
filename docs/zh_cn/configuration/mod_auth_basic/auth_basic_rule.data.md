# mod_auth_basic 规则配置

## 配置简介

`auth_basic_rule.data` 是 `mod_auth_basic` 模块的规则配置文件，用于配置各产品线的 HTTP 基本认证规则。

## 配置描述

| 配置项                 | 类型    | 参数含义                        | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------- | ------- | ------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                | String  | 配置文件版本                    | Y    | -                                                            | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                 | Object  | 所有产品线的 HTTP 基本认证规则配置 | Y    | -                                                            | -                                                            |
| Config{k}              | String  | 产品线名称                      | Y    | -                                                            | -                                                            |
| Config{v}              | Object  | 产品线下 HTTP 基本认证规则列表  | Y    | -                                                            | -                                                            |
| Config{v}[]            | Object  | HTTP 基本认证规则               | Y    | -                                                            | -                                                            |
| Config{v}[].Cond       | String  | 匹配条件                        | Y    | 语法详见[Condition](../../condition/condition_grammar.md)    | -                                                            |
| Config{v}[].UserFile   | String  | 用户密码文件路径                | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Config{v}[].Realm      | String  | 安全域名称                      | N    | 默认值`"Restricted"`                                         | -                                                            |

## 用户密码文件

密码使用 MD5、SHA1 或 BCrypt 进行哈希编码，可使用 htpasswd、openssl 生成 userfile 文件。

openssl 生成密码示例：

```
printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile
```

用户密码文件配置示例：

```  
# user1, 123456
user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/
user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
```

## 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "UserFile": "../conf/mod_auth_basic/userfile",
                "Realm": "example_product"
            }
        ]
    },
    "Version": "20190101000000"
}
```
