# TC-01 Anthropic 高 cache 命中按实际 cache token 计费

## 目的

验证 issue #1343 修复后，`calcChatCost` 不再把 `cacheReadTokens` 截断到 `promptTokens`。

## 前置条件

- BFE 已启动并加载 `cluster_billing_fix`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`（100 元）。
- mock 后端配置为返回 Anthropic 标准 usage 响应。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `billing-fix.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"claude-opus-4-6"}` |

## 后端响应

```json
{
    "usage": {
        "input_tokens": 320,
        "output_tokens": 150,
        "cache_read_input_tokens": 8000,
        "cache_creation_input_tokens": 200
    }
}
```

> 说明：在 Anthropic 协议中，`input_tokens` 仅表示 cache miss 的 token 数；`cache_read_input_tokens` 表示 cache 命中的 token 数，两者之和才是总 prompt token 数。

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis。
2. 预置 Redis 余额。
3. 发送 TC-01 请求。
4. 等待异步 Redis 扣费完成（约 500ms）。
5. 读取 Redis 余额并断言。

## 预期结果

- HTTP 响应状态码为 200。
- `cluster_billing_fix` mock backend 被命中 1 次。
- Redis 余额扣减金额 = `8000*45 + 200*565 + 150*2262 = 812300`（定点）。
- 若按旧逻辑截断到 `promptTokens=320`，扣减金额会显著偏低。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
