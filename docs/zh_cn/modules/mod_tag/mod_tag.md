# mod_tag

## 模块简介

mod_tag根据自定义的条件，为请求设置Tag标识。

## 基础配置

### 配置描述

模块配置文件: conf/mod_tag/mod_tag.conf

| 配置项         | 描述                               |
| -------------- | ---------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径         |
| Log.OpenDebug  | String<br>是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
DataPath = mod_tag/tag_rule.data

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

规则配置文件: conf/mod_tag/tag_rule.data

| 配置项                     | 描述                                         |
| -------------------------- | -------------------------------------------- |
| Version                    | String<br>配置文件版本                       |
| Config                     | Object<br>各产品线的规则列表                 |
| Config[k]                  | String<br>产品线名称                         |
| Config[v]                  | Object<br>产品线的规则列表                   |
| Config[v][]                | Object<br>产品线的规则                       |
| Config[v][].Cond           | String<br>规则的匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].Param.TagName  | String<br>标签名称                           |
| Config[v][].Param.TagValue | String<br>标签取值                           |
| Config[v][].Last           | Boolean<br>设置为true时，命中当前规则后停止处理后续规则 |
  
### 配置示例

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
      {
        "Cond": "req_host_in(\"example.org\")",
        "Param": {
          "TagName": "tag",
          "TagValue": "bfe"
        },
        "Last": false
      }
    ]
  }
}
```
