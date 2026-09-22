# TC-20 Responses API provider-native 路径计费（issue #1382）

## 用例编号与名称

TC-20 Responses API provider-native 路径计费（`/compatible-mode/v1/responses`）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1382 修复；依赖 issue #1381 的 usage 解析修复）

## 测试目的

验证 Codex 真实请求路径 `POST /compatible-mode/v1/responses`（SSE）端到端计费：
provider-native 前缀入口必须被识别为 `responses` 模式，从而命中 Responses 模式价格
并按完整公式扣减。本用例与 TC-18（`/v1/responses` 入口，issue #1381）叠加后覆盖
Codex 完整计费链路。

修复前（issue #1382），该路径被识别为 chat 模式，`LookupModelPrice` 对仅配置
responses 价格的模型 miss，请求被 0 计费（静默漏收）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200、`Content-Type: text/event-stream`，
   body 为 SSE 序列（与 TC-18 相同）：
   ```
   data: {"type":"response.created","response":{"id":"resp_01","status":"in_progress"}}

   data: {"type":"response.output_text.delta","delta":"hello"}

   data: {"type":"response.completed","response":{"id":"resp_01","status":"completed","usage":{"input_tokens":2000,"output_tokens":1500,"total_tokens":3500,"input_tokens_details":{"cached_tokens":8000}}}}
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `gpt-5-codex`
   （mode 为 `responses`），价格同 TC-18：
   - `input_cost_per_token`: `0.000001` → 定点整数 `100`
   - `output_cost_per_token`: `0.000002` → 定点整数 `200`
   - `cache_read_input_token_cost`: `0.0000005` → 定点整数 `50`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/compatible-mode/v1/responses` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-5-codex","stream":true}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）；上游收到的路径保持
  `/compatible-mode/v1/responses` 原样（改写层对 provider-native 路径透传）。
- Redis 中 `quota:plan_rmb` 按 responses 模式价格扣减：
  - PromptTokens 归一化 = 2000 + 8000 = 10000，normal_input = 2000
  - 扣减金额 = 2000×100 + 8000×50 + 1500×200 = 900000
  - 剩余 = `10000000000 - 900000 = 99999100000`
- 修复前本用例扣减为 0（mode 误判 chat → 价格 miss → 0 计费）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
