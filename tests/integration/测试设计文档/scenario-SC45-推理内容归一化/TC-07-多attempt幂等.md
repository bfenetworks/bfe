# TC-07 多 attempt 幂等

## 用例编号与名称

TC-07 多 attempt 幂等

## 所属场景

SC45 推理内容归一化

## 版本声明

- `bfe`：当前源码版本（含 `AIConf.EnsureReasoningContent` 实现）

## 测试目的

验证"按 attempt 生效 + 幂等"：主集群已把 `""` 补进共享请求体后，fallback 到另一个同样开启的集群时**不重复改写**（不出现重复键、不出现二次插入）；fallback 到未开启的集群时请求体保持已补状态且仍合法。这是 fallback/重试路径最容易踩的坑（请求体在 attempt 之间共享）。

## 运行模式

单组件模式：真实 `bfe` + 两个分别记录请求体的 mock 上游。

## 前置条件

1. 路由表使用场景内已带 fallback 的 `apikey_ak_fb`（owner `ak_fb`）：target `cluster_ds`、fallback `cluster_fb`；
2. `cluster_ds` 与 `cluster_fb` 的 `EnsureReasoningContent: ["openai"]`；
3. 两个 mock 上游分别记录请求体与命中计数；
4. fallback 由主集群返回可重试状态码触发（500），使同一个请求体被第二个集群复用。

## 配置构造

```json
"cluster_ds": {"AIConf": {"EnsureReasoningContent": ["openai"], "Keys": [...], "KeyPolicy": {"Strategy": "weighted_random", "MaxRetries": 0}}},
"cluster_fb": {"AIConf": {"EnsureReasoningContent": ["openai"], "Keys": [...], "KeyPolicy": {"Strategy": "weighted_random", "MaxRetries": 0}}}
```

## BFE 请求

| 步骤 | 臂 | 主集群响应 | Body |
|------|----|-----------|------|
| 1 | A：fallback 到开启的集群 | 500 | 场景说明 §4.4 样张 |
| 2 | B：fallback 到未开启的集群（`cluster_fb` 去掉该字段） | 500 | 同上 |

## 后端响应

- `cluster_ds` mock：500（触发 fallback），记录请求体；
- `cluster_fb` mock：200（chat 响应），记录请求体。

## 执行步骤

1. 起 BFE 与两个 mock；
2. 发送臂 A 请求；
3. 取两个 mock 记录的请求体，比对；
4. 切配置跑臂 B（`cluster_fb` 不配该字段），重复步骤 2–3。

## 预期结果

- 臂 A：
  - `cluster_ds` 收到的请求体含且仅含 1 处 `,"reasoning_content":""`；
  - `cluster_fb` 收到的请求体与 `cluster_ds` 收到的**逐字节一致**（第二次 attempt 检测到字段已存在，不再改写）；
  - 全程无重复键（`strings.Count(body, "\"reasoning_content\"") == 1`）；
  - 客户端最终收到 200；`ReasoningPatched` 只 +1（不是 +2）；
- 臂 B：`cluster_fb` 收到的请求体仍带 1 处 `reasoning_content:""`（沿用前一 attempt 的改写结果，合法且对未开启集群无害），客户端 200。

## 清理

停止 `bfe` 进程与两个 mock 上游，删除临时目录。
