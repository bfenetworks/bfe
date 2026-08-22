# TC-12 ImageGeneration 模型按次计费

## 用例编号与名称

TC-12 ImageGeneration 模型按次计费

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本
- `bfe-access-pb`：`v0.3.3`

## 测试目的

验证当请求路径为 `/v1/images/generations`、模型价格表中配置了 `output_cost_per_image` 时，BFE 按实际生成图像张数计费：

```
cost = image_count * output_cost_per_image
```

其中 `image_count` 优先从响应 `usage.image_count` 读取；若响应未返回，则兜底读取请求体中的 `n` 字段（默认 1）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200 与如下 body：
   ```json
   {
       "usage": {
           "image_count": 2
       }
   }
   ```
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `flux-2-pro`：
   - `Mode`: `image_generation`
   - `output_cost_per_image`: `0.03` → 定点整数 `3000000`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `flux-2-pro`：
  - `Mode`: `image_generation`
  - `output_cost_per_image`: `0.03`

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/images/generations` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"flux-2-pro","n":2}` |

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按图像张数扣减：
  - 扣减金额 = 2 * 3000000 = 6000000
  - 剩余 = `10000000000 - 6000000 = 9999400000`

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
