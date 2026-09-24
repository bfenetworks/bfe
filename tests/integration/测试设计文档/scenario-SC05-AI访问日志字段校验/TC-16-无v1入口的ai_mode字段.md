# TC-16 无 /v1 入口的 ai_mode 字段（issue #1379 后续）

## 用例编号与名称

TC-16 无 /v1 入口的 ai_mode 字段

## 所属场景

SC05 AI 访问日志字段校验

## 版本声明

- `bfe`：含 `bfe_basic` 共享 OpenAI 端点表、DetectModeFromPath 兼容无 `/v1` 入口改写的版本（见 `docs/zh_cn/modifications/2026-09-21-ai-mode-detect-no-v1-entry/design-changes.md`）
- `bfe-access-pb`：`v0.3.3`

## 测试目的

验证 openai 标准端点**不带 `/v1` 前缀**发起请求时，`ai_mode` 与带 `/v1` 入口识别为**相同**的计费模式——计费 mode 不得依赖客户端入口是否带 `/v1`：

- `/v1/embeddings` 与 `/embeddings` 的 `ai_mode` 均为 `embedding`；
- `/images/generations`（不带 `/v1`）的 `ai_mode` 为 `image_generation`（修复前误为 `chat`，且 mode 门控的 `ImageCount` 提取被跳过）。

修复前行为：`DetectModeFromPath` 仅识别 `/v1/...` 前缀，裸端点请求一律落默认 `chat`，导致 embedding 等价差端点按 chat 价误计费、图像生成按次计数丢失。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件（修改源码后需重新编译；缓存二进制按 git 版本键控 + 源码 mtime 感知工作区改动）。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`（100 元）。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与标准 token usage（`prompt_tokens=100, completion_tokens=50`）。
4. 临时 BFE 配置已加载，`cluster_rmb` 价格表同时包含 `deepseek-chat`（mode `chat`）与 `flux-2-pro`（mode `image_generation`，`output_cost_per_image = 0.03`）。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
6. 启用 `mod_access_pb3`，b2log 输出到临时 `log/` 目录。

## 配置构造

- `cluster_rmb.AIConf`：同 TC-10 的图像生成配置（`imageGenerationAIConf`）。
- `plan_rmb`：`Unit=RMB`、`Quota=10000000000`、`RedisKey=quota:plan_rmb`。
- `ai_route.data` 中 `apikey_ak_user_a` 命中 `user_a-rmb`，target 为 `cluster_rmb`。

## BFE 请求

按顺序发送 3 次 POST 请求：

| 顺序 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `rmb.example.org` | `/v1/embeddings` | `Bearer ak_user_a` | `{"model":"deepseek-chat"}` |
| 2 | `rmb.example.org` | `/embeddings` | `Bearer ak_user_a` | `{"model":"deepseek-chat"}` |
| 3 | `rmb.example.org` | `/images/generations` | `Bearer ak_user_a` | `{"model":"flux-2-pro","n":2}` |

## 预期结果

- 三次响应状态码均为 200；`cluster_rmb` 收到 3 次命中。
- b2log 中存在 3 条 `RequestLog`（按请求顺序），`ai_mode` 依次为：
  1. `embedding`（带 `/v1` 入口，回归对照）
  2. `embedding`（不带 `/v1` 入口，本修复主场景；修复前为 `chat`）
  3. `image_generation`（不带 `/v1` 入口；修复前为 `chat`，且 `ImageCount` 不提取）
- 注：请求 1、2 的模型 `deepseek-chat` 在价格表中仅有 `chat` 档，`embedding` mode 查价未命中时不扣费（告警），不影响 `ai_mode` 断言。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
