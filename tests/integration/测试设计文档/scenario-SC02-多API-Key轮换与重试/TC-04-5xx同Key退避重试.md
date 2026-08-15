# TC-04 5xx 同 Key 退避重试

## 用例编号与名称

TC-04 5xx 同 Key 退避重试

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当后端返回 5xx 或连接错误时，BFE 保持当前 Key 不变，按退避策略重试，最终成功时返回成功响应。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动。
3. 临时 BFE 配置已生成并加载。
4. `common/mock_backend.go` 已支持记录 `Authorization` 头。

## 配置构造

- `cluster_multi_key.AIConf.KeyPolicy`：
  - `MaxRetries`：3
  - `RetryBackoffInitial`：50 ms
  - `RetryBackoffMax`：200 ms
- `cluster_multi_key` 后端行为：
  - 前 2 次任意 Key 请求返回 503；
  - 第 3 次起返回 200。

实现方式：mock 后端内部计数器，当且仅当计数 ≤ 2 时返回 503。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 响应状态码：200。
- `cluster_multi_key` 后端收到 3 次请求，且 3 次 `Authorization` 头相同（均为首次选中的 Key）。
- 相邻两次请求的时间间隔 ≥ 50 ms（初始退避值，允许 jitter 容差）。
- `cluster_fallback_ok` 未被命中。
- BFE 日志中出现 `transient failure [status=503], retry same key` 相关记录。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
