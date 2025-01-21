# 管理接口说明

BFE提供了一套管理接口，用于监控指标获取、配置加载、调试及分析等。这些接口仅允许来自内部网络访问。

## 配置管理端口

在BFE核心配置文件(conf/bfe.conf)设置管理端口：

```
[Server]
MonitorPort = 8421
```

## 管理接口列表

下列接口基于HTTP协议GET方法访问。

| 路径            | 描述                                          |
| --------------- | -------------------------------------------- |
| /monitor        | 列出所有的监控类别，详见[监控指标获取](monitor.md) |
| /monitor/{name} | 返回指定`name`监控类别中所有指标 |
| /reload         | 列出所有的配置加载项，详见[配置热加载](reload.md) |
| /reload/{name}  | 热加载指定`name`配置信息 |
| /debug/pprof/   | 详见Go文档[pprof Index](https://golang.org/pkg/net/http/pprof/#Index)|
| /debug/cmdline  | 详见Go文档[pprof Cmdline](https://golang.org/pkg/net/http/pprof/#Cmdline)|
| /debug/profile  | 详见Go文档[pprof Profile](https://golang.org/pkg/net/http/pprof/#Profile)|
| /debug/symbol   | 详见Go文档[pprof Symbol](https://golang.org/pkg/net/http/pprof/#Symbol)|
| /debug/trace    | 详见Go文档[pprof Trace](https://golang.org/pkg/net/http/pprof/#Trace)|
