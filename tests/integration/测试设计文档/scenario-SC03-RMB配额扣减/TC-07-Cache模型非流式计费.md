# TC-07 Cache 模型非流式计费

## 用例编号与名称

TC-07 Cache 模型非流式计费

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当模型价格表中配置了 cache 相关单价时，BFE 对非流式响应按 cache 拆分公式计费：

```
normal_input = prompt_tokens - cache_read_tokens
cost = normal_input * input_cost + cache_read_tokens * cache_read_cost
     + cache_write_tokens * cache_creation_cost + completion_tokens * output_cost
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
           "prompt_tokens": 8000,
           "completion_tokens": 1500,
           "total_tokens": 9500,
           "cache_read_tokens": 5000,
           "cache_write_tokens": 1000
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `claude-opus-4-6`，价格：
   - `input_cost_per_token`: `0.00000452` → 定点整数 `452`
   - `output_cost_per_token`: `0.00002262` → 定点整数 `2262`
   - `cache_read_input_token_cost`: `0.00000045` → 定点整数 `45`
   - `cache_creation_input_token_cost`: `0.00000565` → 定点整数 `565`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `claude-opus-4-6`：
  - `input_cost_per_token`: `0.00000452`
  - `output_cost_per_token`: `0.00002262`
  - `cache_read_input_token_cost`: `0.00000045`
  - `cache_creation_input_token_cost`: `0.00000565`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"claude-opus-4-6"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按 cache 拆分公式扣减：
  - normal_input = 8000 - 5000 = 3000
  - 扣减金额 = 3000 * 452 + 5000 * 45 + 1000 * 565 + 1500 * 2262 = 5539000
  - 剩余 = `10000000000 - 5539000 = 99994461000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
