# TC-05 Key 耗尽后返回最后响应

## 用例编号与名称

TC-05 Key 耗尽后返回最后响应

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当所有 API-Key 均因 429/401/403 被排除后，`aiClusterInvoke()` 返回最后一个获得的 4xx 响应，不触发 cluster 级 fallback。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动。
3. 临时 BFE 配置已生成并加载。
4. `common/mock_backend.go` 已支持记录 `Authorization` 头。

## 配置构造

- `cluster_multi_key.AIConf.KeyPolicy.MaxRetries`：3（Key 共 3 个，预算足够尝试所有 Key）。
- `cluster_multi_key` 后端行为：
  - `sk-key-a` 返回 429；
  - `sk-key-b` 返回 401；
  - `sk-key-c` 返回 403。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 响应状态码：429/401/403 之一（最后一次尝试的 Key 决定）。
- `cluster_multi_key` 后端收到 3~4 次请求，分别携带 `sk-key-a`、`sk-key-b`、`sk-key-c`；当 429 Key 被重置后可能再尝试一次。
- `cluster_fallback_ok` 未被命中。
- BFE 日志中出现 `all ai keys exhausted` 相关记录。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
