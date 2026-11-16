# TC-03 reject 转发前拒绝

## 目的
验证 `reject` 动作：接口未定价的请求在**转发前**被拒绝（issue 修改建议"未定价则拒绝"）。

## 步骤
1. 集群 `cluster_price_reject`（`reject`）；
2. `POST /v1/responses`（`ignore` 语义同前）；
3. 断言响应、后端 hits、余额、计数器、pb。

## 预期
- 503，body 含 `error.code=PRICE_NOT_CONFIGURED`；
- 后端 hits=0（未转发）；
- 余额不变（不计费）；
- `PriceRejected`=1；
- pb：`res_status_code=503`、`ai_price_miss=true`、`ai_price_warn=reject`（非 2xx 行也写出，方案 A）。

对应实现：`TestTC03_RejectActionRejectsBeforeForwarding`。
