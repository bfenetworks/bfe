# mod_access_pb3 动态配置

## 配置简介

`mod_access_pb.data` 是 `mod_access_pb3` 模块的动态配置文件，用于配置访问日志的可选项采集策略（当前支持按白名单采集请求头/响应头，写入 proto 字段 `req_headers`/`res_headers`）。

该文件由 `mod_access_pb3.conf` 的 `BasicConf.DataConfPath` 指定路径，**支持热加载**：修改文件后调用 BFE 监控端口的 reload 接口（`/reload/mod_access_pb3`）即可生效，无需重启。文件不存在或整段缺省时，模块按缺省行为运行（不采集任何白名单字段），与未启用该能力时行为一致。

## 配置描述

| 配置项              | 类型   | 参数含义               | 必填 | 补充描述                                                     | 合法性条件                                       |
| ------------------- | ------ | ---------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------ |
| Version             | String | 配置文件版本           | Y    | 通常采用时间戳格式，如 `20261115090000`                      | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| access_header       | Object | 请求/响应头白名单配置  | N    | 缺省表示不采集任何请求头/响应头                              | -                                                |
| access_header.req_headers | Array | 需要采集的请求头名称列表 | N  | 名称匹配**不区分大小写**（按 HTTP 规范化形式归一后比较）；采集值写入 proto 字段 `req_headers`（`repeated HttpHeader{key,value}`） | 元素须为合法 HTTP header 名称（RFC 7230 token 字符），去空白后长度 ≤ 128 字节 |
| access_header.res_headers | Array | 需要采集的响应头名称列表 | N  | 同上，采集值写入 proto 字段 `res_headers`                    | 同上                                             |

说明：

- 头名称匹配不区分大小写：配置 `business-type` 与 `Business-Type` 等价；写入 proto 的 `key` 统一为 HTTP 规范化形式（如 `Business-Type`）；
- 同名头多值按 `,` 拼接为一个值（与既有独立字段的采集惯例一致），value 超长时截断至 2048 字节；
- 文件中的未知顶层字段（段）会被忽略并记录 Warn 日志，不影响已知段的加载（前向兼容）；
- 头名称白名单内容不做敏感头限制；原始 API Key 由模块运行时统一掩码（`maskSensitiveCredentials`），任何字段中均不会落原始凭证。

## 配置示例

```json
{
    "Version": "20261115090000",
    "access_header": {
        "req_headers": ["business_type", "x-request-id"],
        "res_headers": ["x-trace-id"]
    }
}
```
