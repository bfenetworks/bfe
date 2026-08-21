# TC-04 Token 与 RMB 配额共存

## 用例编号与名称

TC-04 Token 与 RMB 配额共存

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证当 API Key 同时绑定 Token 配额计划和 RMB 配额计划时，BFE 分别按各自单位扣减，互不干扰。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置：
   - `quota:plan_token = 1000`
   - `quota:plan_rmb = 10000000000`
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "prompt_tokens": 100,
           "completion_tokens": 50,
           "total_tokens": 150
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 配置 `ModelTable`。
5. `ak_user_a` 同时绑定 `plan_token` 和 `plan_rmb`。

## 配置构造

- `plan_token`：
  - `Unit`: `total_token`
  - `Quota`: `1000`
  - `RedisKey`: `quota:plan_token`
- `plan_rmb`：
  - `Unit`: `RMB`
  - `Quota`: `10000000000`
  - `RedisKey`: `quota:plan_rmb`

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
- `cluster_rmb` 收到 1 次命中。
- Redis 余额变化：
  - `quota:plan_token` = `1000 - 150 = 850`
  - `quota:plan_rmb` = `10000000000 - (100 * 100 + 50 * 200) = 9999998000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
