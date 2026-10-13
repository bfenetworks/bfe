# TC-27 image_generation 条目按 token 计费（issue #1420）

## 用例编号与名称

TC-27 image_generation 条目按 token 计费（token 计价生图规则精确命中条目后按 token 公式扣减）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1420 修复：计费方法由命中条目的价格键存在性判别，`ModelPrice.HasPrice`）

## 测试目的

验证 issue #1420 的修复语义：`image_generation` 模式的价格条目**未配置 `output_cost_per_image`、只配置 token 单价键**（控制面"按 token 量计费"的生图规则）时，请求按 `calcChatCost` token 公式计费并正常扣减 RMB 配额。

修复前：`calcCostUnits` 仅按 mode 派发公式，`image_generation` 恒走 `calcImageGenerationCost`（`image_count × output_cost_per_image + image_input_tokens × input_cost_per_image_token`）；`output_cost_per_image` 未配置按 0 计、`image_input_tokens` 为 0 → cost=0 → RMB 扣减被 `costUnits > 0` 门槛跳过，余额不变（issue 实证：gpt-image-2 规则 200 响应 + 真实 usage，Redis `QUOTA_*` 分毫未动）。

本用例使用 issue 的实证价格与 usage 数字做精确复刻。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 返回 200、JSON body（issue 实证响应形态）：

   ```json
   {
     "data": [{"b64_json": "..."}],
     "usage": {
       "input_tokens": 19,
       "input_tokens_details": {"cached_tokens": 0, "image_tokens": 0, "text_tokens": 19},
       "output_tokens": 196,
       "output_tokens_details": {"image_tokens": 196, "text_tokens": 0},
       "total_tokens": 215
     }
   }
   ```

4. `cluster_rmb` 的 `ModelTable` 含 `(gpt-image-2, image_generation)` 条目，价格键**与 issue 定价规则一致**：`input_cost_per_token=2.402e-5`、`output_cost_per_token=0.00014414`、`cache_read_input_token_cost=6.01e-6`、`input_cost_per_image_token=3.844e-5`，**不含 `output_cost_per_image`**。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## BFE 请求

POST `http://rmb.example.org/v1/images/generations`，body `{"model":"gpt-image-2"}`（不发 `n`；Bearer key `ak_user_a`）。

## 预期结果

- HTTP 200；`cluster_rmb` 命中 1 次。
- usage 解析：`input_tokens/output_tokens` 命名由 Responses API 回退链识别（`usage.input_tokens_details` 门控），`prompt=19 / completion=196 / total=215`，`MarkFinalUsageSeen` 置位。
- 计费：条目无 `output_cost_per_image` 键 → 回落 token 公式：
  - 输入：`round(19 × 2.402e-5 × 1e8) = 45638`
  - 输出：`round(196 × 0.00014414 × 1e8) = 2825144`
  - 合计 `UsedCost = 2870782`（定点单位，≈ ¥0.0287）
- 异步扣减完成后 `quota:plan_rmb` 余额 = `10000000000 − 2870782 = 99999712928`。
- 对照（修复前行为）：同 fixture 下余额不变（cost=0），即 issue 的缺陷形态。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
