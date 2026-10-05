# TC-01 密文 token 配置加载与鉴权

## 目的

验证 `token_rule.data` 的 Tokens 外层键（api-key 值本身）以 `enc$v1$` 密文落盘时：
BFE 用 keyring 解密后重建明文索引，下游鉴权行为与明文配置一致；磁盘文件直读无明文。

对应实现：`TestTC01_EncryptedTokenRuleLoadAndAuth`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- 临时工作目录已生成 keyring（keyID=1）并配置 `bfe.conf [Security] KeyFile`。
- `token_rule.data` 由测试生成：`ak-enc` 的外层键为密文（内层 `key` 字段省略，控制面
  导出形态），`ak-123` 保持明文作对照；其余结构与模板一致。

## 执行步骤

1. 启动 BFE。
2. 直读工作目录 `mod_ai_token_auth/token_rule.data`：
   - 断言外层键含 `enc$v1$` 前缀且文件中不含明文 `ak-enc`；
   - 断言非敏感字段可读（`key_id: ak-enc-id` 等）。
3. 携带 `Authorization: Bearer ak-enc` 发送请求。
4. 携带 `Authorization: Bearer ak-123` 发送请求（明文对照）。
5. 携带错误 key `ak-wrong` 发送请求。

## 预期结果

- 第 1 步启动成功（fail-fast 不触发：keyring 正确）。
- 第 3、4 步均返回 200：密文/明文 token 解密后索引等价，鉴权通过。
- 第 5 步返回 401。

## 清理

- 停止 BFE、关闭 mock 后端。
