# TC-07 fallback 重试去重

## 用例编号与名称

TC-07 fallback 重试去重

## 所属场景

SC21 流量镜像（mod_traffic_mirror）

## 测试目的

验证 AI fallback 重试时 `HandleForward` 被多次触发（每次 clusterInvoke 一次），但同一客户端请求只镜像一次（`CtxMirrored` once 标记），避免镜像集群 QPS 虚高、成本翻倍。

## 运行模式

单组件模式。

## 前置条件

1. `ai_route.data`：`ak_mirror` → targets `[cluster_primary]`，fallbacks `[cluster_fallback]`。
2. `cluster_primary`：第 1 次命中返回 503，后续返回 200（本用例仅 1 次命中）。
3. `cluster_fallback`：返回 200。
4. 镜像规则：`cond=default_t()`、`mirrorCluster=cluster_mirror`、`percentage=100`。

## BFE 请求

POST `mirror.example.org/v1/chat/completions` 1 次。

## 预期结果

- 客户端最终返回 200（fallback 应答）。
- `cluster_primary` 命中 1 次（503）；`cluster_fallback` 命中 1 次（200）。
- `cluster_mirror` **恰好命中 1 次**（关键断言：不是 2 次）。
- 指标：`req_labeled_total{cluster="cluster_mirror"} = 1`。
- `cluster_mirror` 收到的 body 与客户端请求一致。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis。
