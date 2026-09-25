# TC-02 body model 改写与路径改写

## 用例编号与名称

TC-02 body model 改写与路径改写

## 所属场景

SC21 流量镜像（mod_traffic_mirror）

## 版本声明

- `bfe`：当前源码版本（含 `mod_traffic_mirror`）

## 测试目的

验证发布前双跑验证的核心能力：
1. `bodyRewrites` 将镜像副本的 `model` 字段改写为目标模型（生产仍走原模型）；
2. `pathRewrite` 整体替换镜像路径且保留 query string；
3. `setHeaders` 自定义头注入到镜像请求；
4. 生产请求不受任何改写影响。

## 运行模式

单组件模式：真实 `bfe` + 嵌入式 Redis + mock 后端（`cluster_primary`、`cluster_mirror`）。

## 前置条件

镜像规则：

```json
{
    "cond": "default_t()",
    "mirrorCluster": "cluster_mirror",
    "percentage": 100,
    "setHeaders": {"X-Test-Flag": "yes"},
    "bodyRewrites": [{"path": "model", "value": "shadow-v3"}],
    "pathRewrite": "/v1/internal/chat/completions"
}
```

## BFE 请求

POST `mirror.example.org/v1/chat/completions?trace=1`，Body：`{"model":"gpt-4o","messages":[{"role":"user","content":"hello mirror"}]}`

## 预期结果

- 客户端返回 200（主上游应答）。
- `cluster_primary`：`model=gpt-4o`；路径 `/v1/chat/completions`；query `trace=1`。
- `cluster_mirror`：`model=shadow-v3`（body 中 `messages` 内容不变）；路径 `/v1/internal/chat/completions`；query `trace=1` 保留；请求头 `X-Test-Flag=yes`。
- 指标：`req_labeled_total{cluster="cluster_mirror",model="gpt-4o"} = 1`（model 标签记录的是客户端请求模型，不是改写后模型）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis。
