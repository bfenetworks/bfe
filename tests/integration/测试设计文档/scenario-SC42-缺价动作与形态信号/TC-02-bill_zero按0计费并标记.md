# TC-02 bill_zero 按 0 计费并标记

## 目的
验证 `bill_zero` 动作：缺价时按 0 计费（v1.8.8.2 行为），但缺价不静默。

## 步骤
1. 集群 `cluster_price_billzero`（`bill_zero`）；
2. 同 TC-01 的请求；
3. 断言余额、pb。

## 预期
- 200；后端 hits=1（仍调用上游）；
- 余额不变；
- pb：`ai_price_miss=true`、`ai_price_warn=no_entry`、`ai_cost_value=0`、`ai_zero_cost_reason=price_miss`。

对应实现：`TestTC02_BillZeroActionMarksAndChargesNothing`。
