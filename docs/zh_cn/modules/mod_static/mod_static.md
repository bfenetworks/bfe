# mod_static

## 模块简介

mod_static支持返回静态文件作为响应。

## 基础配置

模块基础配置文件说明详见 [mod_static.conf](../../configuration/mod_static/mod_static.conf.md)。

## 规则配置

模块规则配置文件说明详见 [static_rule.data](../../configuration/mod_static/static_rule.data.md)。

## MIME配置

模块 MIME 配置文件说明详见 [mime_type.data](../../configuration/mod_static/mime_type.data.md)。

## 监控项

| 监控项                   | 描述                              |
| ----------------------- | --------------------------------- |
| FILE_BROWSE_COUNT       |统计BROWSE请求数                    |
| FILE_CURRENT_OPENED     |统计当前打开的文件数                  |
| FILE_BROWSE_NOT_EXIST   |文件不存在请求数                     |
| FILE_BROWSE_SIZE        |已处理文件总大小                     |
| FILE_BROWSE_CONTENT_TYPE_ERROR|Content-Type 获取失败请求数        |
| FILE_BROWSE_FALLBACK_DEFAULT  |回退到默认文件请求数                |
