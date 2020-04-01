# 模块简介 

mod_auth_basic支持HTTP基本认证。

# 基础配置
## 配置描述
| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| DataPath            | String<br>规则配置的的文件路径 |
| OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |
## 配置示例

```
[basic]
DataPath = ../conf/mod_auth_basic/auth_basic_rule.data

[log]
OpenDebug = false
```
# 规则配置
## 配置描述
| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>所有产品线的 HTTP 基本认证规则配置 |
| Config.{k} | String<br>产品线名称|
| Config.{v} | Object<br> 产品线下 HTTP 基本认证规则列表 |
| Config.{v}[] | Object<br> HTTP基本认证规则 |
| Config.{v}[].Cond | String<br>匹配条件 |
| Config.{v}[].UserFile | String<br>用户密码文件路径 |
| Config.{v}[].Realm | String<br>认证规则生效范围<br>默认值"Restricted" |
* 用户密码文件说明：
    * 密码使用MD5、SHA1 或 BCrypt 进行哈希编码
    * 可以使用 htpasswd、openssl 生成 userfile 文件
    * openssl 生成密码示例：printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile
    * 用户密码文件配置示例
    ```  
    # user1, 123456
    user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/
    user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
    ```
## 配置示例
```
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