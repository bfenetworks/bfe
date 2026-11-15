# TC-05 reload 失败保留旧配置

## 目的

验证热加载的失败语义：非法数据文件（非法 JSON 或非法头名）使 reload 返回错误，**整份旧配置继续生效**，不出现半新半旧状态。

## 前置条件

- 初始 `mod_access_pb.data` 合法：

```json
{
    "Version": "20261115090000",
    "access_header": {
        "req_headers": ["business_type"],
        "res_headers": []
    }
}
```

## 请求构造

- 请求头携带 `Business-Type: rag` 与 `X-Request-Id: rid-1`；
- 客户端 token：`ak_hdr_a`。

## 执行步骤

1. 发送请求 → 200 → 断言 `req_headers` 含 `Business_type`；
2. 将数据文件改写为非法内容（两个子用例）：
   - 5a：非法 JSON（`{broken`）；
   - 5b：JSON 合法但头名非法（`"req_headers": ["bad name"]`）；
3. 请求 `/reload/mod_access_pb3`；
4. 再次发送相同请求 → 200 → 解码断言。

## 预期结果

- 步骤 3：reload 返回错误（非 200 或响应体含 error）；
- 步骤 4：`req_headers` 仍仅含 `{key: "Business_type", value: "rag"}`——旧白名单继续生效，`X-Request-Id` 仍未采集；
- BFE 运行日志记录加载失败原因。

## 清理

测试结束时关闭 BFE 与 mock 后端。
