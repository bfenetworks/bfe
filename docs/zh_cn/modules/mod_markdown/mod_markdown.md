# mod_markdown

## 模块简介

mod_markdown 用于将 markdown 文件实时渲染为 HTML 后返回给客户端。

## 基础配置

模块基础配置文件说明详见 [mod_markdown.conf](../../configuration/mod_markdown/mod_markdown.conf.md)。

## 规则配置

模块规则配置文件说明详见 [markdown_rule.data](../../configuration/mod_markdown/markdown_rule.data.md)。

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
