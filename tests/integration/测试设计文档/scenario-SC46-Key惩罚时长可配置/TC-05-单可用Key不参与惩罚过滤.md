# TC-05 单可用 Key 不参与惩罚过滤

## 目的

验证单可用 Key 集群的短路语义：`chooseAIKeyWithAffinity` 对 `len(keys)==1 && Weight>0` 直接返回该 Key、完全跳过惩罚过滤，因此惩罚窗口对该 Key 不生效——配置的惩罚时长在该形态下没有实际效果。同时确认写侧行为：惩罚键**仍会被写入**（写侧不判断 Key 数量），但没有读侧消费它，也不写绑定键。

这条是「运营以为配了 TTL 就有效」的反向验证。

## 前置条件

- `cluster_penalty_ttl` 仅配 1 把 Key（`key-a`，Weight 100）；
- `SessionAffinity=true`、`SessionAffinityPenaltyEnable=true`、`SessionAffinityPenaltyTTLAuth=300`、`MaxRetries=3`；
- miniredis 为空。

## 请求构造

同 TC-01。

## 后端响应

`401` + `{"error":"invalid api key"}`。

## 执行步骤

1. 启动 BFE；
2. 发送 1 条请求；
3. 统计 mock 后端命中次数与每次收到的 `Authorization`；
4. 检查 Redis 中是否存在绑定键 `bfe:ai:key_affinity:cluster_penalty_ttl:penalty_ttl_key_id`。

## 预期结果

- 请求状态码 `401`（Key 级重试耗尽后返回最后一次上游响应）；
- mock 后端命中 **4** 次（首次 + `MaxRetries=3`），且 4 次注入的都是 `key-a`——说明惩罚过滤没有生效，同一把 Key 被反复选中；
- Redis 中**不存在**绑定键（短路发生在绑定读写之前）；
- 惩罚键可能已写入（写侧不判断 Key 数量），这不影响本用例结论：没有读侧使用它。

## 清理

同 TC-01。