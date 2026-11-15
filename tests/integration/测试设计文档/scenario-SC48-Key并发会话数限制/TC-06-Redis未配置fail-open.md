# TC-06 Redis 未配置 fail-open

## 目的

验证 `MaxConcurrencyPerKey > 0` 但 `bfe.conf` 未启用 `[AIKeyAffinity]` Redis（客户端为 nil）时限制**静默不生效**（fail-open）：多会话并发请求全部成功，行为等价于不限并发；不创建任何 `conc:` 键。

## 前置条件

- `bfe.conf` **不配置** `[AIKeyAffinity]` 段（或 `Disabled=true`）；
- `cluster_key_conc` 仍配 `MaxConcurrencyPerKey=1`（2 把 Key）；
- 不启动 miniredis（或启动但 BFE 不指向它）。

## 请求构造

同 TC-01（`ak_conc_a`、`ak_conc_b`、`ak_conc_c`）。

## 执行步骤

1. 会话 A、B、C 各发 1 条请求（顺序或并发均可）；
2. 观察三条请求的响应与会话绑定。

## 预期结果

- 三条请求全部 **200**；
- 会话正常加权随机/（亲和关闭时纯随机）绑定 Key，无 429；
- 无任何 Redis 写入（若启动了 miniredis，断言其中不存在 `bfe:ai:key_affinity:conc:*` 键）；
- BFE 日志出现 `aiKeyConcurrency` 相关 Warn 或不出现均可（实现为静默跳过，不强制断言日志）。

## 清理

测试结束时关闭 BFE、mock 后端（与 miniredis）。
