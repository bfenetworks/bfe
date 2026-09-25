# TC-08 SSE 读空与 usage/finish_reason 解析

## 用例编号与名称

TC-08 SSE 读空与 usage/finish_reason 解析

## 所属场景

SC21 流量镜像（mod_traffic_mirror）

## 测试目的

验证镜像侧对 SSE 流式响应完整读空至 `data: [DONE]`，并在读空过程中解析出 `usage`（prompt/completion tokens）与 `finish_reason`，进入对应 Prometheus 指标。半途断开会使镜像集群取消推理、统计不到 usage，因此"完整读空"是 AI 镜像区别于通用 L7 镜像的关键行为。

## 运行模式

单组件模式。

## 前置条件

1. `cluster_primary`：非流式 JSON 200（主链路简单应答）。
2. `cluster_mirror`：SSE 后端，按真实分帧 flush 4 个事件：
   - chunk1：`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`
   - chunk2：`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`
   - chunk3：`{"choices":[{"index":0,"delta":{"content":"!"}}],"finish_reason":null}` → 实际写入 `finish_reason":"stop"` 与 usage 的最终 chunk：`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}}`
   - chunk4：`[DONE]`
3. 镜像规则：`cond=default_t()`、`mirrorCluster=cluster_mirror`、`percentage=100`。

## BFE 请求

POST `mirror.example.org/v1/chat/completions`，Body 带 `"stream":true`。

## 预期结果

- 客户端返回 200（主上游非流式应答）。
- `cluster_mirror` 命中 1 次；镜像连接被完整读空（mock 服务端写 `[DONE]` 后正常结束，无异常断流）。
- 指标（轮询等待）：
  - `resp_status_total{cluster="cluster_mirror",status="200"} = 1`
  - `finish_reason_total{cluster="cluster_mirror",reason="stop"} = 1`
  - `tokens_total{cluster="cluster_mirror",kind="prompt"} = 7`
  - `tokens_total{cluster="cluster_mirror",kind="completion"} = 5`
  - `fail_total{cluster="cluster_mirror"} = 0`（读空不算失败，不触发熔断）

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis。
