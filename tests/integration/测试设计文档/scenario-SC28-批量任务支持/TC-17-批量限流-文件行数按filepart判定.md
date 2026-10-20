# TC-17 批量限流-按文件行数判定（multipart 上传，issue #1434）

## 目的

验证 `batch_limits.max_file_lines` 的判定对象是 multipart 请求中 `name="file"` part 的内容行数，而不是整条 multipart 信封（boundary 行、part 头、`purpose` 字段、CRLF 各占行）。修复前，1 行文件 + `max_file_lines=10` 会被 413 拒绝或流式中断（信封 11 行）；修复后同一文件必须被接受，且 `BATCH_FILE.lines` 记为文件内容行数。

## 前置条件

- 见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。
- `rate_limit` 策略配 `batch_limits.max_file_lines=<N>`、`max_file_bytes=1048576`，绑定 `ak_batch`。
- mock provider 的 `/v1/files` 返回 File 对象（`id`/`bytes`/`filename`/`purpose`）。

## 请求构造

- 上传用场景公共 helper `e.upload`：`multipart/form-data`，`file` part 文件名 `batch.jsonl`，内容为 2 行 JSONL（`inputJSONL`，274 字节），随后 `purpose=batch` 表单字段 → 信封 12 行 / 638 字节。
- 子用例 A（放行）：`max_file_lines=2`（恰好等于文件行数，小于信封行数 12）。
- 子用例 B（拒绝）：同一文件、`max_file_lines=1`。
- 子用例 C（流式放行）：同一文件、`max_file_lines=2`，请求改用 `Transfer-Encoding: chunked`（无 `Content-Length`）。
- 子用例 D（流式中断）：文件内容 10 行、`max_file_lines=3`，`chunked` 上传。

## 后端响应

mock provider `/v1/files` 返回 `200` + `{"id":"file-in-1","bytes":274,"filename":"batch-input.jsonl","purpose":"batch"}`；子用例 B/D 不应到达 provider。

## 执行步骤

1. 启动 BFE（`e.startBFE`，策略见前置条件），等待配置收敛。
2. 子用例 A：`e.upload(t, inputJSONL)` → 断言 `200` 且返回 `file id`。
3. 断言 miniredis `BATCH_FILE:<file-id>` 的 `lines=2`、`bytes=274`、`lines_src=exact`、`bytes_src=exact`（信封口径会得到 12/638）。
4. 断言 mock provider 收到 1 次上传，且收到的 body 与客户端 multipart 逐字节一致（透传）。
5. 子用例 B：重建环境（`max_file_lines=1`）后 `e.upload` → 断言 `413` + `BATCH_FILE_TOO_LARGE` + `limit_type=batch_file`，provider 上传次数 0。
6. 子用例 C：`chunked` 上传同一文件（`max_file_lines=2`）→ 断言 `200`。
7. 子用例 D：`chunked` 上传 10 行文件（`max_file_lines=3`）→ 断言非 2xx（连接中断/5xx），provider 未拿到完整 body；计数器 `UPLOAD_ABORT_STREAM` ≥ 1，且 `BATCH_FILE` 未写入该文件绑定。

## 预期结果

| 子用例 | 状态码 | provider | 绑定/计数器 |
|--------|--------|----------|-------------|
| A | 200 | 1 次，body 逐字节一致 | `BATCH_FILE.lines=2/bytes=274`，`*_src=exact` |
| B | 413 `BATCH_FILE_TOO_LARGE` | 0 次 | `UPLOAD_REJECT_SIZE` ≥ 1 |
| C | 200 | 1 次 | `lines=2`，`*_src=exact`（流式闸门已精确） |
| D | 非 2xx（中断） | 收到截断请求 | `UPLOAD_ABORT_STREAM` ≥ 1 |

## 清理

`e.Close()`：停止 BFE、关闭 mock provider、关闭 miniredis、删除临时工作目录。