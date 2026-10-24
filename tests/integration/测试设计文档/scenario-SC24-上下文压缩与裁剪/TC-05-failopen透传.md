# TC-05 fail-open 透传

## 用例编号与名称

TC-05 fail-open 透传（skip_parse_err / repair_rollback）

## 所属场景

SC24 上下文压缩与裁剪

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_context`）

## 测试目的

验证 fail-open 原则：解析失败（`skip_parse_err`）与结构修复失败（`repair_rollback`）两类异常路径一律放行原始请求——上游收到逐字节不变的 body，客户端得到上游正常响应（非 5xx），bfe 不报错。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程。

## 前置条件

同 TC-01。

## 配置构造

同 TC-01。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body 要点 |
|------|------|------|---------------|-----------|
| 1 | `ctx.example.org` | `/v1/chat/completions` | `Bearer ak_ctx` | 非法 JSON：`{"model":"gpt-test","messages":[{"role":"user","content":"hi"},`（截断） |
| 2 | 同上 | 同上 | 同上 | 合法 JSON 但 role 非法（`wizard`，内容 600 字符，估算超触发线）⇒ repair 无法修复 ⇒ 整体回滚 |

## 预期结果

- 两个请求均返回 200（上游正常应答），均无 `x-ai-context-compression` 响应头。
- mock 后端收到 2 个请求：body 分别与客户端发送**逐字节一致**（畸形 body 未被截断、未被重写）。
- 访问日志：
  - 步骤 1：`ai_context_compress_status=skip_parse_err`；
  - 步骤 2：`ai_context_compress_status=repair_rollback`，不带 after-tokens。
