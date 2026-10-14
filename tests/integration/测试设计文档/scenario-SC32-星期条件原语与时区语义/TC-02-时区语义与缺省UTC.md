# TC-02 时区语义与缺省 UTC

## 用例编号与名称

TC-02 时区语义与缺省 UTC

## 所属场景

SC32 星期条件原语与时区语义

## 版本声明

- `bfe`：当前源码版本（含条件原语 `time_weekday_in`，`zone` 参数复用 `bfe_util.TimeZoneMap`，省略时按 `Z`（UTC）判定）

## 测试目的

锁定 `time_weekday_in` 的**时区语义**——这是该原语唯一会"静默判错一整天中某几个小时"的地方：

- `zone` 参与判定：同一时刻在 `"H"`（UTC+8）与省略（UTC）下可能分属**不同星期**；
- 省略 `zone` 时按 **UTC** 判定（而非宿主本地时区、也不是隐式 UTC+8）；
- 判定依据是**带时区的时刻**而非请求中的时间字符串字面量：`X-Bfe-Debug-Time` 里写的 `H` 后缀
  在 fetcher 内已被换算为 UTC 时刻，再由原语的 `zone` 换算回本地星期，两者不可混淆；
- 条件不命中时按规则顺序回落到后续规则（`weekday-utc.example.org` 的 work 规则不穷尽）。

用例取北京时间 08:00 这一**偏移边界**：北京周一 07:59:59 时 UTC 仍是周日 23:59:59，
而北京周一 08:00:00 时 UTC 恰为周一 00:00:00。两臂在这两个时刻分别「分歧」与「收敛」，
把 UTC 判定的边界钉到秒级。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：`cluster_weekday_work` / `cluster_weekday_sat` / `cluster_weekday_sun` /
   `cluster_weekday_utc_work` / `cluster_weekday_utc_default` / `cluster_weekday_default` 均返回 200，
   响应体含 cluster 名。
3. `weekday.example.org`（`"H"` 臂）与 `weekday-utc.example.org`（省略 zone 臂）均已加入
   `host_rule.data` 的 `ai_product`；`ak_user_a` 绑定 `apikey_ak_user_a`。

## 配置构造

- `"H"` 臂：`weekday.example.org` 三条规则（`{1..5}` / `{6}` / `{0}`，均带 `"H"`），互斥且穷尽；
- UTC 臂：`weekday-utc.example.org` 两条规则——`time_weekday_in("1|2|3|4|5")`（省略 zone）
  与**无条件**的 `weekday-utc-default`（用于承接未命中）；
- 详细规则见「场景说明 §4.1」。

## BFE 请求

请求：`POST http://127.0.0.1:<bfePort>/v1/chat/completions`，Header
`Authorization: Bearer ak_user_a`、`X-Bfe-Debug-Time: <下表时间>`，Host 见下表，
body `{"model":"m","messages":[]}`。

| 步骤 | `X-Bfe-Debug-Time`（`H` 后缀 = 北京时间） | 换算后的 UTC 时刻与星期 | Host | 期望命中 cluster |
|------|-------------------------------------------|-------------------------|------|------------------|
| 1 | `20190204075959H`（北京周一 07:59:59） | 2019-02-03 23:59:59 **周日** | `weekday.example.org` | `cluster_weekday_work`（`"H"` 判定为周一） |
| 2 | `20190204075959H` | 同上 | `weekday-utc.example.org` | `cluster_weekday_utc_default`（UTC 判定为周日，不在 `{1..5}` → 回落） |
| 3 | `20190204080000H`（北京周一 08:00:00） | 2019-02-04 00:00:00 **周一** | `weekday-utc.example.org` | `cluster_weekday_utc_work`（跨过 UTC 零点后命中） |
| 4 | `20190204080000H` | 同上 | `weekday.example.org` | `cluster_weekday_work`（同刻两臂一致） |

> 步骤 3 同时证明 UTC 臂并非"永不命中"：命中与否取决于判定时刻在 UTC 下的星期，
> 而不是该 Host 的规则被整体跳过。

## 预期结果

- 步骤 1/2 体现**分歧**：同一时刻、同一原语，仅因 `zone` 不同（`"H"` vs 省略）而判定为不同星期，
  路由到不同 cluster（`work` vs `utc_default`）；若步骤 2 命中 `cluster_weekday_utc_work`，
  说明省略 zone 时未按 UTC 判定（例如误用宿主本地时区或误按 UTC+8）；
- 步骤 3/4 体现**收敛**：UTC 跨过零点后两臂一致命中 `work`；
- 步骤 2 同时验证**未命中回落**：`weekday-utc-work` 不命中时按顺序落到无条件的 `weekday-utc-default`；
- 每步仅期望 cluster 命中 1 次，其余 cluster 命中数均为 0；全部步骤 HTTP 200。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。