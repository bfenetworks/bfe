# TC-01 OpenAI错误体重写与脱敏

## 用例编号与名称

TC-01 OpenAI错误体重写与脱敏

## 所属场景

SC27 上游错误体归一

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.NormalizeUpstreamError` 实现，
  `bfe-access-pb >= v0.3.12`）

## 测试目的

验证非流式归一主路径：上游 OpenAI 错误 envelope 被识别并改写为统一错误体，
状态码按映射表重映射（上游 401 → 502 `UPSTREAM_AUTH_ERROR`），上游原始状态码/
错误码落入 `details` 与访问日志，上游消息中回显的 cluster key 被脱敏。

## 运行模式

单组件模式：真实 `bfe` + 脚本化错误 mock 后端（`cluster_err`），无 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. `cluster_err` AIConf：`Keys=[{key-primary, sk-upstream-secret}]`，
   `NormalizeUpstreamError={Enabled:true}`（其余字段缺省）。
3. `ak_front` 绑定 `apikey_ak_front`（target `cluster_err`），无限额配额计划。

## 配置构造

`cluster_conf.data` 关键片段：

```json
"AIConf": {
    "Keys": [{"Name": "key-primary", "Key": "sk-upstream-secret", "Weight": 100}],
    "NormalizeUpstreamError": {"Enabled": true}
}
```

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_front` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}` |

## 后端响应

`cluster_err` 返回 401：

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":{"message":"Incorrect API key provided: sk-upstream-secret","type":"authentication_error","code":"invalid_api_key"}}
```

## 执行步骤

1. 按上述配置启动 BFE，发送请求；
2. 解析响应状态/头/体；
3. 读取 b2log 访问日志（`accessLogs()`），找到本请求记录。

## 预期结果

- 响应状态码 **502**（`UPSTREAM_AUTH_ERROR` 映射）；
- 响应头 `X-Bfe-Gw-Error: 1`（归一重写标记）；
- 响应体为统一错误格式：
  - `error.code == "UPSTREAM_AUTH_ERROR"`、`error.type == "internal_error"`；
  - `error.details.upstream_status == 401`；
  - `error.details.upstream_code == "invalid_api_key"`；
  - `error.message` 含 `••••••••` 且**不含** `sk-upstream-secret`；
- 访问日志：`ai_err_normalized=true`、`ai_upstream_status=401`、
  `ai_upstream_err_code="invalid_api_key"`、`ai_err_normalize_miss` 未设置；
- `cluster_err` `Hits() == 1`。

## 清理

停止 `bfe` 进程与 mock 后端，删除临时目录。
