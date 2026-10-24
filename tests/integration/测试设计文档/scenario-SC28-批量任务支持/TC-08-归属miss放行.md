# TC-08-归属miss放行

## 前置条件

见 `../场景说明.md` §2/§3（真实 bfe + miniredis + mock provider，tc 级 rule data 由测试代码生成）。

## 测试步骤与断言

ak_batch 下载无绑定 file_id（file-unknown-9）→ OwnerCheckMissPolicy=allow_log → 期望 200 且响应体为 provider 的结果文件字节；绑定仍不存在。

## 观测点

访问日志（common.ParseAccessLog）、miniredis 键值、mock 后端命中记录。
