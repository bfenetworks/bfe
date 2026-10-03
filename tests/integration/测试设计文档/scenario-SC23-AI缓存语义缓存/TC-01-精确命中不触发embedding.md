# TC-01 精确命中不触发 embedding

## 用例编号与名称

TC-01 精确命中不触发 embedding

## 所属场景

SC23 AI 缓存语义缓存

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_cache` 语义缓存二期）

## 测试目的

验证精确命中优先：首个请求 miss 时做一次语义检索并双写回（Redis + 向量库）；此后相同问题的请求走 Redis 精确命中，**不再调用 embedding 服务、不再查询向量库**——避免为精确命中支付 embedding 成本。

## 运行模式

单组件模式：真实 `bfe` 进程 + 内存 Redis（miniredis）+ 进程内 embedding mock / Chroma mock（httptest）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 内存 Redis、embedding mock（关键字→向量脚本：what is bfe→[1,0,0]）、Chroma mock（真实 cosine distance 计算）均已启动。
3. mock 后端 `cluster_cache` 返回 200 与非流式答案 `BFE is a layer-7 load balancer`。
4. `ai_route.data` 中 `ak_cache` 命中 `cluster_cache`；`ak_cache` 绑定无限额配额计划。
5. `mod_ai_cache_rule.data` 含顶层 `Semantic` 块（`topK=1`、`threshold=0.15`、`thresholdRelation=lt`）与 `enableSemanticCache=true` 规则。

## 配置构造

- 缓存规则（`Semantic` 块 + 默认规则）：

```json
{
    "Version": "1.0",
    "Semantic": { "topK": 1, "threshold": 0.15, "thresholdRelation": "lt" },
    "Config": {
        "ai_product": [
            {
                "cond": "default_t()",
                "cacheKeyStrategy": "lastQuestion",
                "cacheTTL": 3600,
                "maxBodyBytes": 1048576,
                "maxValueBytes": 1048576,
                "enableSemanticCache": true
            }
        ]
    }
}
```

## BFE 请求

发送 3 次相同 POST 请求：

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `cache.example.org` | `/v1/chat/completions` | `Bearer ak_cache` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}` |
| 2 | 同上 | 同上 | 同上 | 同上 |
| 3 | 同上 | 同上 | 同上 | 同上 |

## 预期结果

- 第 1 次请求（miss）：返回 200，响应体含上游答案；`backend=1`、`embedding=1`（语义检索一次）；轮询（最长 5s）确认 Redis 出现 `ai_cache:cache_key_id:` 前缀缓存键、Chroma `upsert=1`。
- 第 2、3 次请求（exact hit）：返回 200，响应头 `X-Bfe-Ai-Cache: hit`；`backend` 保持 1（不调上游）；`embedding` 保持 1（**精确命中零 embedding 调用**）。
- 全部完成后 Chroma `query=1`（仅第 1 次 miss 做过向量查询，精确命中零向量查询）。

## 清理

停止 `bfe` 进程、mock 后端、embedding mock、Chroma mock 与内存 Redis，删除临时目录。
