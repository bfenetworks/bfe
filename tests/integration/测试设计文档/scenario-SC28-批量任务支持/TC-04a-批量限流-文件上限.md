# TC-04-批量限流维度

## 前置条件

见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。

## 测试步骤与断言

rate_limit 策略配 batch_limits.max_file_bytes=100 并绑定 ak_batch：上传 200 字节文件(CL 已知)返回 413 BATCH_FILE_TOO_LARGE(limit_type=batch_file)；另测 max_create_rpm=1：第二次创建返回 429 且错误信息含策略名。

## 观测点

访问日志（common.ParseAccessLog）、miniredis 键值、mock 后端命中记录。
