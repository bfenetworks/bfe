# TC-05 多 Fallbacks 最终成功

## 用例编号与名称

TC-05 多 Fallbacks 最终成功

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 primary cluster 失败后，BFE 按 fallback 链路依次降级，并最终成功。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已按以下规则配置：
   - 所有 primary cluster 返回 500
   - `cluster_fallback_1` 返回 502
   - `cluster_fallback_2` 返回 200
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `user_a-rule1` 配置 3 个 primary targets 与 2 个 fallbacks。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{}` |

## 预期结果

- 响应状态码：200
- 至少 1 个 primary cluster 被尝试
- `cluster_fallback_1` 被尝试 1 次
- `cluster_fallback_2` 被尝试 1 次并返回成功响应

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
