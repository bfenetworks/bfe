# TC-30 回源 Accept-Encoding 协商收窄（issue #1468）

## 用例编号与名称

TC-30 回源 `Accept-Encoding` 协商收窄（`Server.AIUpstreamAcceptEncoding = auto`）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1468 修复）

## 测试目的

验证 BFE 在 AI 转发时把握手结果收窄到**网关可解码的编码集合**，从而让压缩流式响应的 usage 始终可解析：

1. 客户端声明 `gzip, deflate, br, zstd` 时，上游实际收到 `gzip, deflate`（不含 `br`/`zstd`）；
2. 收窄只作用于**出站**请求头：入站请求头（访问日志、`mod_compress`、`mod_ai_cache` 可见的那份）保持不变；
3. 收窄后响应仍正常计费（与 TC-29 同判据）；
4. 客户端只声明网关解不了的编码（如 `zstd`）时，**不改写**该头（不得回一个客户端没接受的编码），
   此时若上游返回 zstd，则退化为估算并在 `ai_usage_source=estimated` 上可见。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动：
   - 记录每次请求的完整请求头（既有 `headerCopies` 能力）；
   - 响应为 gzip 压缩的流式 SSE（与 TC-29 相同的 `SSECompress = "gzip"` 形态），
     末帧 usage 与 TC-29 相同（`prompt_tokens=296476`、`completion_tokens=160`、`cached_tokens=296320`）。
4. 临时 BFE 配置：
   - `Server.AIUpstreamAcceptEncoding = auto`（默认值，本用例显式写出以示对照）；
   - `cluster_rmb` 的 `ModelTable` 价格与 TC-29 相同；
   - `Server.EstimateToken = true`。
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- 无需新增 BFE 配置键；`Server.AIUpstreamAcceptEncoding` 已在 `conf/bfe.conf` 与配置文档中说明。
- mock 后端需要能回读上游收到的 `Accept-Encoding`（`headerCopies`）。

## BFE 请求

本用例含三个臂，均发往 `rmb.example.org`、路径 `/v1/chat/completions`、`Authorization: Bearer ak_user_a`：

| 臂 | Accept-Encoding | Body | 目的 |
|----|-----------------|------|------|
| A | `gzip, deflate, br, zstd` | `{"model":"deepseek-flash","stream":true}` | 断言上游收到 `gzip, deflate` |
| B | `zstd` | 同上 | 断言上游收到的仍是 `zstd`（交集为空不改写） |
| C | `gzip;q=0.5, zstd` | 同上 | 断言 q 值保留：上游收到 `gzip;q=0.5` |

## 执行步骤

1. 启动嵌入式 Redis 并预置余额。
2. 启动 mock 后端。
3. 启动 `bfe` 进程，加载临时配置。
4. 依次发送 A/B/C 三个臂的请求，各读取完整响应。
5. 读取 mock 后端记录的请求头，断言上游实际收到的 `Accept-Encoding`。
6. 等待异步扣减，读取 Redis 余额与 pb3 访问日志。

## 预期结果

| 臂 | 上游收到的 `Accept-Encoding` | 计费 |
|----|------------------------------|------|
| A | `gzip, deflate` | 与 TC-29 相同：`ai_usage_source = parsed`，扣减 **640240** |
| B | `zstd`（未被改写） | 上游虽返回 gzip（mock 固定 gzip），仍可解析：`parsed`，扣减 640240；**关键断言是头未被改写** |
| C | `gzip;q=0.5` | 与 A 相同 |

补充断言：

- 三个臂的入站请求头（`basicReq.HttpRequest.Header`）未被修改：访问日志与 `mod_compress` 看到的仍是客户端原值
  （通过「同一连接复用 + `Vary`/日志字段不变」间接验证，或在单元测试 `reverseproxy_accept_encoding_test.go` 中直接断言）。
- 臂 B 若让 mock 返回 `Content-Encoding: zstd`（变体），则应得到 `ai_usage_source = estimated` 且
  `ResStreamDecodeSkipped` +1、转发字节不变——该变体由 TC-31 覆盖。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
