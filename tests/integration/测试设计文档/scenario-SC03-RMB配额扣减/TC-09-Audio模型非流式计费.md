# TC-09 Audio 模型非流式计费

## 用例编号与名称

TC-09 Audio 模型非流式计费

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.3`

## 测试目的

验证当模型价格表中配置了音频相关单价时，BFE 对非流式响应按音频 input/output 拆分公式计费：

```
normal_input  = prompt_tokens - audio_input_tokens
normal_output = completion_tokens - audio_output_tokens
cost = normal_input * input_cost + audio_input_tokens * audio_input_cost
     + normal_output * output_cost + audio_output_tokens * audio_output_cost
```

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 4000,
           "completion_tokens": 500,
           "total_tokens": 4500,
           "audio_input_tokens": 1000,
           "audio_output_tokens": 200
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `gpt-audio-1.5`，价格：
   - `input_cost_per_token`: `0.00000178` → 定点整数 `178`
   - `output_cost_per_token`: `0.00000715` → 定点整数 `715`
   - `input_cost_per_audio_token`: `0.00002288` → 定点整数 `2288`
   - `output_cost_per_audio_token`: `0.00004576` → 定点整数 `4576`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `gpt-audio-1.5`：
  - `input_cost_per_token`: `0.00000178`
  - `output_cost_per_token`: `0.00000715`
  - `input_cost_per_audio_token`: `0.00002288`
  - `output_cost_per_audio_token`: `0.00004576`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"gpt-audio-1.5"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按音频拆分公式扣减：
  - normal_input = 4000 - 1000 = 3000
  - normal_output = 500 - 200 = 300
  - 扣减金额 = 3000 * 178 + 1000 * 2288 + 300 * 715 + 200 * 4576 = 3951700
  - 剩余 = `10000000000 - 3951700 = 99996048300`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
