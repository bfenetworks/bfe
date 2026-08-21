# TC-06 多 Fallbacks 全部失败

## 用例编号与名称

TC-06 多 Fallbacks 全部失败

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当所有 primary 与 fallback cluster 均失败时，BFE 返回最后一个错误响应。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已按以下规则配置：
   - 所有 primary cluster 返回 500
   - `cluster_fallback_1` 返回 500
   - `cluster_fallback_2` 返回 500
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

- 响应状态码：500
- `cluster_fallback_2` 被尝试 1 次（确认 fallback 链路已穷尽）

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
