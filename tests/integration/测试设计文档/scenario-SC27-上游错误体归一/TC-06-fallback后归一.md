# TC-06 fallback后归一

## 用例编号与名称

TC-06 fallback后归一

## 所属场景

SC27 上游错误体归一

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.NormalizeUpstreamError` 实现）

## 测试目的

验证归一与容错的时序边界：归一仅在 fallback 循环结束、最终结果确定后介入，
**不影响** cluster fallback 决策（主 cluster 500 仍触发 fallback），最终结果
（fallback cluster 的 429）被归一。

## 运行模式

单组件模式：真实 `bfe` + 两个脚本化错误 mock 后端（`cluster_err` /
`cluster_fb`），无 Redis。

## 前置条件

1. `cluster_err`：返回 500（`{"error":{"message":"boom","type":"server_error"}}`），
   AIConf `NormalizeUpstreamError={Enabled:true}`；
2. `cluster_fb`：返回 429（`rate_limit_exceeded`），**不开归一**
   （验证归一决策取"最终响应所在集群"的配置）；
3. `ak_fb` 绑定 `apikey_ak_fb`：target `cluster_err`、fallback `cluster_fb`。

> 说明：`NormalizeUpstreamError` 从 `lastCluster`（最终响应所在集群）取配置——
> 本用例 `cluster_fb` 未开归一，预期**不归一**（逐字节透传 429），以此锁定
> 配置归属；`cluster_err` 开归一只为验证其对中间尝试零影响（ Hits 断言）。

## 配置构造

- `cluster_err` AIConf：`NormalizeUpstreamError={Enabled:true}`；
- `cluster_fb` AIConf：仅 `Keys`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `front.example.org` | `/v1/chat/completions` | `Bearer ak_fb` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}` |

## 后端响应

- `cluster_err`：`500 {"error":{"message":"boom","type":"server_error"}}`；
- `cluster_fb`：`429 {"error":{"message":"slow","type":"rate_limit_error","code":"rate_limit_exceeded"}}`。

## 执行步骤

1. 启动 BFE，发送请求；
2. 解析响应与两后端 `Hits()`；
3. 检查日志记录。

## 预期结果

- `cluster_err` `Hits() == 1`（主 cluster 被尝试，500 触发 fallback——
  归一未干预中间尝试的容错决策）；
- `cluster_fb` `Hits() == 1`；
- 最终响应 **429** 且 body 为 `cluster_fb` 原始 envelope **逐字节透传**
  （配置归属：最终集群未开归一，故不重写）；
- 访问日志：`ai_err_normalized` 未设置、`ai_upstream_status=429`。

> 镜像对照：若将 `cluster_fb` 也开启 `Enabled:true`，同一请求最终应返回
> 归一的 429 `UPSTREAM_RATE_LIMITED`——本用例实现时可加为该 TC 的步骤 2
> （重启 BFE 换配置后重发），一次性覆盖两种归属。

## 清理

停止 `bfe` 进程与 mock 后端，删除临时目录。
