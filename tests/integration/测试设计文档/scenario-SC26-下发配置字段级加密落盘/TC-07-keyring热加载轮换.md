# TC-07 keyring 热加载轮换

## 目的

验证轮换的 BFE 侧关键路径：**先让解密端具备新钥能力，再让加密端产出新密文**——
更新 keyring 文件（追加新钥）后，触发既有 reload 端点（`/reload/mod_ai_token_auth`、
`/reload/server_data_conf`）即加载新钥加密的配置，**无需重启**。

对应实现：`TestTC07_KeyringHotReloadRotation`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- keyring v1 仅含 keyID=1（keyA）；`token_rule.data` 与 `cluster_conf.data` 均用
  keyID=1 加密。
- BFE 已启动且 `ak-enc`/`k1` 调用成功。

## 执行步骤

1. 基线：携带 `ak-enc` 请求返回 200。
2. 模拟控制面轮换：测试代码生成 keyring v2（含 keyID=1 的 keyA + keyID=2 的 keyB），
   **覆写** keyring 文件；同时把 `token_rule.data` 外层键改写为 keyID=2 加密的
   `ak-enc-v2`（旧钥密文不再出现，模拟下一导出周期全量重加密）。
3. 依次调用 `GET /reload/mod_ai_token_auth` 与 `GET /reload/server_data_conf`。
4. 携带 `ak-enc-v2` 发送请求。
5. 携带旧明文 `ak-enc` 发送请求。

## 预期结果

- 第 3 步两个 reload 均返回 `{"error":null}`（keyring 在加载时重读，新钥生效）。
- 第 4 步返回 200：新钥密文加载成功。
- 第 5 步返回 401：旧 token 已被新配置替换（全量重导出语义）。

## 清理

- 停止 BFE、关闭 mock 后端。
