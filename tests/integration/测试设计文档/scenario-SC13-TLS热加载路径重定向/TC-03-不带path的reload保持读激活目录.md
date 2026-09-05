# TC-03 不带 path 的 reload 保持读激活目录

## 目的

验证向后兼容：不带 `path` 参数调用 `/reload/tls_conf`（无 conf-agent 的手工 reload 场景）时，client CA 仍从激活的 `tls_conf` 目录读取，行为与修复前一致，不受版本目录内容影响。

对应实现：`TestTC03_ReloadWithoutPathKeepsActivatedDir`。

## 前置条件

- BFE 已启动，激活规则 `ClientAuth=false`，且激活目录 `tls_conf/client_ca/example_ca.crt` 已被删除。
- 版本目录 `tls_conf_20260904205703` 已生成（含 `ClientAuth=true` 规则和完整的 `client_ca/example_ca.crt`）。

## 执行步骤

1. 调用 `GET /reload/tls_conf`（不带 `path`）。
2. 不带客户端证书进行 TLS 握手。

## 预期结果

- reload 返回 `{"error":null}`（激活目录规则为 `ClientAuth=false`，不触发 CA 加载）。
- 握手成功：生效的仍是激活目录的配置，版本目录的 `ClientAuth=true` 未被采用。

## 清理

- 停止 BFE、关闭 mock 后端。
