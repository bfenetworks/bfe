# mod_compress

## 模块简介

mod_compress支持对响应主体压缩。

## 基础配置

### 配置描述

模块配置文件: conf/mod_compress/mod_compress.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.DataPath            | String<br>规则配置的的文件路径 |
| Log.OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

- 模块配置文件

```ini
[Basic]
DataPath = mod_compress/compress_rule.data

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的压缩规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Object<br>产品线下的压缩规则列表 |
| Config{v}[] | Object<br>压缩规则详细信息 |
| Config{v}[].Cond | String<br>描述匹配请求或连接的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Action | Object<br>匹配成功后的动作|
| Config{v}[].Action.Cmd | String<br>匹配成功后执行的指令 |
| Config{v}[].Action.Quality | Integer<br>压缩级别 |
| Config{v}[].Action.FlushSize | Integer<br>压缩过程当中的缓存大小 |

### 模块动作

| 动作                    | 含义                     |
| ------------------------| -------------------------|
| GZIP                    | gzip压缩                 |
| BROTLI                    | brotli压缩                 |

### 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 9,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```

## 监控项

| 监控项                   | 描述                              |
| ----------------------- | --------------------------------- |
| REQ_TOTAL               |统计mod_compress处理的总请求数        |
| REQ_SUPPORT_COMPRESS    |支持压缩请求数                       |
| REQ_MATCH_COMPRESS_RULE |命中压缩规则请求数                    |
| RES_ENCODE_COMPRESS     |响应被压缩请求数                      |
