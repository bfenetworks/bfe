# TC-02 上游429归一与Retry-After

## 用例编号与名称

TC-02 上游429归一与Retry-After

## 所属场景

SC27 上游错误体归一

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.NormalizeUpstreamError` 实现）

## 测试目的

验证上游 429 的归一映射：`rate_limit_exceeded` → `UPSTREAM_RATE_LIMITED`、
`insufficient_quota` → `UPSTREAM_QUOTA_EXHAUSTED`（状态码保持 429），上游
`Retry-After` 头解析进 `details.retry_after_seconds`。

## 运行模式

单组件模式：真实 `bfe` + 脚本化错误 mock 后端，无 Redis。

## 前置条件

1. `cluster_err` AIConf：`NormalizeUpstreamError={Enabled:true}`（其余缺省）。
2. `ak_front` 无限额配额计划。

## 配置构造

同 TC-01。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_front` | 触发脚本 A（rate_limit_exceeded）的请求体 |
| 2 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_front` | 触发脚本 B（insufficient_quota）的请求体 |

> 实现提示：脚本化后端按请求体内容关键字区分脚本（如 user content 含
> "quota" 走脚本 B），避免序号状态。

## 后端响应

脚本 A：

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 30
Content-Type: application/json

{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}
```

脚本 B：

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json

{"error":{"message":"Balance is zero","type":"rate_limit_error","code":"insufficient_quota"}}
```

## 执行步骤

1. 启动 BFE，依次发送两个请求；
2. 分别解析响应体与日志记录。

## 预期结果

- 步骤 1：状态码 **429**，`error.code == "UPSTREAM_RATE_LIMITED"`、
  `error.type == "rate_limit_error"`、
  `error.details.retry_after_seconds == 30`；
- 步骤 2：状态码 **429**，`error.code == "UPSTREAM_QUOTA_EXHAUSTED"`、
  `error.type == "quota_error"`、`error.details` 无 `retry_after_seconds`；
- 两条日志均 `ai_err_normalized=true`、`ai_upstream_status=429`。

## 清理

停止 `bfe` 进程与 mock 后端，删除临时目录。
