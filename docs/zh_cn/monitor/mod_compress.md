# 简介

mod_compress支持响应压缩，如：GZIP压缩。

# 监控项

| 监控项                   | 描述                              |
| ----------------------- | --------------------------------- |
| REQ_TOTAL               |统计mod_compress处理的总请求数        |
| REQ_SUPPORT_COMPRESS    |支持压缩请求数                       |
| REQ_MATCH_COMPRESS_RULE |命中压缩规则请求数                    |
| RES_ENCODE_COMPRESS     |响应被压缩请求数                      |