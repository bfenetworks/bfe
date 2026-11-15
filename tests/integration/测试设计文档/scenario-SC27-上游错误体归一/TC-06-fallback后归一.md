# TC-06 fallback后归一

## 用例编号与名称

TC-06 fallback后归一

## 所属场景

SC27 上游错误体归一

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.NormalizeUpstreamError` 实现；含 2026-11-15
  "降级耗尽透出主选错误"变更，见
  `docs/zh_cn/modifications/2026-11-15-ai-route-fallback-primary-error/`）

## 测试目的

验证归一与容错的时序边界：归一仅在 fallback 循环结束、最终结果确定后介入，
**不影响** cluster fallback 决策（主 cluster 500 仍触发 fallback）。

2026-11-15 行为变更后，全链失败时透出**主选 cluster** 的错误（fallback cluster
的响应被尝试但不再到达客户端），归一配置取**主选 cluster**（最终响应所在集群）
的配置。本 TC 分两步锁定该归属：

- 步骤 1：主选 cluster 未开归一 → 其 500 错误体逐字节透传（即使 fallback
  cluster 开了归一也不生效）；
- 步骤 2：主选 cluster 开归一 → 其 500 被重写为统一错误目录
  （`MODEL_INTERNAL_ERROR`，status 保持 500），fallback cluster 配置无关。

## 运行模式

单组件模式：真实 `bfe` + 两个脚本化错误 mock 后端（`cluster_err` /
`cluster_fb`），无 Redis。

## 前置条件

1. `cluster_err`：返回 500（`{"error":{"message":"boom","type":"server_error"}}`）；
2. `cluster_fb`：返回 429（`rate_limit_exceeded`）；
3. `ak_fb` 绑定 `apikey_ak_fb`：target `cluster_err`、fallback `cluster_fb`。

> 说明：`NormalizeUpstreamError` 从最终响应所在集群取配置——行为变更后最终
> 响应恒为主选 cluster（`cluster_err`）的，故本 TC 以 `cluster_err` 的开关
> 锁定配置归属；`cluster_fb` 是否开归一对透出结果无影响。

## 配置构造

- 步骤 1：`cluster_err` 仅 `Keys`（不开归一）；`cluster_fb` 开
  `NormalizeUpstreamError={Enabled:true}`；
- 步骤 2：`cluster_err` 开 `NormalizeUpstreamError={Enabled:true}`；
  `cluster_fb` 仅 `Keys`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1/2 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_fb` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}` |

## 后端响应

- `cluster_err`：`500 {"error":{"message":"boom","type":"server_error"}}`；
- `cluster_fb`：`429 {"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`。

## 执行步骤

1. 启动 BFE，发送请求；
2. 解析响应与两后端 `Hits()`；
3. 检查日志记录。

## 预期结果

- 两步骤均有 `cluster_err` `Hits() == 1`、`cluster_fb` `Hits() == 1`
  （主 cluster 被尝试，500 触发 fallback——归一未干预中间尝试的容错决策）；
- 步骤 1：最终响应 **500** 且 body 为 `cluster_err` 原始 envelope **逐字节透传**
  （最终集群未开归一，故不重写）；
- 步骤 2：最终响应 **500**，body 为统一错误目录
  `code=MODEL_INTERNAL_ERROR`、`details.upstream_status=500`；
- 访问日志：`ai_upstream_status=500`；步骤 2 设置 `ai_err_normalized`。

## 清理

停止 `bfe` 进程与 mock 后端，删除临时目录。
