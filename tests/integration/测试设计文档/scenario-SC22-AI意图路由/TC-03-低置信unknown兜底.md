# TC-03 低置信 unknown 兜底

## 用例编号与名称

TC-03 低置信 unknown 兜底

## 所属场景

SC22 AI 意图路由

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_intent`）

## 测试目的

验证读取时置信度门控的 unknown 兜底：test_writing@0.45 低于全局 `MinConfidence=0.6`，读取时派生为 unknown，规则 1/2/3 均不命中，走规则 4 默认路由到 `cluster_kimi`；主请求本身不受影响（返回 200）。

## 运行模式

单组件模式：真实 `bfe` 进程 + mock 上游后端（`cluster_flash` / `cluster_kimi`）+ mock 决策服务（测试进程内 `httptest.Server`）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 决策服务已启动，`Calls()` 计数清零，「很模糊」脚本返回 test_writing@0.45。
3. mock 后端 `cluster_flash`、`cluster_kimi` 已启动，`Hits()` 计数清零。
4. 临时 BFE 配置已生成并加载（同 TC-01）。
5. `ak_intent` 绑定无限额配额计划。

## 配置构造

- 路由表 `apikey_ak_intent`（场景说明 §4.3）：规则 4 `default_t()` → `cluster_kimi`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `intent.example.org` | `/v1/chat/completions` | `Bearer ak_intent` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"很模糊，帮我看看"}]}` |

## 预期结果

- 请求返回 200，响应体含 `cluster_kimi` 标识答案 → 命中规则 4。
- `cluster_flash` `Hits()=0`，`cluster_kimi` `Hits()=1`。
- mock 决策服务 `Calls()=1`（分类正常发生，是读取时门控将答案置为 unknown）。
- 模块状态页辅助断言：`ReqUnknown≥1`。

## 清理

停止 `bfe` 进程、mock 后端与 mock 决策服务，删除临时目录。
