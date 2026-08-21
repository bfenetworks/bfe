# TC-08 AIConf 扩展字段加载

## 用例编号与名称

TC-08 AIConf 扩展字段加载

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 BFE 启动时能够正确解析并加载包含 `Keys`、`KeyPolicy`、`Provider`、`ModelTable` 的 `AIConf`，且 `Provider` 与 `ModelTable` 不影响转发。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动。
3. 临时 BFE 配置已生成并加载，`cluster_multi_key.AIConf` 包含完整的 `Provider` 与 `ModelTable`。

## 配置构造

- `cluster_multi_key.AIConf.Provider`：`mock-provider`。
- `cluster_multi_key.AIConf.ModelTable`：含 1 条 `ModelPrice`，`Limits`/`Prices`/`Capabilities`/`SupportedParameters` 均完整填写。
- `cluster_multi_key.AIConf.Keys`：仅保留 `key-c` weight 100，避免重试干扰。
- `cluster_multi_key.AIConf.KeyPolicy.MaxRetries`：0。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- BFE 正常启动，无配置解析异常。
- 响应状态码：200。
- `cluster_multi_key` 后端收到 1 次请求，`Authorization` 为 `Bearer sk-key-c`。
- 请求体中 `model` 被 `ModelMapping` 覆盖为 `mapped-model`（验证 `ModelMapping` 与 `ModelTable` 共存时互不影响）。
- `cluster_fallback_ok` 未被命中。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
