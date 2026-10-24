# TC-10 allQuestions 策略

## 用例编号与名称

TC-10 allQuestions 策略

## 所属场景

SC20 AI 缓存精确匹配

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_cache`）

## 测试目的

验证 `cacheKeyStrategy=allQuestions`：全部 `role=user` 消息内容参与缓存键，尾问相同但对话历史不同的请求不命中（与 `lastQuestion` 策略区分）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动。
3. mock 后端 `cluster_cache` 已启动，返回 200 与非流式答案。
4. 临时 BFE 配置已生成并加载，包含 `mod_ai_cache` 规则：`cacheKeyStrategy=allQuestions`，其余同 TC-01。

## BFE 请求

| 步骤 | Authorization | Body |
|------|---------------|------|
| 1 | `Bearer ak_cache` | 对话 A：`{"model":"deepseek-chat","messages":[{"role":"user","content":"my name is tom"},{"role":"assistant","content":"nice to meet you"},{"role":"user","content":"what is my name?"}]}` |
| 2 | `Bearer ak_cache` | 对话 A（完全重复） |
| 3 | `Bearer ak_cache` | 对话 B：尾问同为 `what is my name?`，但首轮为 `my name is jerry` |

## 预期结果

- 步骤 1：miss，上游命中 1 次。
- 步骤 2：hit（完整对话一致），上游命中仍为 1 次。
- 步骤 3：miss（尾问相同但历史不同，缓存键不同），上游命中 2 次。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
