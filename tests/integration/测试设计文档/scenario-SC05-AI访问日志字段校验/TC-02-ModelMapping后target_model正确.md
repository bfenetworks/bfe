# TC-02 ModelMapping 后 target_model 正确

## 用例编号与名称

TC-02 ModelMapping 后 target_model 正确

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.2.0`

## 测试目的

验证当请求模型经过 `AIConf.ModelMapping` 映射后，访问日志中 `ai_requested_model` 与 `ai_target_model` 分别记录原始请求模型和映射后的目标模型。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 同 TC-01，但请求 body 中 model 为 `gpt-4`。
2. `cluster_rmb.AIConf.ModelMapping` 配置 `"gpt-4" -> "deepseek-chat"`。

## 配置构造

- `cluster_rmb.AIConf.ModelMapping`：`{"gpt-4": "deepseek-chat"}`
- 其余同 TC-01。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-4"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中，后端收到的 model 为 `deepseek-chat`。
- b2log 中：
  - `ai_requested_model` = `"gpt-4"`
  - `ai_target_model` = `"deepseek-chat"`
  - `ai_cost_value` 按 `deepseek-chat` 价格计算，即 `100 * 100 + 50 * 200 = 20000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
