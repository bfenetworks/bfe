# mod_header 规则配置

## 配置简介

`mod_header.data` 是 `mod_header` 模块的规则配置文件。

## 配置描述

| 配置项                        | 类型    | 参数含义             | 必填 | 补充描述                                                   | 合法性条件                                           |
| ----------------------------- | ------- | -------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                       | String  | 配置文件版本         | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                        | Object  | 各产品线的 Header 规则 | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                     | String  | 产品线名称           | Y    | -                                                          | -                                                    |
| Config{v}                     | Array   | 产品线的 Header 规则列表 | Y    | -                                                          | -                                                    |
| Config{v}[]                   | Object  | Header 规则          | Y    | -                                                          | -                                                    |
| Config{v}[].Cond              | String  | 规则条件             | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Last              | Boolean | 规则匹配成功后是否继续匹配后续规则 | N    | 默认值为 `false`                                           | -                                                    |
| Config{v}[].Actions           | Array   | 匹配成功后的动作列表 | Y    | -                                                          | -                                                    |
| Config{v}[].Actions[].Cmd     | String  | 动作名称             | Y    | 合法值详见模块动作说明                                     | -                                                    |
| Config{v}[].Actions[].Params  | Array   | 动作参数列表         | N    | 参数依具体动作而定；元素类型为 String                      | -                                                    |

## 模块动作

| 动作名称          | 含义           | 参数列表说明                                                                 |
| ----------------- | -------------- | ---------------------------------------------------------------------------- |
| REQ_HEADER_SET    | 设置请求头     | HeaderName, HeaderValue                                                      |
| REQ_HEADER_ADD    | 添加请求头     | HeaderName, HeaderValue                                                      |
| REQ_HEADER_DEL    | 删除请求头     | HeaderName                                                                   |
| REQ_HEADER_RENAME | 重命名请求头   | OriginalHeaderName, NewHeaderName                                            |
| RSP_HEADER_SET    | 设置响应头     | HeaderName, HeaderValue                                                      |
| RSP_HEADER_ADD    | 添加响应头     | HeaderName, HeaderValue                                                      |
| RSP_HEADER_DEL    | 删除响应头     | HeaderName                                                                   |
| RSP_HEADER_RENAME | 重命名响应头   | OriginalHeaderName, NewHeaderName                                            |
| REQ_HEADER_MOD    | 修改请求头     | scheme_set/query_add, HeaderName, ...                                        |
| RSP_HEADER_MOD    | 修改响应头     | scheme_set/query_add, HeaderName, ...                                        |
| REQ_COOKIE_SET    | 设置请求 Cookie | CookieName, CookieValue                                                      |
| REQ_COOKIE_DEL    | 删除请求 Cookie | CookieName                                                                   |
| RSP_COOKIE_SET    | 设置响应 Cookie | Name, Value, Domain, Path, Expires(RFC1123), MaxAge(int), HttpOnly(bool), Secure(bool) |
| RSP_COOKIE_DEL    | 删除响应 Cookie | Name, Domain, Path                                                           |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "cond": "req_path_prefix_in(\"/header\", false)",
                "actions": [
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Log-Id",
                            "%bfe_log_id"
                        ]
                    },
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Vip",
                            "%bfe_vip"
                        ]
                    },
                    {
                        "cmd": "RSP_HEADER_SET",
                        "params": [
                            "X-Proxied-By",
                            "bfe"
                        ]
                    }
                ],
                "last": true
            }
        ]
    }
}
```
