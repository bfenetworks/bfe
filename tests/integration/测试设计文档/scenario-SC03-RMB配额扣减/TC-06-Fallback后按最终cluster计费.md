# TC-06 Fallback 后按最终 cluster 计费

## 用例编号与名称

TC-06 Fallback 后按最终 cluster 计费

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当请求触发 cluster 级 fallback 后，BFE 按最终命中的 cluster 的 `ModelTable` 价格计费。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 502（触发 fallback）。
4. mock 后端 `cluster_fallback_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 100,
           "completion_tokens": 50,
           "total_tokens": 150
       }
   }
   ```
5. 临时 BFE 配置已加载：
   - `cluster_rmb` 的 `ModelTable` 价格：`input=0.000001`, `output=0.000002`
   - `cluster_fallback_rmb` 的 `ModelTable` 价格：`input=0.000003`, `output=0.000004`
6. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。
7. 路由表配置 `cluster_rmb` 的 fallbacks 为 `cluster_fallback_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models[0].Prices`：
  - `input_cost_per_token`: `0.000001`
  - `output_cost_per_token`: `0.000002`
- `cluster_fallback_rmb.AIConf.ModelTable.Models[0].Prices`：
  - `input_cost_per_token`: `0.000003`
  - `output_cost_per_token`: `0.000004`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（502）。
- `cluster_fallback_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额扣减按 `cluster_fallback_rmb` 价格计算：
  - 扣减金额 = `100 * 300 + 50 * 400 = 50000`（0.0005 元）
  - 剩余 = `9999995000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
