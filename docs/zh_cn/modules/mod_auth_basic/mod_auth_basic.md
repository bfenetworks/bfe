# mod_auth_basic

## 模块简介

mod_auth_basic支持HTTP基本认证。

## 基础配置

### 配置描述

模块配置文件: conf/mod_auth_basic/mod_auth_basic.conf

| 配置项              | 描述                                        |
| ------------------- | ------------------------------------------- |
| Basic.DataPath      | String<br>规则配置的的文件路径 |
| Log.OpenDebug       | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
DataPath = mod_auth_basic/auth_basic_rule.data

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>所有产品线的 HTTP 基本认证规则配置 |
| Config{k} | String<br>产品线名称|
| Config{v} | Object<br> 产品线下 HTTP 基本认证规则列表 |
| Config{v}[] | Object<br> HTTP基本认证规则 |
| Config{v}[].Cond | String<br>匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].UserFile | String<br>用户密码文件路径 |
| Config{v}[].Realm | String<br>安全域名称<br>默认值"Restricted" |

用户密码文件说明：

* 密码使用MD5、SHA1 或 BCrypt 进行哈希编码, 可使用 htpasswd、openssl 生成 userfile 文件
* openssl 生成密码示例

```
printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile
```

* 用户密码文件配置示例

```  
# user1, 123456
user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/
user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
```

### 配置示例

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
    Version": "20190101000000"
}
```

## 监控项

| 监控项                   | 描述                                |
| ----------------------- | ---------------------------------- |
| REQ_AUTH_RULE_HIT       | 命中基本认证规则的请求数               |
| REQ_AUTH_CHALLENGE      | 命中规则、未携带AUTHORIZATION头的请求数 |
| REQ_AUTH_SUCCESS        | 认证成功的请求数                      |
| REQ_AUTH_FAILURE        | 认证失败的请求数                      |
