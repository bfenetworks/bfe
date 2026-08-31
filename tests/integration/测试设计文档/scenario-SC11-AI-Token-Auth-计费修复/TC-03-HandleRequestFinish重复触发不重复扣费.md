# TC-03 HandleRequestFinish 重复触发不重复扣费

## 目的

验证 issue #1345 修复后，单次请求在 `HandleRequestFinish` 阶段的扣费具有幂等性。

## 前置条件

- BFE 已启动并加载 `cluster_billing_fix`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端配置为返回标准 usage 响应。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `billing-fix.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `Content-Type: application/json` |
| Body | `{"model":"deepseek-chat"}` |

## 后端响应

```json
{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis。
2. 预置 Redis 余额。
3. 发送 TC-03 请求。
4. 等待异步 Redis 扣费完成。
5. 读取 Redis 余额并断言。

## 预期结果

- HTTP 响应状态码为 200。
- `cluster_billing_fix` mock backend 被命中 1 次。
- Redis 余额扣减金额 = `100*100 + 50*200 = 20000`（定点）。
- 若存在重复扣费 bug，实际扣减金额会大于 20000。

## 补充说明

由于集成测试运行在独立 BFE 进程内，无法直接重复调用框架回调，本 TC 通过断言单次请求的扣费金额来间接验证。直接模拟重复回调的覆盖由单元测试 `TestTokenRequestFinishHandler_NoDuplicateDeduction` 承担，该单元测试通过直接调用 `tokenRequestFinishHandler(req, res)` 两次，并校验 mock Redis 只扣减一次。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
