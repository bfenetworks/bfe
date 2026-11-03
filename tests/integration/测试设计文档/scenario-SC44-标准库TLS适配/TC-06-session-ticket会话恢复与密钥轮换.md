# TC-06 session ticket 会话恢复与密钥轮换

## 目的

验证 Redis 跨实例 session cache 删除后，标准库**本进程 session ticket** 恢复机制工作正常：同进程内复用会话；通过 `/reload/tls_session_ticket_key` 轮换票据密钥后，旧票据无法恢复，必须全量握手。

对应实现（Phase 6）：`TestTC06_SessionTicketResumptionAndKeyRotation`。

## 前置条件

- `tls_conf/session_ticket_key.data` 为 32 字节（64 hex 字符）标准库 `SessionTicketKey`，`SessionTicketsDisabled=false`。
- BFE 已启动；客户端持有显式 `ClientSessionCache`（`tls.NewLRUClientSessionCache(8)`）并跨多次连接复用同一 `tls.Config`。

## 执行步骤

1. 第一次连接（TLS 1.3）握手并正常关闭，检查服务端下发了会话票据、客户端缓存了会话。
2. 使用同一 `tls.Config`/session cache 发起第二次连接，检查 `ConnectionState.DidTLS13Resume == true`。
3. 以 TLS 1.2（`MaxVersion=1.2`）重复第 1~2 步，第二次连接 `ConnectionState.DidResume == true`（1.2 票据恢复）。
4. 生成新 32 字节密钥写入 `session_ticket_key.data`，调用 `GET /reload/tls_session_ticket_key`。
5. 仍用旧 session cache 再次连接。

## 预期结果

- 第 2、3 步会话恢复成功（票据密钥在进程内有效）。
- 第 4 步 reload 返回 `{"error":null}`，`HttpsListener.UpdateSessionTicketKey` 通过克隆替换 listener 当前配置。
- 第 5 步无法恢复旧会话：`DidTLS13Resume == false`（1.2 为 `DidResume == false`），执行全量握手但仍连接成功。

## 补充说明

旧 fork 票据密钥文件为 48 字节（Name+Key）结构；加载器仍接受 48 字节以兼容存量部署，但标准库只使用前 32 字节。本场景固定使用新格式 32 字节文件；旧格式兼容由配置加载单元测试覆盖。

## 清理

- 停止 BFE、关闭 mock 后端。
