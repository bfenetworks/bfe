# TC-18 批量限流-按文件字节判定（multipart 上传，issue #1434）

## 目的

验证 `batch_limits.max_file_bytes` 的判定对象是 multipart 请求中 `name="file"` part 的内容字节数，而不是信封的 `Content-Length`。修复前，只要信封（文件内容 + 约 364 字节 part 结构开销）超过上限就返回 413，导致"文件本身没超限"的上传被拒；修复后按文件内容判定，恰好等于上限必须放行。

## 前置条件

- 见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider）。
- `rate_limit` 策略配 `batch_limits.max_file_bytes=<N>`（不配 `max_file_lines`），绑定 `ak_batch`。
- mock provider 的 `/v1/files` 返回 File 对象。

## 请求构造

- 上传用场景公共 helper `e.upload`：`multipart/form-data`，`file` part 内容为 2 行 JSONL（`inputJSONL`，274 字节），另有 `purpose=batch` 字段 → 信封 638 字节。
- 子用例 A（放行）：`max_file_bytes=274`（信封 638 > 274）。
- 子用例 B（拒绝）：同一文件、`max_file_bytes=273`。
- 子用例 C（raw 回归）：`application/jsonl` 直接发 200 字节 body、`max_file_bytes=100` → 413（非 multipart 时信封即文件，早期 `Content-Length` 拒绝保持，与 TC-04a 同型）。
- 子用例 D（逐策略预检回归）：multipart 上传 + `max_file_bytes=274`，但请求同时带 `max_file_bytes=400` 的另一条策略绑定 → 仍需 200，证明 `mod_ai_rate_limit` 的逐策略预检不再用信封长度判 multipart。

## 后端响应

mock provider `/v1/files` 返回 `200` + `{"id":"file-in-1","bytes":274,"purpose":"batch"}`。

## 执行步骤

1. 启动 BFE（子用例 A 策略），`e.upload(t, inputJSONL)` → 断言 `200`。
2. 断言 miniredis `BATCH_FILE:<file-id>.bytes=274`（信封口径会是 638）、`bytes_src=exact`。
3. 子用例 B：重建环境（`max_file_bytes=273`）后上传 → `413 BATCH_FILE_TOO_LARGE`，provider 0 次、`UPLOAD_REJECT_SIZE` ≥ 1。
4. 子用例 C：`e.do(t, "POST", "/v1/files", 200 字节, "application/jsonl")` → `413`；再发 100 字节 → `200`（边界 ±1）。
5. 子用例 D：按前置条件配两条策略（400 / 274）后上传同一 multipart → `200`，provider 1 次。

## 预期结果

| 子用例 | 状态码 | 观测点 |
|--------|--------|--------|
| A | 200 | `BATCH_FILE.bytes=274`、`bytes_src=exact`；provider body 逐字节一致 |
| B | 413 `BATCH_FILE_TOO_LARGE` | provider 0 次；`UPLOAD_REJECT_SIZE` ≥ 1 |
| C | 200 字节 → 413；100 字节 → 200 | raw 上传行为与 TC-04a 一致 |
| D | 200 | 逐策略预检未按信封长度误拒 |

## 清理

`e.Close()`：停止 BFE、关闭 mock provider、关闭 miniredis、删除临时工作目录。