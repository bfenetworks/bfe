# TC-09 全明文无 keyring 正常启动

## 目的

验证渐进启用/回滚基线：`bfe.conf` 未配置 `[Security]` 且全部配置文件为明文时，BFE
行为与改造前完全一致（基线兼容、回滚态安全）。

对应实现：`TestTC09_AllPlaintextNoKeyringBaseline`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- `bfe.conf` 不配置 `[Security] KeyFile`。
- 全部配置文件为模板明文形态。

## 执行步骤

1. 启动 BFE。
2. 携带 `ak-123` 发送请求。
3. 调用 `GET /reload/mod_ai_token_auth` 与 `GET /reload/server_data_conf`。

## 预期结果

- 启动成功；第 2 步返回 200。
- 第 3 步两个 reload 均返回 `{"error":null}`（明文直通，keyring 缺失无影响）。

## 清理

- 停止 BFE、关闭 mock 后端。
