# TC-04 前缀裁剪后再 ModelMapping

## 用例编号与名称

TC-04 前缀裁剪后再 ModelMapping

## 所属场景

SC04 Provider/Model 前缀裁剪

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证前缀裁剪后，BFE 继续执行 `ModelMapping`，下游最终收到映射后的模型名。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：
   - `cluster_openrouter` 返回 200
3. 临时 BFE 配置已生成并加载。

## 配置构造

- `apikey_ak_user_a` 路由表中 `user_a-openrouter` 规则：
  - 条件：`req_body_json_prefix_in("model", "openrouter/", false)`
  - targets：`cluster_openrouter`
- `cluster_openrouter` 的 `AIConf`：
  - `MatchPrefix`：`openrouter/`
  - `StripPrefix`：`true`
  - `ModelMapping`：`{"anthropic/claude-sonnet-4.6": "claude-3-sonnet-20250219"}`

## BFE 请求

| 字段 | 值 |
|------|-----|
| Host | `api.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"openrouter/anthropic/claude-sonnet-4.6","messages":[{"role":"user","content":"hello"}]}` |

## 预期结果

- 响应状态码：200
- `cluster_openrouter` 收到 1 次请求
- `cluster_openrouter` 收到的请求体中 `model` 为 `claude-3-sonnet-20250219`

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
