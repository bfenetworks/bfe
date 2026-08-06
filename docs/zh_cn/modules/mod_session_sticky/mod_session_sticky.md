# mod_session_sticky

## 模块简介

mod_session_sticky 用于实现会话保持功能，确保同一用户的请求在会话有效期内始终路由到同一个后端服务。

该模块支持两种会话保持模式：

- **Cookie 模式**：将后端信息加密后存储在 Cookie 中，客户端每次请求携带该 Cookie，BFE 根据 Cookie 内容将请求路由到对应的后端
- **Sticky 模式**：通过缓存机制维护 stickyid 与后端的映射关系，支持从 Cookie、请求头或 URL 参数中获取 stickyid

## 基础配置

模块基础配置文件说明详见 [mod_session_sticky.conf](../../configuration/mod_session_sticky/mod_session_sticky.conf.md)。

## 规则配置

模块规则配置文件说明详见 [session_sticky.data](../../configuration/mod_session_sticky/session_sticky.data.md)。

## 工作原理

### Cookie 模式

1. **Encode（编码阶段）**：当请求首次到达且没有会话保持信息时，BFE 选择后端并将后端信息（地址、端口、子集群）加密后存入 Cookie 返回给客户端
2. **Decode（解码阶段）**：客户端后续请求携带该 Cookie，BFE 解密 Cookie 内容，获取后端信息，将请求路由到对应的后端

### Sticky 模式

1. **Encode（编码阶段）**：当请求首次到达时，BFE 选择后端，并将 stickyid（从 Cookie 或 JSON 响应体中获取）与后端信息的映射关系存入缓存
2. **Decode（解码阶段）**：客户端后续请求携带相同的 stickyid（从 Cookie、请求头、URL 参数或 JSON 请求体中获取），BFE 从缓存中查找对应的后端信息，将请求路由到对应的后端

### 缓存类型

模块支持两种缓存类型：

- **local（本地缓存）**：使用 LRU 缓存存储 stickyid 与后端的映射关系，适用于单机部署或无需跨节点共享会话的场景
- **redis（Redis 分布式缓存）**：使用 Redis 存储映射关系，适用于多节点部署场景，确保会话在不同节点间共享

### JSON 请求/响应体字段

对于 OpenAI 兼容接口等场景，模块支持从 JSON 请求体和响应体中提取 stickyid：

- **StickyRequestField**：从 JSON 请求体中提取 stickyid，例如从 `previous_response_id` 字段获取
- **StickyResponseField**：从 JSON 响应体中提取 stickyid，例如从 `response_id` 字段获取

提取优先级（Decode 阶段）：Cookie > Header > URIParam > StickyRequestField

提取优先级（Encode 阶段）：Cookie > StickyResponseField

### Cookie 续期机制

当 Cookie 的剩余有效期小于 `RenewWindow` 时，BFE 会重新设置 Cookie，延长其有效期。这确保了用户在长时间会话中不会因为 Cookie 过期而丢失会话保持状态。

### 掩码兼容

模块支持主掩码和备用掩码，当使用主掩码解密失败时，会尝试使用备用掩码。这在需要更换掩码但又不能中断现有会话时非常有用。

## 监控项

| 监控项  | 描述                 |
| ------- | -------------------- |
| VERSION | 当前生效的规则版本号 |
