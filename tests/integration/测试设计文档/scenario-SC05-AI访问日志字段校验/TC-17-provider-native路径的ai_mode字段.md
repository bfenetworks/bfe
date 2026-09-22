# TC-17 provider-native 路径的 ai_mode 字段（issue #1382）

## 用例编号与名称

TC-17 provider-native 路径的 ai_mode 字段（`/compatible-mode/v1/responses` 与 `/compatible-mode/v1/chat/completions`）

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：当前源码版本（含 issue #1382 修复）

## 测试目的

验证 provider-native 客户端入口（前缀以 `/v1` 段结尾的 OpenAI SDK base_url 形态）的
billing mode 识别：`/compatible-mode/v1/responses` 必须归类为 `responses`（而不是
`ModeChat` 兜底），`/compatible-mode/v1/chat/completions` 保持 `chat`。同时验证
mode 修正后价格按真实模式命中（responses 价格命中扣减，而非价格 miss 后 0 计费）。

修复前（issue #1382），`/compatible-mode/v1/responses` 被识别为 chat：访问日志
`ai_mode` 错误；模型仅配置 responses 价格时 `LookupModelPrice` miss，请求被 0 计费
（静默漏收）；两种价格都配置时按 chat 价格错计。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200、body 为
   `{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`。
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含：
   - `deepseek-chat`（chat 模式）：input `0.000001` → 100 单位，output `0.000002` → 200 单位；
   - `o3-deep-research`（responses 模式）：input `0.00001` → 1000 单位，output `0.00002` → 2000 单位。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## BFE 请求

按顺序发送 2 次 POST 请求：

| # | Host | Path | Body |
|---|------|------|------|
| 1 | `rmb.example.org` | `/compatible-mode/v1/responses` | `{"model":"o3-deep-research"}` |
| 2 | `rmb.example.org` | `/compatible-mode/v1/chat/completions` | `{"model":"deepseek-chat"}` |

## 预期结果

- 两次响应状态码均为 200，`cluster_rmb` 共 2 次命中。
- 访问日志（按请求顺序）：
  - `ai_mode[0] = responses`；
  - `ai_mode[1] = chat`。
- Redis 扣减（usage 均为 prompt 100 / completion 50）：
  - 请求 1 按 responses 价格：100×1000 + 50×2000 = 200000；
  - 请求 2 按 chat 价格：100×100 + 50×200 = 20000；
  - 合计 220000，剩余 = `10000000000 - 220000 = 99999780000`。
- 修复前请求 1 的 mode 为 chat，`o3-deep-research` 无 chat 价格 → 价格 miss，
  扣减为 0（本用例合计仅 20000）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
