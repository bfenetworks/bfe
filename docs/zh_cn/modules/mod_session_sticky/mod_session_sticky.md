# mod_session_sticky

## 模块简介

mod_session_sticky 用于实现会话保持功能，确保同一用户的请求在会话有效期内始终路由到同一个后端服务。

该模块支持两种会话保持模式：

- **Cookie 模式**：将后端信息加密后存储在 Cookie 中，客户端每次请求携带该 Cookie，BFE 根据 Cookie 内容将请求路由到对应的后端
- **Sticky 模式**：通过缓存机制维护 stickyid 与后端的映射关系，支持从 Cookie、请求头或 URL 参数中获取 stickyid

## 基础配置

### 配置描述

模块基础配置文件：conf/mod_session_sticky/mod_session_sticky.conf

| 配置项 | 描述 |
| ------ | ---- |
| Basic.DataPath | 规则配置文件路径 |
| Basic.CacheSize | Sticky 模式下的缓存大小，默认值为 10000 |
| Log.OpenDebug | 是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheSize = 10000

[Log]
OpenDebug = true
```

## 规则配置

### 配置描述

模块规则配置文件：conf/mod_session_sticky/session_sticky.data

| 配置项 | 描述 |
| ------ | ---- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的规则配置 |
| Config[k] | String<br>产品线名称 |
| Config[v] | Object<br>产品线规则列表 |
| Config[v][].Cond | String<br>规则条件，语法详见 [Condition](../../condition/condition_grammar.md) |
| Config[v][].Type | String<br>会话保持类型，可选值为 "Cookie" 或 "Sticky"，默认为 "Cookie" |
| Config[v][].CookieKey | String<br>Cookie 名称，默认为 "bfe_ssbl" |
| Config[v][].Domain | String<br>Cookie 的 Domain 属性 |
| Config[v][].Path | String<br>Cookie 的 Path 属性 |
| Config[v][].MaxAge | Integer<br>Cookie 的 MaxAge 属性，单位为秒，默认为 3600 |
| Config[v][].MaskCode | String<br>主掩码，用于对 Cookie 值进行加密，长度不小于 4 |
| Config[v][].StandbyMaskCode | String<br>备用掩码，当主掩码解密失败时使用，长度不小于 4 |
| Config[v][].Header | String<br>Sticky 模式下，从请求头中获取 stickyid 的字段名 |
| Config[v][].URIParam | String<br>Sticky 模式下，从 URL 参数中获取 stickyid 的参数名 |
| Config[v][].Secure | Boolean<br>Cookie 的 Secure 属性，默认为 false |
| Config[v][].HttpOnly | Boolean<br>Cookie 的 HttpOnly 属性，默认为 false |
| Config[v][].RenewWindow | Integer<br>Cookie 续期窗口，单位为秒。当剩余有效期小于此值时，会重新设置 Cookie。默认为 MaxAge 的一半 |

### 配置示例

#### Cookie 模式示例

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

#### Sticky 模式示例

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

## 工作原理

### Cookie 模式

1. **Encode（编码阶段）**：当请求首次到达且没有会话保持信息时，BFE 选择后端并将后端信息（地址、端口、子集群）加密后存入 Cookie 返回给客户端
2. **Decode（解码阶段）**：客户端后续请求携带该 Cookie，BFE 解密 Cookie 内容，获取后端信息，将请求路由到对应的后端

### Sticky 模式

1. **Encode（编码阶段）**：当请求首次到达时，BFE 选择后端，并将 stickyid（从 Cookie）与后端信息的映射关系存入缓存
2. **Decode（解码阶段）**：客户端后续请求携带相同的 stickyid（从 Cookie、请求头或 URL 参数中获取），BFE 从缓存中查找对应的后端信息，将请求路由到对应的后端

### Cookie 续期机制

当 Cookie 的剩余有效期小于 `RenewWindow` 时，BFE 会重新设置 Cookie，延长其有效期。这确保了用户在长时间会话中不会因为 Cookie 过期而丢失会话保持状态。

### 掩码兼容

模块支持主掩码和备用掩码，当使用主掩码解密失败时，会尝试使用备用掩码。这在需要更换掩码但又不能中断现有会话时非常有用。