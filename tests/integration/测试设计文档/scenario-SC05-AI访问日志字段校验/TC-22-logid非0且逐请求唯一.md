# TC-22 logid 非 0 且逐请求唯一（issue #1417）

## 目的

验证 `mod_access_pb3` 写入的**日志级字段** `BfeLog.logid` 是真实的「逐请求唯一标识」：

- 每个请求的 `logid` **非 0**；
- 同一秒内同 API Key、同模型的多次请求，`logid` **互不相同**；
- 取值 ≤ `2^63-1`（报表库 `logid` 列为有符号 `BIGINT`）。

**缺陷背景（bfenetworks/bfe#1417）**：修复前 `request_log.go:51` 以十进制解析 `mod_logid` 产出的 32 位十六进制 logid（`strconv.ParseUint(req.LogId, 10, 64)`），错误被 `_` 丢弃 → `logid` **恒为 0**。报表库 `bfe_ai_request_log` 的幂等键 `uk_dedup (hostid, log_time, ai_apikey_id, ai_requested_model, logid)` 中 `log_time` 为秒级，写入侧又是 `ON DUPLICATE KEY UPDATE` 全列覆盖，于是同秒同 Key 同模型的 n 个并发请求**只留 1 行**（生产实测丢率恰为 (n−1)/n，窗口 13.5%）。

本 TC 断言的是「判别列可用」这一**上游前提**；下游一半（报表库在唯一键下确实保留 n 行）由 log-reader 的 `LR03` 场景断言，两边合起来才闭环。

## 前置条件

- 嵌入式 Redis：`quota:plan_rmb` 余额充足（`10000000000`）。
- `cluster_rmb` 配置 `ModelTable` 与 RMB 配额，`AIConf` 与 TC-01 相同（`defaultRMBAIConf()`）。
- API Key `ak_user_a`（`key_id = user_a_key_id`）绑定 `plan_rmb`。
- **两个臂的 `mod_logid` 状态不同**：
  - 臂 1：沿用场景默认 `testdata/bfe.conf`，**未加载 `mod_logid`**（此时 `req.LogId` 为空，走本地重生兜底路径）；
  - 臂 2：在 `bfe.conf` 追加 `Modules = mod_logid` 后重启 BFE（生产出厂默认形态，`req.LogId` 由 `mod_logid` 分配）。

## 请求构造

| 项 | 值 |
|----|----|
| Method | `POST` |
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Headers | `Authorization: Bearer ak_user_a`、`Content-Type: application/json` |
| Body | `{"model":"deepseek-chat"}` |
| 次数 | 每个臂**连续 3 次**（同一秒内，同 Key 同模型——即报表库唯一键的碰撞前提） |

## 后端响应

mock 后端 `cluster_rmb` 返回 `200` 与标准 usage：

```json
{"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}
```

## 执行步骤

1. 启动环境（臂 1 配置：未加载 `mod_logid`），设 Redis 余额。
2. 连续发送 3 次请求，断言均 `200`。
3. 等待 500ms 让访问日志刷盘，停止 BFE。
4. 用 `common.ParseBfeLogsAfterStop(logDir)` 解码 b2log（**注意**：`ParseAccessLog*` 会丢弃日志级字段，故本 TC 使用新增的 `ParseBfeLogs*`）：
   - 记录数应为 3；
   - 每行 `BfeLog.logid` 非 0、≤ `2^63-1`、三行互不相同；
   - 打印每行的 `logid` / `timestamp` / `ai_apikey_id` / `ai_requested_model` 作为证据。
5. 删除臂 1 的日志文件（`log/pb_access3.log`），在 `bfe.conf` 追加 `Modules = mod_logid`，**重启 BFE**。
6. 重复步骤 2–4，断言臂 2 同样满足上述口径。

## 预期结果

| # | 断言 | 说明 |
|---|------|------|
| 1 | 臂 1：3 行 `logid` 全部非 0 且互不相同 | `mod_logid` **未加载**时依然成立——取值自举，不依赖模块启用 |
| 2 | 臂 2：3 行 `logid` 全部非 0 且互不相同 | 生产默认形态（`mod_logid` 启用，`logid` 复用其分配值） |
| 3 | 所有 `logid` ≤ `9223372036854775807` | 报表库（MySQL / Doris / StarRocks）为有符号 `BIGINT`；超界会被 Doris 非严格模式截断为边界值（高位行互撞）或在 MySQL 严格模式整批报错（`Error 1264`） |
| 4 | 3 行共享同一 `ai_apikey_id` 与 `ai_requested_model` | 打印证据；唯一差别就是 `logid`，即修复的目标 |

**修复前表现**：两臂的 `logid` 均为 `0`，断言 1/2 失败（且第 4 条的"唯一差别就是 logid"退化为"三行完全无法区分"）。

## 清理

`testEnv.Close()` 停止 BFE、关闭 mock 后端与 Redis；`processEnv` 的临时工作目录按既有机制清理。

## 说明与边界

- **本场景未覆盖的部分**：`mod_logid` 正常启用时「`logid` 与 `X-Bfe-Log-Id` 字符串逐值恒等」由单元测试断言（`bfe_modules/mod_access_pb3/logid_test.go` 的 `TestRequestLogGenLogidParsed`、`bfe_modules/mod_logid/mod_logid_test.go` 的 `TestGenLogId`），集成测试侧不做该断言——BFE 只在**请求 header** 上设置 `X-Bfe-Log-Id`，客户端（非信任源）无法读到该值，无法在集成层构造恒等比对。
- **报表库侧**：本 TC 只断言 pb 日志；「同秒并发 n 请求 → 明细表 n 行」由 log-reader `scenario-LR03-mysql-write` 新增用例断言（需要 `ai-gateway-api` DDL 把 `logid` 加入 `uk_dedup`）。
- **相关设计文档**：`docs/zh_cn/modifications/2026-10-13-issue-1417-logid-dedup-fix/design-changes.md`。