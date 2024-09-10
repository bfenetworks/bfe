# mod_block

## 模块简介

mod_block基于自定义的规则，对连接或请求进行封禁。

## 基础配置

### 配置描述

模块配置文件: conf/mod_block/mod_block.conf

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Basic.ProductRulePath | String<br>封禁规则文件路径 |
| Basic.IPBlocklistPath | String<br>全局IP黑名单文件路径 |

* 全局IP黑名单文件说明：
  * 可以配置单独的 IP，也可配置起始 IP
  * 全局IP黑名单文件配置示例

```
192.168.1.253 192.168.1.254
192.168.1.250
```

### 配置示例

```ini
[Basic]
# product rule config file path
ProductRulePath = mod_block/block_rules.data
  
# global ip blocklist file path
IPBlocklistPath = mod_block/ip_blocklist.data
```

## 规则配置

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的封禁规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Object<br>产品线下的封禁规则列表 |
| Config{v}[] | Object<br>封禁规则详细信息 |
| Config{v}[].Cond | String<br>描述匹配请求或连接的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Name | String<br>规则名称 |
| Config{v}[].Action | Object<br>匹配成功后的动作|
| Config{v}[].Action.Cmd | String<br>匹配成功后执行的指令 |
| Config{v}[].Action.Params | Object<br>执行指令的相关参数列表 |
| Config{v}[].Action.Params[] | String<br>参数信息 |

### 模块动作

| 动作  | 含义     |
| ----- | -------- |
| CLOSE | 关闭连接 |
| ALLOW | 允许请求 |

### 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "global": [
            {
                "action": {
                    "cmd": "ALLOW",
                    "params": []
                },
                "cond": "req_host_in(\"n.example.org\") && req_path_prefix_in(\"/index/\", false) && req_query_key_in(\"space\")",
                "name": "example whiterule"
            }
        ],
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

## 监控项

| 监控项        | 描述                         |
| ------------- | ---------------------------- |
| CONN_TOTAL    | 连接总数                     |
| CONN_REFUSE   | 连接拒绝的总数               |
| CONN_ACCEPT   | 连接接受的总数               |
| REQ_TOTAL     | 请求总数                     |
| REQ_REFUSE    | 请求拒绝的总数               |
| REQ_ACCEPT    | 请求接受的总数               |
| REQ_TO_CHECK  | 检查的请求数                 |
