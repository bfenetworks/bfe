# TC-04 单 target 无 fallbacks 回归

## 目的

验证单 target、无 fallbacks 的最简配置在新逻辑下行为与旧版完全一致：失败直接透出，无多余 attempt、无异常构造。

## 前置条件

- BFE 启动，路由规则 `target-fb-single`：`targets=[cluster_ta:100]`，无 fallbacks；
- mock：`cluster_ta` 返回 500 + 错误体；
- `cluster_ta` 初始命中数为 0。

## 请求构造

| 字段 | 值 |
|------|-----|
| Method | POST |
| Host | `target-fb.example.org` |
| Path | `/v1/chat/completions` |
| Header | `Authorization: Bearer ak_target_fb`、`Content-Type: application/json` |
| Body | `{"model":"gpt-4"}` |

连续发送 5 条同构请求。

## 后端响应

| cluster | 行为 |
|---------|------|
| cluster_ta | 500 + `{"error":"ta unavailable"}` |

## 执行步骤

1. 启动 BFE 与 mock 后端；
2. 连续发送 5 条请求；
3. 收集每条响应的 HTTP 状态码与错误体；
4. 读取 `cluster_ta.Hits()`。

## 预期结果

- 全部 5 条响应均为 500，错误体为 `{"error":"ta unavailable"}`（上游错误原样透出）；
- `cluster_ta.Hits() == 5`：每条请求**恰好尝试 1 次**——单 target 无可并入的兄弟 target、无 fallbacks，新逻辑不得引入任何多余 attempt。

## 清理

测试结束时关闭 BFE 与 mock 后端（`testEnv.Close()`）。
