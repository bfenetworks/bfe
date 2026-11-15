# TC-01 兄弟 target 兜底

## 目的

验证多 targets 规则中，选中 target 故障时，未选中的兄弟 target 被尝试（targets 互为兜底），而不是直接降级到 fallbacks。

## 前置条件

- BFE 启动，路由规则 `target-fb-default`：`targets=[cluster_ta:50, cluster_tb:50]`，`fallbacks=[cluster_fc]`；
- mock：`cluster_ta` 返回 500 + 错误体；`cluster_tb` 返回 200 + `{"cluster":"tb"}`；`cluster_fc` 返回 200 + `{"cluster":"fc"}`；
- 三个 cluster 初始命中数均为 0。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | POST |
| Host | `target-fb.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_target_fb`、`Content-Type: application/json` |
| Body | `{"model":"gpt-4"}`（非 batch/file 透传，body 可回放） |

连续发送 20 条同构请求。

## 后端响应

| cluster | 行为 |
|---------|------|
| cluster_ta | 500 + `{"error":"ta unavailable"}` |
| cluster_tb | 200 + `{"cluster":"tb"}` |
| cluster_fc | 200 + `{"cluster":"fc"}` |

## 执行步骤

1. 启动 BFE 与三个 mock 后端；
2. 连续发送 20 条请求；
3. 收集每条响应的 HTTP 状态码与响应体；
4. 读取三个 mock 的 `Hits()`。

## 预期结果

- 全部 20 条响应均为 200，响应体为 `{"cluster":"tb"}`（由兄弟 target 兜底，而非 fallbacks）；
- `cluster_fc.Hits() == 0`：**fallbacks 在 targets 仍有可用项时绝不触及**（新旧行为的关键区分点）；
- `cluster_tb.Hits() == 20`：每条请求恰好由 tb 服务一次（直接选中 tb 的请求 1 次上游交互；先选中 ta 的请求为 ta 失败 1 次 + tb 兜底 1 次）；
- `1 <= cluster_ta.Hits() < 20`：加权随机下两种选中结果都出现（20 条样本下概率极高），ta 的每次命中都对应一次失败后的兄弟兜底。

## 清理

测试结束时关闭 BFE 与 mock 后端（`testEnv.Close()`）。
