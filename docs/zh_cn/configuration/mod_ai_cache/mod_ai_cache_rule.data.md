# mod_ai_cache 缓存规则配置

## 配置简介

`mod_ai_cache_rule.data` 是 `mod_ai_cache` 模块的缓存规则文件，按 product 组织规则列表。请求处理时先按 `req.Route.Product` 定位规则列表，再逐条匹配 condition，取第一条命中的规则；product 查不到时直接放行（与其他 AI 模块一致，不回退 global product）。

规则间的区分完全由 condition 承担（如用 `req_body_json_in("model", ...)` 区分不同模型）；`cacheKeyStrategy` 为 `disabled` 的规则表示匹配后不缓存。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 |
| ------ | ---- | -------- | ---- | -------- |
| Version | String | 规则版本号 | Y | - |
| Config | Map\<String, Array\> | product 到规则列表的映射 | Y | key 为 product 名，须与 `host_rule.data` 解析出的 product 一致 |
| cond | String | 规则匹配条件 | Y | 条件表达式语法见[条件原语](../condition/condition_grammar.md) |
| cacheKeyStrategy | String | 缓存键生成策略 | N | `lastQuestion`（默认，取最后一个用户问题）/ `allQuestions`（拼接所有用户问题）/ `disabled`（禁用缓存） |
| cacheTTL | Integer | 缓存 TTL（秒） | N | 默认取基础配置 `DefaultCacheTTL`（3600） |
| cacheKeyFrom | String | 缓存键提取 GJSON PATH | N | 覆盖 `lastQuestion` 策略默认的 `messages.@reverse.0.content` |
| cacheValueFrom | String | 非流式答案提取 GJSON PATH | N | 默认 `choices.0.message.content` |
| cacheStreamValueFrom | String | 流式答案提取 GJSON PATH | N | 默认 `choices.0.delta.content` |
| cacheToolCallsFrom | String | 工具调用提取 GJSON PATH | N | 预留字段，简化版暂未启用 |
| responseTemplate | String | 命中缓存时返回的响应模板 | N | `%s` 为缓存内容占位，默认见配置示例 |
| streamResponseTemplate | String | 命中流式缓存时返回的 SSE 模板 | N | `%s` 为缓存内容占位，默认见配置示例 |
| maxBodyBytes | Integer | 请求体大小上限（字节） | N | 默认 1048576；超限不缓存 |
| maxValueBytes | Integer | 缓存值大小上限（字节） | N | 默认 1048576；超限不写缓存 |

## 配置示例

```json
{
  "Version": "1.0",
  "Config": {
    "default": [
      {
        "cond": "req_path_in(\"/v1/chat/completions\") && req_body_json_in(\"model\", \"deepseek-chat\", false)",
        "cacheKeyStrategy": "lastQuestion",
        "cacheTTL": 3600,
        "maxBodyBytes": 1048576,
        "maxValueBytes": 1048576
      },
      {
        "cond": "default_t()",
        "cacheKeyStrategy": "disabled"
      }
    ]
  }
}
```

## 关键行为说明

- **租户隔离**：缓存键强制带 API Key 的 `key_id` 前缀，不同租户不会读到彼此的缓存。
- **跳过头**：请求头 `x-bfe-skip-ai-cache: on` 时，当前请求既不读缓存也不写缓存，访问日志记录 `ai_cache_status=skip`。
- **命中模板**：缓存命中时，`%s` 会被替换为 JSON 转义后的缓存内容；流式命中返回一段完整 SSE（含 `data:[DONE]`）。
- **计费协同**：命中缓存的请求在 `mod_ai_token_auth` 中跳过 token/费用扣减，访问日志记录 `ai_cache_status=hit`。
