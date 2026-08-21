# TC-07 Target 与 Fallback 模型覆盖

## 用例编号与名称

TC-07 Target 与 Fallback 模型覆盖

## 所属场景

SC01 路由表查找与绑定

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 target 与 fallback 切换时，请求体中的 `model` 字段被正确覆盖，且原始请求体内容被完整保留。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已按以下规则配置：
   - 所有 primary cluster 返回 500
   - `cluster_fallback_1` 返回 200
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `user_a-rule1` 的 targets 设置 `Model` 为 `target-model-a/b/c`。
- `user_a-rule1` 的 fallbacks 设置 `Model` 为 `fallback-model-1`。

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"origin-model","messages":[{"role":"user","content":"hello"}]}` |

## 预期结果

- 响应状态码：200
- 至少 1 个 primary backend 收到 `target-model-*`
- `cluster_fallback_1` 收到的请求体中 `model` 为 `fallback-model-1`
- `cluster_fallback_1` 收到的请求体中 `messages` 字段保持原样

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
