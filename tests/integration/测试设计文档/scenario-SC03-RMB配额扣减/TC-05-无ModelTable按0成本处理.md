# TC-05 无 ModelTable 按 0 成本处理

## 用例编号与名称

TC-05 无 ModelTable 按 0 成本处理

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 RMB 配额计划命中的 cluster 未配置 `ModelTable` 时，BFE 按 0 成本处理，请求成功且 Redis 余额不变。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_no_table` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 100,
           "completion_tokens": 50,
           "total_tokens": 150
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_no_table` 的 `AIConf` 未配置 `ModelTable`。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_no_table.AIConf`：
  - 配置 `Keys` 与 `KeyPolicy`
  - 不配置 `ModelTable`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `notable.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"deepseek-chat"}` |

## 预期结果

- 响应状态码：200。
- `cluster_no_table` 收到 1 次命中。
- Redis 中 `quota:plan_rmb` 保持为 `10000000000`（余额不变）。
- BFE 日志中出现 model table not found 或 model price not found 的 Warn 日志。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
