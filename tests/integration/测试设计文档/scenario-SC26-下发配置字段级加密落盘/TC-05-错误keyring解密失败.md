# TC-05 错误 keyring 解密失败

## 目的

验证解密失败语义：keyring 配置的密钥与加密时不一致（密钥轮换中误删旧钥、keyring
分发错误）或密文 keyID 在 keyring 中不存在时，GCM 认证失败/未知 keyID，加载失败且
错误信息不含密文。

对应实现：`TestTC05_WrongKeyringDecryptFails`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- `token_rule.data` 外层键用 keyA（keyID=1）加密。

## 执行步骤

1. **密钥不符**：keyring 配置 keyB（keyID=1，材质不同），启动 BFE。
2. **未知 keyID**：keyring 仅含 keyID=2 的 keyA，启动 BFE。
3. 对每次启动失败，收集 BFE 启动日志/返回错误。

## 预期结果

- 第 1 步启动失败：模块 Init 报错（`decrypt failed` / GCM 认证失败），BFE 拒绝启动
  （fail-fast）。
- 第 2 步启动失败：`unknown keyID 1`。
- 第 3 步所有错误信息只含 keyID/长度等元信息，**不含密文串本身**。

## 清理

- 停止 BFE（如启动成功则终止）、关闭 mock 后端。
