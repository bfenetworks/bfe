# TC-01 fallback_chat 按 chat 价计费并标记

## 目的
验证缺省动作 `fallback_chat`：responses 请求无对应价格条目时按同模型 chat 价计费，且缺价可见。

## 步骤
1. 启动 BFE（静态 AIConf，集群 `cluster_price_fallback` 仅配 chat 价），Redis 预置余额；
2. `POST /v1/responses`（Host `price-fallback.example.org`，Bearer `ak_user_a`）；
3. 读余额、计数器，停 BFE 后解析 pb 日志。

## 预期
- 200；后端 hits=1；
- 余额扣减 = 100×300 + 50×900 = 75000 固定单位（chat 价）；
- `PriceFallbackUsed`=1；
- pb：`ai_price_miss=true`、`ai_price_warn=fallback`、`ai_cost_value=75000`。

对应实现：`TestTC01_FallbackChatBillsAndMarks`。
