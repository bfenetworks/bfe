# TC-35 按次计费无 usage 对象按 data 数回退计费

## 用例编号与名称

TC-35 按次计费无 usage 对象按 data 数回退计费（issue #1498）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1498 修复）

## 测试目的

验证按次计费模式（image / video generation）上游响应**整体省略 `usage` 对象**时，计费按响应 `data.#` 实际个数回退（`#video-generations` / #1420 批准条款），并锁定该回退**不依赖 `EstimateToken` 开关**：修复前 EstimateToken=true 按鉴权期请求预置 `n` 计费（`data.# < n` 时多扣）、EstimateToken=false unrecognized 零扣（漏计费），两种形态都未按 `data.#` 回退。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动；`video_generation` 条目配置 `output_cost_per_video = 0.5`（`50000000` 定点/视频）。
4. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- 价格条目：Model `kling-video`，`Mode: video_generation`，`Prices` 含 `output_cost_per_video: 0.5`。
- 两形态各起一次环境：
  - 形态 A：`EstimateToken=true`（bfe.conf 模板缺省即 true）；
  - 形态 B：生成配置写入 `EstimateToken = false`。

## BFE 请求

两形态各发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/video/generations` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"kling-video","n":3}` |

mock 后端返回**无 `usage` 对象、`data` 恰 2 项**的 JSON：`{"data":[{"url":"u1"},{"url":"u2"}],"created":123}`。

## 预期结果

- 响应状态码：200，`cluster_rmb` 各命中 1 次；客户端收到上游原样 body。
- 两种形态 Redis `quota:plan_rmb` 均扣减 **`2 × 50000000 = 100000000`** 定点：
  - 形态 A 修复前为 `3 × 50000000 = 150000000`（按请求预置 `n` 多扣）——本用例精确断言排除；
  - 形态 B 修复前余额不变（unrecognized 零扣）——本用例精确断言排除。
- 访问日志断言（`ai_video_count=2`、`ai_image_count=0`、`ai_usage_source=parsed`）由单元测试 `TestUpdateCtxByUsage_PerCountDataFallback` / `TestTokenRequestFinishHandler_PerCountDataFallbackParsed` 覆盖——SC03 场景无 pb3 日志解析设施，不在本 TC 重复。
- 对照臂：响应带 `usage.video_count=3`（`data.#=2`）时按 `3 × 50000000` 计费——usage 字段优先于 `data.#`，不回归。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
