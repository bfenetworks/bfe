# 模块简介 

mod_compress支持响应压缩，如：GZIP压缩。

# 基础配置
## 配置描述
| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| DataPath            | String<br>规则配置的的文件路径 |
| OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |
## 配置示例
- 模块配置文件
```
[basic]
DataPath = ../conf/mod_compress/compress_rule.data

[log]
OpenDebug = false
```
# 规则配置
## 配置描述
| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的压缩规则 |
| Config.{k} | String<br>产品线名称 |
| Config.{v} | Object<br>产品线下的压缩规则列表 |
| Config.{v}[] | Object<br>压缩规则详细信息 |
| Config.{v}[].Cond | String<br>描述匹配请求或连接的条件 |
| Config.{v}[].Action | Object<br>匹配成功后的动作|
| Config.{v}[].Action.Cmd | String<br>匹配成功后执行的指令 |
| Config.{v}[].Action.Quality | Integer<br>压缩级别 |
| Config.{v}[].Action.FlushSize | Integer<br>压缩过程当中的缓存大小 |
## 配置示例
```
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

# 压缩动作

| 动作                    | 含义                        |
| ------------------------- | ---------------------------- |
| GZIP                      | gzip压缩                 |


