# mod_static

## 模块简介

mod_static支持返回静态文件作为响应。

## 基础配置

### 配置描述

模块配置文件: conf/mod_static/mod_static.conf

| 配置项         | 描述                               |
| -------------- | ---------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径         |
| Basic.MimeTypePath | String<br>MIME配置文件路径     |

### 配置示例

```ini
[Basic]
DataPath = mod_static/static_rule.data
MimeTypePath = mod_static/mime_type.data

```

## 规则配置

### 配置描述

规则配置文件: conf/mod_static/static_rule.data

| 配置项                      | 描述                                         |
| --------------------------- | -------------------------------------------- |
| Version                     | String<br>配置文件版本                       |
| Config                      | Object<br>各产品线的规则列表                 |
| Config[k]                   | String<br>产品线名称                         |
| Config[v]                   | Object<br>产品线的规则列表                   |
| Config[v][]                 | Object<br>产品线的规则                       |
| Config[v][].Cond            | String<br>规则的匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].Action          | Object br>规则的执行动作                     |
| Config[v][].Action.Cmd      | String<br>动作名称, 合法值包括BROWSE(访问指定目录下的静态文件) |
| Config[v][].Action.Params   | Object<br>动作参数                           |
| Config[v][].Action.Param[0] | String<br>第一个参数为根目录位置             |
| Config[v][].Action.Param[1] | String<br>第二个参数为默认静态文件名         |

### 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [
                        "./",
                        "index.html"
                    ]
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```

## MIME配置

### 配置描述

MIME配置文件: conf/mod_static/mime_type.data

| 配置项                      | 描述                                  |
| --------------------------- | ------------------------------------- |
| Version                     | String<br>配置文件版本                |
| Config                      | Object<br>文件扩展名与MIME类型映射表  |
| Config[k]                   | String<br>文件扩展名                  |
| Config[v]                   | String<br>MIME类型                    |

### 配置示例

```json
{
    "Config": {
        ".avi": "video/x-msvideo",
        ".doc": "application/msword"
    },
    "Version": "20190101000000"
}
```

## 监控项

| 监控项                   | 描述                              |
| ----------------------- | --------------------------------- |
| FILE_BROWSE_COUNT       |统计BROWSE请求数                    |
| FILE_CURRENT_OPENED     |统计当前打开的文件数                  |
| FILE_BROWSE_NOT_EXIST   |文件不存在请求数                     |
| FILE_BROWSE_SIZE        |已处理文件总大小                     |
