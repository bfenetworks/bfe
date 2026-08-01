# mod_unified_waf

## 模块简介

BFE 支持在 http request 的处理流程中引入统一的第三方WAF支持。

## 基础配置

模块基础配置文件说明详见 [mod_unified_waf.conf](../../configuration/mod_unified_waf/mod_unified_waf.conf.md)。

## WAF访问具体参数配置

WAF 访问具体参数配置文件说明详见 [mod_unified_waf.data](../../configuration/mod_unified_waf/mod_unified_waf.data.md)。

## WAF访问产品线配置

WAF 访问产品线配置文件说明详见 [product_param.data](../../configuration/mod_unified_waf/product_param.data.md)。

## WAF RS实例池配置

WAF RS 实例池配置文件说明详见 [waf_instances.data](../../configuration/mod_unified_waf/waf_instances.data.md)。

## 监控项

| 监控项        | 描述                     |
| ------------- | ------------------------ |
| REQ_NO_CHECK  | 未执行 WAF 检测的请求数  |
| REQ_FORBIDDEN | WAF 判定拦截的请求数     |
| REQ_OK        | WAF 判定正常的请求数     |
| REQ_TIMEOUT   | WAF 检测超时的请求数     |
| REQ_OTHER     | 其他状态请求数           |
| NET_ERR       | WAF 检测网络错误的请求数 |

模块还通过 delay counter 统计以下延迟（key prefix）：

| 延迟统计项                        | 描述              |
| --------------------------------- | ----------------- |
| waf_client_delay                  | WAF 请求响应延迟  |
| waf_client_delay_peek_body        | 读取 body 延迟    |
| waf_client_delay_call_competition | 并发竞争延迟      |
