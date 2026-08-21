# TC-07 请求体在 Key 轮换中完整回绕

## 用例编号与名称

TC-07 请求体在 Key 轮换中完整回绕

## 所属场景

SC02 多 API-Key 轮换与重试

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 Key 级轮换时，请求体能够回绕到起始位置，每次尝试的后端都能收到完整、一致的请求体。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动。
3. 临时 BFE 配置已生成并加载。
4. `common/mock_backend.go` 已支持记录请求体与 `Authorization` 头。

## 配置构造

- `cluster_multi_key.AIConf.KeyPolicy.MaxRetries`：3。
- `cluster_multi_key` 后端行为：
  - 收到 `sk-key-a` 时返回 429；
  - 收到 `sk-key-b`/`sk-key-c` 时返回 200。
- 请求体大小：约 100 KB 的 JSON，包含固定字段与随机内容，确保 `Content-Length` 大于 0。

## BFE 请求

连续发送 100 次相同请求：

| 字段 | 值 |
|------|-----|
| Host | `multikey.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | 约 100 KB JSON，`{"model":"gpt-4","content":"..."}` |

## 预期结果

- 所有请求响应状态码：200。
- `cluster_multi_key` 后端总命中次数大于 100（存在因 429 触发的 Key 轮换）。
- 所有记录的请求体字节完全一致，且与客户端发送字节完全一致。
- `cluster_fallback_ok` 未被命中。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
