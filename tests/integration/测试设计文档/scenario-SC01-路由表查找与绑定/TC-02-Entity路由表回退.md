# TC-02 Entity 路由表回退

## 用例编号与名称

TC-02 Entity 路由表回退

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 apikey 级路由表无匹配规则时，BFE 按 `ApikeyRouteTableBindings` 绑定顺序继续搜索，并命中 entity 级默认规则。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动，所有 cluster 默认返回 200。
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `ai_route.data` 中 `apikey_ak_user_a` 表的规则仅命中 `api.example.org`。
- `entity_dept_ai` 表的 `dept_ai-default` 规则使用 `default_t()`，命中 `cluster_entity_default`。
- `ApikeyRouteTableBindings` 中 `ak_user_a` 绑定 `[apikey_ak_user_a, entity_dept_ai, global_default]`。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `other.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{}` |

## 预期结果

- 响应状态码：200
- `cluster_entity_default` 被命中 1 次
- `cluster_primary_a/b/c`、`cluster_global_default` 未被命中

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
