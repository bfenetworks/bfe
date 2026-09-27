# TC-04 coding 意图走默认

## 用例编号与名称

TC-04 coding 意图走默认

## 所属场景

SC22 AI 意图路由

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_intent`）

## 测试目的

验证无对应意图规则时的默认路由：prompt「重构代码」被判为 coding@0.90，路由表中无 coding 专属规则，规则 1/2/3 不命中，走规则 4 默认路由到 `cluster_kimi`。

## 运行模式

单组件模式：真实 `bfe` 进程 + mock 上游后端（`cluster_flash` / `cluster_kimi`）+ mock 决策服务（测试进程内 `httptest.Server`）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 决策服务已启动，`Calls()` 计数清零，「重构代码」脚本返回 coding@0.90。
3. mock 后端 `cluster_flash`、`cluster_kimi` 已启动，`Hits()` 计数清零。
4. 临时 BFE 配置已生成并加载（同 TC-01）。
5. `ak_intent` 绑定无限额配额计划。

## 配置构造

- 路由表 `apikey_ak_intent`（场景说明 §4.3）：无 coding 相关规则，规则 4 `default_t()` → `cluster_kimi`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `intent.example.org` | `/v1/chat/completions` | `Bearer ak_intent` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"帮我重构代码，提取公共函数"}]}` |

## 预期结果

- 请求返回 200，响应体含 `cluster_kimi` 标识答案 → 命中规则 4。
- `cluster_flash` `Hits()=0`，`cluster_kimi` `Hits()=1`。
- mock 决策服务 `Calls()=1`（coding 是合法选项、置信度 0.90≥0.6 门控通过，仅因无匹配规则而落默认路由，并非 unknown）。

## 清理

停止 `bfe` 进程、mock 后端与 mock 决策服务，删除临时目录。
