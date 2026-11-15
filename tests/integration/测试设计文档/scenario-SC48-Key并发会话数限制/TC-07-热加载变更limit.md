# TC-07 热加载变更 limit

## 目的

验证 `MaxConcurrencyPerKey` 热加载即时生效：limit 由 1 改为 2 后，此前被拒的第三会话可成功占用槽，无需重启 BFE。

## 前置条件

- 同 TC-01 集群配置（limit=1）；
- miniredis 为空；
- 具备热加载能力（更新 `cluster_conf.data` 后触发 reload，参考 SC46 TC-04 的做法）。

## 请求构造

同 TC-01（`ak_conc_a`、`ak_conc_b`、`ak_conc_c`）。

## 执行步骤

1. 会话 A、B 各发 1 条请求 → 200，占满两把 Key；
2. 会话 C 发 1 条请求 → 429 `NO_AVAILABLE_KEY`（基线，同 TC-01）；
3. 更新 `cluster_conf.data`：`MaxConcurrencyPerKey` 1 → 2，触发热加载；
4. 会话 C 再发 1 条请求。

## 预期结果

- 步骤 4：**200**，C 成功占用某把 Key（`conc:...` 对应 ZSet 出现第二个成员 `kid-c`）；
- 热加载不改键空间与已占用成员（A、B 的槽保持）；
- 会话 A、B 各自再请求仍 200 且命中原绑定 Key（不被新 limit 挤走）。

## 清理

测试结束时关闭 BFE、mock 后端与 miniredis。
