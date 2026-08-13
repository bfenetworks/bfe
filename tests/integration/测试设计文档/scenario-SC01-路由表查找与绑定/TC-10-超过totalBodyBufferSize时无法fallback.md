# TC-10 超过 totalBodyBufferSize 时无法 fallback

## 用例编号与名称

TC-10 超过 totalBodyBufferSize 时无法 fallback

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本
- `bfe.conf` 中 `accessibleBodySize = 4194304`

## 测试目的

验证当全局 bytes_body 缓冲区已经达到 `totalBodyBufferSize` 上限时，后续请求的 fallback 被禁用：BFE 不会为新请求分配可回绕的 body 缓冲区，primary cluster 失败后不会尝试 fallback。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：
   - `cluster_primary_a` 返回 500
   - `cluster_fallback_1` 返回 200
   - `cluster_holder` 接收请求后阻塞在读取 body 之前
3. 临时 BFE 配置已生成并加载，且 `totalBodyBufferSize` 被覆盖为 2 MB。

## 配置构造

- 使用 `user_a-large` 规则：
  - 条件：`req_host_in("large.example.org")`
  - targets：`cluster_primary_a`（`Model` 为空）
  - fallbacks：`cluster_fallback_1`（`Model` 为空）
- 使用 `user_a-holder` 规则：
  - 条件：`req_host_in("holder.example.org")`
  - targets：`cluster_holder`（`Model` 为空）
  - fallbacks：`cluster_fallback_2`（`Model` 为空，用于触发 body 包装）
- `cluster_holder` 的 `BackendConf` 与 `ClusterBasic` 超时设置为 60 s，确保在测试请求期间不会超时。

## BFE 请求

### holder 请求（占用缓冲区）

| 字段 | 值 |
|------|-----|
| Host | `holder.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Content-Type | `application/octet-stream` |
| Body | 2 MB 确定性字节序列 |

`cluster_holder` 后端在收到请求头后、读取 body 前阻塞，使 BFE 无法完成 body 写入并关闭 bytes_body 缓冲区，从而将全局 total 维持在 2 MB。

### test 请求（验证 fallback 禁用）

| 字段 | 值 |
|------|-----|
| Host | `large.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Content-Type | `application/octet-stream` |
| Body | 512 KB 确定性字节序列 |

## 预期结果

- 通过 BFE monitor 接口 `/monitor/server_stat` 观察到 `total_bytes_body_buffer` 达到 2 MB 后再发送 test 请求。
- `cluster_primary_a` 被尝试 1 次。
- `cluster_fallback_1` 命中次数为 0。
- BFE 日志中出现 `request body is not rewindable, disable fallback`。
- test 请求未通过 fallback 成功（即未返回 200）。

## 清理

关闭 holder 后端阻塞，停止 `bfe` 进程与所有 mock 后端，删除临时目录。
