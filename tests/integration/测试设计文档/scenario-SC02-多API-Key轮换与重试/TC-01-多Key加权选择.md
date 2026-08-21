# TC-01 多 Key 加权选择

## 用例编号与名称

TC-01 多 Key 加权选择

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 cluster 配置多个 API-Key 时，BFE 按 `Keys[].Weight` 进行加权随机选择。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_multi_key` 与 `cluster_fallback_ok` 已启动，默认返回 200。
3. 临时 BFE 配置已生成并加载，`cluster_multi_key` 配置 3 个 Key，weight 分别为 50/30/20。
4. `common/mock_backend.go` 已支持记录 `Authorization` 头。

## 配置构造

- `cluster_multi_key.AIConf.Keys`：
  - `key-a` weight 50
  - `key-b` weight 30
  - `key-c` weight 20
- `cluster_multi_key.AIConf.KeyPolicy.MaxRetries`：0（避免重试干扰命中分布）。

## BFE 请求

连续发送 1000 次相同请求：

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 所有请求响应状态码：200。
- `cluster_multi_key` 收到 1000 次命中，`cluster_fallback_ok` 未被命中。
- 后端记录的 `Authorization` 头中：
  - `Bearer sk-key-a` 占比约 50%（容差 ±7%）
  - `Bearer sk-key-b` 占比约 30%（容差 ±6%）
  - `Bearer sk-key-c` 占比约 20%（容差 ±5%）
- 无 429/5xx 重试日志。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
