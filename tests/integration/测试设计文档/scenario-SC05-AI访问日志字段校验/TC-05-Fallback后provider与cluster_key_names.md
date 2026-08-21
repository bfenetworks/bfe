# TC-05 Fallback 后 provider 与 cluster_key_names

## 用例编号与名称

TC-05 Fallback 后 provider 与 cluster_key_names

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.2.0`

## 测试目的

验证当主 cluster 返回 502 触发 fallback 到另一个 cluster 后，访问日志中 `ai_provider` 记录最终 fallback cluster 的 provider，且 `ai_cluster_key_names` 包含两个 cluster 的尝试记录。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01，但存在 `cluster_fallback_rmb`。
2. `cluster_rmb` 的 mock 后端返回 502。
3. `cluster_fallback_rmb` 的 mock 后端返回 200 与 usage。
4. `ai_route.data` 中 `user_a-rmb` 的 fallbacks 包含 `cluster_fallback_rmb`。

## 配置构造

- `cluster_rmb.AIConf.Provider`：`mock-provider`
- `cluster_fallback_rmb.AIConf.Provider`：`mock-provider-fallback`
- `cluster_fallback_rmb.AIConf.ModelTable.Currency`：`RMB`
- `cluster_fallback_rmb.AIConf.ModelTable.Models[0].Prices`：
  - `input_cost_per_token`: `0.000003`
  - `output_cost_per_token`: `0.000004`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（502）。
- `cluster_fallback_rmb` 收到 1 次命中（200）。
- b2log 中：
  - `ai_provider` = `"mock-provider-fallback"`（最终成功 cluster 的 provider）
  - `ai_cost_value` 按 fallback cluster 价格计算，即 `100 * 300 + 50 * 400 = 50000`
  - `ai_cluster_key_names` 包含至少 2 条记录，分别属于 `cluster_rmb` 和 `cluster_fallback_rmb`
  - `ai_route_rule_hits` 仍包含 `user_a-rmb` 的命中记录

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
