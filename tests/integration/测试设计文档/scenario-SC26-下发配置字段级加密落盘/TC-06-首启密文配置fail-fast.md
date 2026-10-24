# TC-06 首启密文配置 fail-fast

## 目的

验证首次启动（非 reload）失败语义：磁盘配置已是密文而 keyring 缺失/错误时，BFE
必须拒绝启动，而不是容忍启动丢空鉴权配置（容忍启动会静默丢失全部鉴权覆盖）。

对应实现：`TestTC06_FirstStartCiphertextFailFast`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- `token_rule.data` 外层键为密文（keyID=1，keyA）。
- 场景 A：`bfe.conf` 未配置 `[Security] KeyFile`。
- 场景 B：keyring 配置了 keyB（与 keyA 不符）。

## 执行步骤

1. 场景 A：启动 BFE，等待退出。
2. 场景 B：启动 BFE，等待退出。
3. 对照组：keyring 配置 keyA，启动 BFE。

## 预期结果

- 场景 A/B：BFE 启动失败（进程非零退出 / 启动报错），错误为解密失败或 keyring 未
  配置，**不含密文**。
- 对照组：启动成功，密文 token 可鉴权。

## 清理

- 终止未退出的进程、关闭 mock 后端。
