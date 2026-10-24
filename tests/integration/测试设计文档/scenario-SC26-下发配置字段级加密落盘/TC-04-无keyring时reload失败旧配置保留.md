# TC-04 无 keyring 时 reload 失败旧配置保留

## 目的

验证 reload 失败语义（版本错配危险态的保护）：BFE 以明文配置启动后，配置文件被改写
为密文但无 keyring（`[Security]` 未配置），`/reload/mod_ai_token_auth` 必须失败且
**旧生效配置保留**（可继续鉴权、不静默丢配置）。

对应实现：`TestTC04_ReloadCiphertextWithoutKeyringKeepsOldConf`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- `bfe.conf` **不配置** `[Security] KeyFile`（解密禁用态）。
- BFE 以全明文 `token_rule.data`（`ak-123`）启动成功。

## 执行步骤

1. 携带 `ak-123` 发送请求（基线鉴权通过）。
2. 测试代码将 `token_rule.data` 改写为 `ak-enc` 外层键密文形态（模拟控制面开加密、
   BFE 未配 keyring 的错配场景）。
3. 调用 `GET /reload/mod_ai_token_auth`。
4. 携带 `ak-123` 再次发送请求。
5. 携带 `ak-enc` 发送请求。

## 预期结果

- 第 3 步返回 `{"error":"...decrypt token key failed...keyring not configured..."}`，
  错误信息不含密文内容。
- 第 4 步仍返回 200：旧明文配置保留并继续生效。
- 第 5 步返回 401：新配置未生效。

## 清理

- 停止 BFE、关闭 mock 后端。
