# mod_session_sticky 规则配置

## 配置简介

`session_sticky.data` 是 `mod_session_sticky` 模块的规则配置文件。

## 配置描述

| 配置项                          | 类型    | 参数含义                                                      | 必填 | 补充描述                                                     | 合法性条件                    |
| ------------------------------- | ------- | ------------------------------------------------------------- | ---- | ------------------------------------------------------------ | ----------------------------- |
| Version                         | String  | 配置文件版本                                                  | Y    | 通常采用时间戳格式，如 `2024-01-01 00:00:00`                 | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                          | Object  | 各产品线的规则配置                                            | Y    | 以产品线名称为键                                             | -                             |
| Config[k]                       | String  | 产品线名称                                                    | Y    | -                                                            | -                             |
| Config[v]                       | Array   | 产品线规则列表                                                | Y    | -                                                            | -                             |
| Config[v][].Cond                | String  | 规则条件                                                      | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | -                             |
| Config[v][].Type                | String  | 会话保持类型                                                  | N    | 默认值为 `Cookie`；可选值为 `Cookie` 或 `Sticky`             | 取值范围为 `Cookie`、`Sticky` |
| Config[v][].CookieKey           | String  | Cookie 名称                                                   | N    | 默认值为 `bfe_ssbl`                                          | -                             |
| Config[v][].Domain              | String  | Cookie 的 Domain 属性                                         | N    | -                                                            | -                             |
| Config[v][].Path                | String  | Cookie 的 Path 属性                                           | N    | -                                                            | -                             |
| Config[v][].MaxAge              | Integer | Cookie 的 MaxAge 属性                                         | N    | 默认值为 `3600`；单位为秒                                    | 非负整数                      |
| Config[v][].MaskCode            | String  | 主掩码，用于对 Cookie 值进行加密                              | 条件 | Cookie 模式下加密 Cookie 时必填                              | 长度不小于 4                  |
| Config[v][].StandbyMaskCode     | String  | 备用掩码，当主掩码解密失败时使用                              | N    | -                                                            | 长度不小于 4                  |
| Config[v][].Header              | String  | Sticky 模式下，从请求头中获取 stickyid 的字段名               | N    | -                                                            | -                             |
| Config[v][].URIParam            | String  | Sticky 模式下，从 URL 参数中获取 stickyid 的参数名            | N    | -                                                            | -                             |
| Config[v][].StickyRequestField  | String  | Sticky 模式下，从 JSON 请求体中提取 stickyid 的字段名（如 previous_response_id），用于 OpenAI 兼容接口 | N    | -                                                            | -                             |
| Config[v][].StickyResponseField | String  | Sticky 模式下，从 JSON 响应体中提取 stickyid 的字段名（如 response_id），用于 OpenAI 兼容接口 | N    | -                                                            | -                             |
| Config[v][].Secure              | Boolean | Cookie 的 Secure 属性                                         | N    | 默认值为 `false`                                             | -                             |
| Config[v][].HttpOnly            | Boolean | Cookie 的 HttpOnly 属性                                       | N    | 默认值为 `false`                                             | -                             |
| Config[v][].RenewWindow         | Integer | Cookie 续期窗口                                               | N    | 单位为秒；当剩余有效期小于此值时，会重新设置 Cookie；默认值为 `MaxAge` 的一半 | 非负整数                      |

## 配置示例

### Cookie 模式示例

```json
{
    "Version": "2024-01-01 00:00:00",
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "Type": "Cookie",
                "CookieKey": "bfe_ssbl",
                "Domain": ".example.com",
                "Path": "/",
                "MaxAge": 3600,
                "MaskCode": "my_secret_mask_code",
                "StandbyMaskCode": "backup_mask_code",
                "Secure": true,
                "HttpOnly": true,
                "RenewWindow": 1800
            }
        ]
    }
}
```

### Sticky 模式示例

```json
{
    "Version": "2024-01-01 00:00:00",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/api\", true)",
                "Type": "Sticky",
                "CookieKey": "JSESSIONID",
                "Header": "X-Sticky-Id",
                "URIParam": "sticky_id"
            }
        ]
    }
}
```
