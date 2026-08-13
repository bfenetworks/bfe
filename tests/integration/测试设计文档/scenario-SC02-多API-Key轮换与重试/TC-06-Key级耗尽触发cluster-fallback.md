# TC-06 Key 级耗尽触发 cluster fallback

## 用例编号与名称

TC-06 Key 级耗尽触发 cluster fallback

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 Key 级重试因 5xx/连接错误耗尽后，`aiClusterInvoke()` 返回 5xx/错误，外层 `ServeHTTPForAI()` 触发 cluster 级 fallback。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动。
3. 临时 BFE 配置已生成并加载。
4. `common/mock_backend.go` 已支持记录 `Authorization` 头。

## 配置构造

- `cluster_multi_key.AIConf.KeyPolicy.MaxRetries`：2。
- `cluster_multi_key` 后端行为：
  - 任意 Key 均返回 503。
- `cluster_fallback_ok` 后端行为：
  - 返回 200。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 响应状态码：200。
- `cluster_multi_key` 后端收到 ≤ 3 次请求（含同 Key 退避重试），均返回 503。
- `cluster_fallback_ok` 后端被命中 1 次，返回 200。
- BFE 日志中出现 `fallback triggered` 相关记录。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
