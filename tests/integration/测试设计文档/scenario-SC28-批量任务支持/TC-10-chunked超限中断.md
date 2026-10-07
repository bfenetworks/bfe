# TC-10-chunked超限中断

## 前置条件

见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。

## 测试步骤与断言

rate policy max_file_lines=3。客户端以 chunked（无 Content-Length）上传 10 行 jsonl → 计数 Reader 在第 4 行中断 → 客户端收到非 2xx 或连接错误；provider 收到的 body 不完整（行数<10）。

## 观测点

访问日志（common.ParseAccessLog）、miniredis 键值、mock 后端命中记录。
