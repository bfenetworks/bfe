# TC-13-failover-404摘除

## 前置条件

见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。

## 测试步骤与断言

mock provider 对 Authorization=key-a 的 GET /v1/batches/{id} 返回 404，key-b 返回 200。生命周期到创建（四步绑定 key-a 链）→ get#1 命中 key-a → 404，BFE 删 batch 绑定并 60s 惩罚 key-a → get#2 经惩罚过滤确定性落 key-b → 200。

## 观测点

访问日志（common.ParseAccessLog）、miniredis 键值、mock 后端命中记录。
