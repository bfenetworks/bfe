# mod_ai_token_auth

## 模块简介

mod_ai_token_auth 支持大模型 api-key(token) 鉴权。一个 api-key 代表一个对某些大模型服务拥有一定访问权限和配额的令牌。在此模块中根据规则对请求中携带的 api-key 进行检查，决定该请求是否允许访问大模型服务。

请求 header 携带 api-key:
```
Authorization: Bearer <api-key>
```

## 基础配置

模块基础配置文件说明详见 [mod_ai_token_auth.conf](../../configuration/mod_ai_token_auth/mod_ai_token_auth.conf.md)。

## 规则配置

模块规则配置文件说明详见 [token_rule.data](../../configuration/mod_ai_token_auth/token_rule.data.md)。

## 工作原理

### Token 鉴权流程

1. **请求进入**：当请求到达时，模块检查请求是否匹配鉴权规则
2. **Token 验证**：从请求的 `Authorization` Header 中提取 api-key，验证其有效性（状态、过期时间等）
3. **模型权限检查**：验证请求访问的模型是否在允许列表中，且不在禁止列表中
4. **IP 子网检查**：验证请求来源 IP 是否在允许的子网范围内
5. **配额检查**：检查关联的配额计划是否有足够的配额
6. **配额扣除**：请求完成后，从响应体中提取 token 使用量，扣除相应配额

### 监控指标

| 指标名称 | 类型 | 描述 |
| -------- | ---- | ---- |
| REQ_TOTAL | Counter | 总请求数 |
| REQ_AUTH | Counter | 触发鉴权的请求数 |
| REQ_AUTH_FAIL | Counter | 鉴权失败的请求数 |
