# TC-06 EPP 宕机降级本地均衡

## 目的

验证单 EPP 地址（无备）且 EPP 进程消失时的降级：请求快速失败（短连接超时）并回退本地均衡，服务不中断。

对应实现：`TestTC06_EppDownFallback`。

## 前置条件

- 单 mock EPP；`EPPTimeout`：`Connect=200ms`、`Call=1s`。
- 启动后关闭 mock EPP（监听端口关闭，连接被拒绝）。

## 执行步骤

1. 发送请求（标记串 `hello-epp-tc06`）。
2. 断言响应成功且由本地后端服务。
3. 断言 `epp_fallback_local_total >= 1`。

## 预期结果

- 响应 200，body 含 `hello-epp-tc06`。
- `epp_fallback_local_total{cluster="cluster_epp_sim"} >= 1`。

## 清理

- 同 TC-01。
