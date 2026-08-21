# TC-09 body 超过 accessibleBodySize 时无法 fallback

## 用例编号与名称

TC-09 body 超过 accessibleBodySize 时无法 fallback

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本
- `bfe.conf` 中 `accessibleBodySize = 4194304`

## 测试目的

验证当请求 body 大小超过 `accessibleBodySize` 时，BFE 无法将 body 完整回绕；primary cluster 失败后，fallback 不会被执行。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：`cluster_primary_a` 返回 500，`cluster_fallback_1` 返回 200。
3. 临时 BFE 配置已生成并加载。

## 配置构造

- 使用 `user_a-large` 规则：
  - 条件：`req_host_in("large.example.org")`
  - targets：`cluster_primary_a`（`Model` 为空）
  - fallbacks：`cluster_fallback_1`（`Model` 为空）

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `large.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Content-Type | `application/octet-stream` |
| Body | 5 MB 确定性字节序列（大于 4 MB） |

## 预期结果

- `cluster_primary_a` 被尝试 1 次
- `cluster_fallback_1` 命中次数为 0
- BFE 尝试 fallback 但因 body 无法回绕而中止（日志中出现 `fallback aborted, request body cannot be rewound`）
- 客户端请求未通过 fallback 成功（即未返回 200）

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
