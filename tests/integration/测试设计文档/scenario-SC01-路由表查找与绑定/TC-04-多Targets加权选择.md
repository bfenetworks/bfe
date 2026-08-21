# TC-04 多 Targets 加权选择

## 用例编号与名称

TC-04 多 Targets 加权选择

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证同一规则下多个 target 按 weight 加权随机选择。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动，所有 primary cluster 返回 200。
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `user_a-rule1` 的 targets 权重为：
  - `cluster_primary_a`：60
  - `cluster_primary_b`：30
  - `cluster_primary_c`：10

## BFE 请求

连续发送 1000 次：

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{}` |

## 预期结果

- 每次响应状态码均为 200
- 总命中次数 = 1000
- `cluster_primary_a` 命中次数约 600（允许 ±50）
- `cluster_primary_b` 命中次数约 300（允许 ±50）
- `cluster_primary_c` 命中次数约 100（允许 ±50）

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
