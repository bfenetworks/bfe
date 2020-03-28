# 模块简介 

基于预定义的规则，对连接或请求进行封禁。

# 基础配置
## 配置描述
| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| ProductRulePath | String<br>封禁规则文件路径 |
| IPBlacklistPath | String<br>全局IP黑名单文件路径 |
* 全局IP黑名单文件说明：
    * 可以配置单独的 IP，也可配置起始 IP
    * 全局IP黑名单文件配置示例
    ```
    192.168.1.253 192.168.1.254
    192.168.1.250
    ```
## 配置示例
```
[basic]
# product rule config file path
ProductRulePath = ../conf/mod_block/block_rules.data
  
# global ip blacklist file path
IPBlacklistPath = ../conf/mod_block/ip_blacklist.data
```

# 规则配置
## 配置描述
| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的封禁规则 |
| Config.{k} | String<br>产品线名称 |
| Config.{v} | Object<br>产品线下的封禁规则列表 |
| Config.{v}[] | Object<br>封禁规则详细信息 |
| Config.{v}[].Cond | String<br>描述匹配请求或连接的条件 |
| Config.{v}[].Name | String<br>规则名称 |
| Config.{v}[].Action | Object<br>匹配成功后的动作|
| Config.{v}[].Action.Cmd | String<br>匹配成功后执行的指令 |
| Config.{v}[].Action.Params | Object<br>执行指令的相关参数列表 |
| Config.{v}[].Action.Params[] | String<br>参数信息 |

## 配置示例
```
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "action": {
                    "cmd": "CLOSE",
                    "params": []
                },
                "name": "example rule",
                "cond": "req_path_in(\"/limit\", false)"            
            }
        ]
    }
}
```
# 封禁动作
| 动作  | 含义     |
| ----- | -------- |
| CLOSE | 关闭连接 |