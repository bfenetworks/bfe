# TC-08 双 keyID 并存解密

## 目的

验证轮换并存期形态：keyring 同时含 keyID=1 与 keyID=2 时，不同 keyID 加密的密文
各自选钥解密（选钥自描述，无需字段级密钥映射），同一文件内可混合加载。

对应实现：`TestTC08_MultiKeyIDCoexist`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- keyring 含 keyID=1（keyA）与 keyID=2（keyB）。
- `token_rule.data`：`ak-old` 外层键用 keyID=1 加密、`ak-new` 外层键用 keyID=2
  加密，共存于同一 Tokens map。

## 执行步骤

1. 启动 BFE。
2. 分别携带 `ak-old`、`ak-new` 发送请求。

## 预期结果

- 启动成功：两个 keyID 的密文各自解密、明文索引完整。
- 第 2 步两个 token 均返回 200。

## 清理

- 停止 BFE、关闭 mock 后端。
