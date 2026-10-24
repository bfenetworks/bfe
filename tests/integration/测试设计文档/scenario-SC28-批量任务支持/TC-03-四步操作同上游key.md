# TC-03-四步操作同上游key

## 前置条件

见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。

## 测试步骤与断言

cluster_batch 配双 key 且 AIConf.KeyPolicy.SessionAffinity=false(排除客户端亲和)；四步操作后断言 mock 后端收到的 Authorization 头四次均为同一 key(BATCH_* 亲和绑定生效)；Redis 存在 bfe:ai:key_affinity:batch:cluster_batch:<file_id|batch_id> 绑定。

## 观测点

访问日志（common.ParseAccessLog）、miniredis 键值、mock 后端命中记录。
