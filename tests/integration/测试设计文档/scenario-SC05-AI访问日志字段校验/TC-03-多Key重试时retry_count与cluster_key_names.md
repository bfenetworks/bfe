# TC-03 多 Key 重试时 retry_count 与 cluster_key_names

## 用例编号与名称

TC-03 多 Key 重试时 retry_count 与 cluster_key_names

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.2.0`

## 测试目的

验证当 `aiClusterInvoke` 内部触发 key-level 重试时，访问日志中 `ai_retry_count` 正确累加，且 `ai_cluster_key_names` 记录所有尝试过的 key。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01，但 `cluster_rmb.AIConf.KeyPolicy.MaxRetries >= 1`。
2. `cluster_rmb.AIConf.Keys` 配置两个 Key：`key-primary` 和 `key-secondary`。
3. mock 后端在首次请求时返回 500（模拟主 Key 失败），第二次请求返回 200。

## 配置构造

- `cluster_rmb.AIConf.KeyPolicy`：
  - `Strategy`: `weighted_random`
  - `MaxRetries`: `2`
  - `RetryBackoffInitial`: `10`
  - `RetryBackoffMax`: `50`
- `cluster_rmb.AIConf.Keys`：
  - `{"Name": "key-primary", "Key": "sk-primary", "Weight": 100}`
  - `{"Name": "key-secondary", "Key": "sk-secondary", "Weight": 100}`

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
- `cluster_rmb` 收到 2 次命中（第一次 500，第二次 200）。
- b2log 中：
  - `ai_retry_count` >= `1`
  - `ai_cluster_key_names` 包含至少 2 条记录，均属于 `cluster_rmb`
  - 由于 weighted random 可能两次选到同一 key，断言时只需验证 `len(ai_cluster_key_names) >= 2` 且所有记录的 `cluster_name` 均为 `cluster_rmb`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
