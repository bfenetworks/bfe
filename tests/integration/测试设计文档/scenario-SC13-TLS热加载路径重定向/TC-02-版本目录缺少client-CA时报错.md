# TC-02 版本目录缺少 client CA 时 reload 报错

## 目的

验证修复没有弱化校验：版本目录中缺少 `tls_rule_conf.data` 引用的 client CA 文件时，reload 仍然报错（只是读取的基目录从激活目录换成了版本目录）。

对应实现：`TestTC02_ReloadWithPathMissingClientCA`。

## 前置条件

- BFE 已启动，激活规则 `ClientAuth=false`。
- 版本目录 `tls_conf_20260904205703` 已生成，但 `client_ca/` 下没有 `example_ca.crt`。

## 执行步骤

1. 生成版本目录：`tls_rule_conf.data` 为 `ClientAuth=true, ClientCAName=example_ca`，但不放置 `example_ca.crt`。
2. 调用 `GET /reload/tls_conf?path=tls_conf_20260904205703`。

## 预期结果

- 返回的 JSON `error` 非空，且包含 `ClientCALoad`。
- BFE 继续使用激活目录的旧配置（TLS 规则不变）。

## 补充说明

conf-agent 在收到 reload 错误后会保持符号链接不切换，这正是"配置未通过校验不生效"的安全语义；本修复只保证"文件已就位时 BFE 看对目录"。

## 清理

- 停止 BFE、关闭 mock 后端。
