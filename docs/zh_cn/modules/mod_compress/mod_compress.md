# mod_compress

## 模块简介

mod_compress支持对响应主体压缩。

## 基础配置

模块基础配置文件说明详见 [mod_compress.conf](../../configuration/mod_compress/mod_compress.conf.md)。

## 规则配置

模块规则配置文件说明详见 [compress_rule.data](../../configuration/mod_compress/compress_rule.data.md)。

## 监控项

| 监控项                   | 描述                              |
| ----------------------- | --------------------------------- |
| REQ_TOTAL               |统计mod_compress处理的总请求数        |
| REQ_SUPPORT_COMPRESS    |支持压缩请求数                       |
| REQ_MATCH_COMPRESS_RULE |命中压缩规则请求数                    |
| RES_ENCODE_COMPRESS     |响应被压缩请求数                      |
| RES_ENCODE_GZIP_COMPRESS|响应被gzip压缩请求数                  |
| RES_ENCODE_BR_COMPRESS  |响应被brotli压缩请求数                |
