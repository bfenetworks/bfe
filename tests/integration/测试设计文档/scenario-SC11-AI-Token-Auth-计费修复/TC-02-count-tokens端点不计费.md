# TC-02 count_tokens 端点不计费

## 目的

验证 issue #1344 修复后，`tokenRequestFinishHandler` 对 `/count_tokens` 路径跳过扣费逻辑。

## 前置条件

- BFE 已启动并加载 `cluster_billing_fix`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端配置为返回任意 200 响应体（模拟 count_tokens 结果）。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `billing-fix.example.org` |
| Path | `/anthropic/v1/messages/count_tokens` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hello"}]}` |

## 后端响应

```json
{"input_tokens":100}
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis。
2. 预置 Redis 余额。
3. 发送 TC-02 请求到 `/anthropic/v1/messages/count_tokens`。
4. 等待异步处理完成。
5. 读取 Redis 余额并断言。

## 预期结果

- HTTP 响应状态码为 200。
- `cluster_billing_fix` mock backend 被命中 1 次。
- Redis 余额保持 `10000000000` 不变。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
