# TC-12 questions 热加载

## 用例编号与名称

TC-12 questions 热加载

## 所属场景

SC22 AI 意图路由

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_intent`）

## 测试目的

验证 `intent_questions.data` 热加载：bump Version 并给 task_type 增加 `unit_test` 选项后，（1）已缓存意图失效——相同 prompt「单元测试」再次请求会重新调用决策服务（计数+1）；（2）新选项在新规则中可用——热更后的 `ai_route.data` 新增 `intent-unit` 规则引用 `unit_test`，请求按新配置路由到 `cluster_kimi`。

## 运行模式

单组件模式：真实 `bfe` 进程 + mock 上游后端（`cluster_flash` / `cluster_kimi`）+ mock 决策服务（测试进程内 `httptest.Server`）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 决策服务已启动，`Calls()` 计数清零；「单元测试」脚本按请求 questions 判定：含 `unit_test` 选项（热更后）→ unit_test@0.95，否则 test_writing@0.93。
3. mock 后端 `cluster_flash`、`cluster_kimi` 已启动，`Hits()` 计数清零。
4. 临时 BFE 配置已生成并加载（同 TC-01）；monitor 端口可用。
5. `ak_intent` 绑定无限额配额计划。

## 配置构造

- 初始路由表 `apikey_ak_intent`（场景说明 §4.3）：规则 1~4。
- 热更内容（步骤 2 执行）：
  - 覆写 `intent_questions.data`：Version bump 为 `2026092602`，task_type 的 `Criteria` 增加 `"unit_test": "编写单元测试、用例设计、断言补充"`；
  - 覆写 `ai_route.data`：在 intent-flash 之前新增规则 `intent-unit`（条件 `req_ai_intent_in("task_type", "unit_test", 0.9)`，targets `cluster_kimi`）。

## BFE 请求

| 步骤 | 操作 | Host | Path | Authorization | Body |
|------|------|------|------|---------------|------|
| 1 | 直接请求 | `intent.example.org` | `/v1/chat/completions` | `Bearer ak_intent` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我给这个函数写单元测试"}]}` |
| 2 | 覆写 `intent_questions.data` 与 `ai_route.data`，调用 `/reload/mod_ai_intent?path=` 与 `/reload/mod_ai_route` 触发热更 | — | — | — | — |
| 3 | 与步骤 1 相同 prompt 再次请求 | 同步骤 1 | 同步骤 1 | 同步骤 1 | 同步骤 1 |

## 预期结果

- 步骤 1：返回 200，响应体含 `from cluster_flash`；`cluster_flash` `Hits()=1`；mock 决策服务 `Calls()=1`（test_writing@0.93 → 规则 1，分类结果写入进程内缓存，缓存 key 含旧 Version）。
- 步骤 2：两个 reload 接口均返回成功；模块状态页 `QuestionsVersion=2026092602`。
- 步骤 3：返回 200，响应体含 `from cluster_kimi`；mock 决策服务 `Calls()=2`（Version bump 使缓存 key 变化，旧缓存项失效，重新分类）；mock 见请求 questions 含 `unit_test` 选项 → 应答 unit_test@0.95 → 新规则 `intent-unit`（0.95≥0.9）命中 → `cluster_kimi` `Hits()=1`，`cluster_flash` `Hits()` 保持 1 → 新选项在新规则中可用。

## 清理

停止 `bfe` 进程、mock 后端与 mock 决策服务，删除临时目录（含热更覆写的配置文件）。
