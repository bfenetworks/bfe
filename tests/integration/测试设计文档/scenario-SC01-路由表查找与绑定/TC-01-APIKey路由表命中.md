# TC-01 APIKey 路由表命中

## 用例编号与名称

TC-01 APIKey 路由表命中

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 apikey 级路由表存在匹配规则时，BFE 直接命中 apikey 表，不再继续搜索 entity/global 路由表。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动，所有 cluster 默认返回 200。
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `ai_route.data` 中 `apikey_ak_user_a` 表的 `user_a-rule1` 规则命中 `api.example.org`。
- `ApikeyRouteTableBindings` 中 `ak_user_a` 绑定 `[apikey_ak_user_a, entity_dept_ai, global_default]`。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{}` |

## 预期结果

- 响应状态码：200
- `cluster_primary_a`、`cluster_primary_b`、`cluster_primary_c` 中恰好有 1 个被命中
- `cluster_entity_default`、`cluster_global_default` 未被命中

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
