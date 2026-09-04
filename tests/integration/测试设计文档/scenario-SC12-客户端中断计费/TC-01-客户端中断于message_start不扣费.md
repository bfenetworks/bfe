# TC-01 客户端中断于 message_start 不扣费

## 目的

验证 issue #1352 修复：EstimateToken = true 时，客户端在仅收到 `message_start`（初始 usage，无输出 token）后发送 TCP RST，BFE 不得按完整请求体估算输入 token 全量扣费。

对应实现：`TestTC01_ClientAbortAfterMessageStartNoDeduction`（`scenario-SC12-client-abort-billing/sc12_client_abort_billing_test.go`）。

## 前置条件

- BFE 已启动并加载 `cluster_client_abort`，`EstimateToken = true`。
- Redis 中 `quota:plan_rmb` 余额预置为 `10000000000`。
- mock 后端进入 SSE 模式：发送 `message_start` 帧后阻塞在 `SSEHold`。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | `POST` |
| Host | `abort-billing.example.org` |
| Path | `/anthropic/v1/messages` |
| Header | `Authorization: Bearer ak_user_a` |
| Header | `anthropic-version: 2023-06-01` |
| Body | `{"model":"claude-opus-4-6","stream":true,"max_tokens":1024}` |

## 后端响应（SSE）

```
data: {"type":"message_start","message":{"id":"msg_01","type":"message","usage":{"input_tokens":320,"output_tokens":0}}}

（阻塞，直至客户端中断后释放）

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"<1MB 填充>"}}
```

## 执行步骤

1. 启动 BFE、mock 后端与内存 Redis，预置余额。
2. 客户端通过裸 TCP 发送请求，读取 SSE 数据直至出现 `message_start`。
3. 客户端设置 `SO_LINGER=0` 后关闭连接（发送 TCP RST）。
4. 释放 `SSEHold`，后端追加 1MB 数据帧，强制 BFE 写客户端失败（`ErrClientWrite`）。
5. 等待请求结束，读取 Redis 余额并断言。

## 预期结果

- HTTP 响应状态码为 200（SSE 响应头先于中断发送）。
- Redis 余额不变（`remaining = 10000000000`）。
- 修复前行为：按请求体长度估算输入 token（`ContentLength / 4 × 452`）+ 响应内容估算输出 token，产生多扣费。

## 补充说明

修复涉及两层保护，任一成立即可保证零扣费：

1. `ErrClientWrite` 且未收到最终 usage → `tokenRequestFinishHandler` 直接跳过扣费。
2. 响应未完成（无 `message_stop`/`[DONE]`）时 `EstimateToken` 估算不生效（`IsResponseCompleted` 守卫）。

## 清理

- 停止 BFE、关闭 mock 后端与 Redis。
