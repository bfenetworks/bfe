# TC-05 embedding 故障降级

## 用例编号与名称

TC-05 embedding 故障降级

## 所属场景

SC23 AI 缓存语义缓存

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_cache` 语义缓存二期）

## 测试目的

验证 fail-open：embedding 服务故障（HTTP 500）时，请求降级为纯精确缓存流程——主请求不受影响、精确缓存读写正常、向量库零接触。

## 运行模式

单组件模式：真实 `bfe` 进程 + 内存 Redis + 进程内 embedding mock（故障模式）/ Chroma mock。

## 前置条件

1. 同 TC-01 前置条件 1-4。
2. embedding mock 开启强制故障（`SetFailAll(true)`，全部请求返回 HTTP 500），且该开关在 bfe 启动前生效。

## 配置构造

- 同 TC-01（`Semantic` 块 `topK=1`/`threshold=0.15`/`thresholdRelation=lt`，规则 `enableSemanticCache=true`）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `cache.example.org` | `/v1/chat/completions` | `Bearer ak_cache` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}` |
| 2 | 同上 | 同上 | 同上 | 同上 |

## 预期结果

- 第 1 步（embedding 故障下的 miss）：
  - 返回 200，响应体含上游答案（主请求不受影响）；
  - `backend=1`、`embedding=1`（embedding 被尝试调用一次后失败）；
  - Chroma `query=0`、`upsert=0`（**向量库零接触**——embedding 失败不再继续向量流程）；
  - 轮询确认 Redis 精确缓存键已写入（降级后精确流程完整）。
- 第 2 步（embedding 故障下的 exact hit）：返回 200，响应头 `X-Bfe-Ai-Cache: hit`；`backend=1`、`embedding=1`（精确命中不调用 embedding）；精确缓存读写不受 embedding 故障影响。

## 清理

停止 `bfe` 进程、mock 后端、embedding mock、Chroma mock 与内存 Redis，删除临时目录。
