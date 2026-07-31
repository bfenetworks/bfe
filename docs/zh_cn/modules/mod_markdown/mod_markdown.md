# mod_markdown

## 模块简介

mod_markdown 用于将 markdown 文件实时渲染为 HTML 后返回给客户端。

## 基础配置

### 配置描述

模块配置文件: conf/mod_markdown/mod_markdown.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.ProductRulePath | String<br>Markdown 规则文件路径<br>默认值 mod_markdown/markdown_rule.data |
| Log.OpenDebug         | Boolean<br>是否开启 debug 日志<br>默认值 False |

### 配置示例

```ini
[Basic]
ProductRulePath = mod_markdown.data

[Log]
OpenDebug = true
```

## 规则配置

### 配置描述

规则配置文件: markdown_rule.data

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的 Markdown 渲染规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Object<br>产品线下的规则列表 |
| Config{v}[] | Object<br>规则详细信息 |
| Config{v}[].Cond | String<br>描述匹配请求的条件, 语法详见[Condition](../../condition/condition_grammar.md) |

### 配置示例

```json
{
    "Version": "123",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/md\", false)"
            }
        ]
    }
}
```

## 监控项

| 监控项              | 描述                     |
| ------------------- | ------------------------ |
| REQ_TOTAL           | 请求总数                 |
| REQ_MARK_DOWN_RULE_HIT | 命中 Markdown 规则的请求数 |
| RSP_RENDER_SUCCESS  | 渲染成功的响应数         |
| RSP_RENDER_FAILURE  | 渲染失败的响应数         |
| RSP_RENDER_IGNORE   | 忽略渲染的响应数         |
| ERR_COUNT_READ_FAIL | 读取失败的次数           |
| ERR_COUNT_RENDER_FAIL | 渲染失败的次数           |
