# mod_trace

## 模块简介

mod_trace根据自定义的条件，为请求开启分布式跟踪。

## 基础配置

模块基础配置文件说明详见 [mod_trace.conf](../../configuration/mod_trace/mod_trace.conf.md)。

## 规则配置

模块规则配置文件说明详见 [trace_rule.data](../../configuration/mod_trace/trace_rule.data.md)。

## 监控项

| 监控项           | 描述             |
| ---------------- | ---------------- |
| START_SPAN_COUNT | 开启的 span 总数 |
| FINISH_SPAN_COUNT| 结束的 span 总数 |
