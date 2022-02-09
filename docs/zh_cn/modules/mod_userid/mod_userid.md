# mod_userid

## 模块简介

mod_userid为新用户自动在Cookie中添加用户标识。

## 基础配置

### 配置描述

模块基础配置文件: conf/mod_userid/mod_userid.conf

| 配置项         | 描述                     |
| -------------- | ------------------------ |
| Basic.DataPath | 规则配置文件路径         |
| Log.OpenDebug  | 是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
DataPath = mod_userid/userid_rule.data

[Log]
OpenDebug = true
```

## 规则配置

### 配置描述

模块规则配置文件：conf/mod_userid/userid_rule.data

| 配置项      | 描述                   |
| ----------- | ---------------------- |
| Version     | String<br>配置文件版本 |
| Config    | Object<br>各产品线的规则配置 |
| Config[k] | String<br>产品线名称 |
| Config[v] | Object<br>产品线规则列表 |
| Config[v][].Cond          | 规则条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].Params.Name   | Cookie的Name属性 |
| Config[v][].Params.Domain | Cookie的Domain属性 |
| Config[v][].Params.Path   | Cookie的Path属性 |
| Config[v][].Params.MaxAge | Cookie的MaxAge属性 |

### 配置示例

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/abc\", true)",
                "Params": {
                     "Name": "bfe_userid_abc",
                     "Domain": "",
                     "Path": "/abc",
                     "MaxAge": 3153600
                 },
                 "Generator": "default"
            }, 
            {
                "Cond": "default_t()",
                "Params": {
                     "Name": "bfe_userid",
                     "Domain": "",
                     "Path": "/",
                     "MaxAge": 3153600
                 }
            }
        ]
    }
}
```
