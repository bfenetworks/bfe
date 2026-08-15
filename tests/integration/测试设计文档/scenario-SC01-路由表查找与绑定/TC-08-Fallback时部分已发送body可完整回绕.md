# TC-08 Fallback 时部分已发送 body 可完整回绕

## 用例编号与名称

TC-08 Fallback 时部分已发送 body 可完整回绕

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本
- `bfe.conf` 中 `accessibleBodySize = 4194304`

## 测试目的

验证当请求 body 已经被部分发送给 primary cluster、且 primary 连接中途关闭时，BFE 仍能将完整 body 回绕并发送给 fallback cluster。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动；`cluster_fallback_1` 返回 200。
3. `cluster_primary_a` 被配置为读取 1 KB 请求体后强制关闭连接。
4. 临时 BFE 配置已生成并加载。

## 配置构造

- `ai_route.data` 中新增 `user_a-large` 规则：
  - 条件：`req_host_in("large.example.org")`
  - targets：`cluster_primary_a`（`Model` 为空）
  - fallbacks：`cluster_fallback_1`（`Model` 为空）
- `server_data_conf/host_rule.data` 增加 `large.example.org`。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `large.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Content-Type | `application/octet-stream` |
| Body | 1 MB 确定性字节序列 |

## 预期结果

- `cluster_primary_a` 被尝试 1 次（读取 1 KB 后关闭连接）
- `cluster_fallback_1` 被命中 1 次
- `cluster_fallback_1` 收到的请求体长度等于 1 MB
- `cluster_fallback_1` 收到的请求体内容与发送内容完全一致

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
