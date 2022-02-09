# 监控指标

BFE内置丰富的监控指标，支持多种格式，可以通过BFE实例的监控接口获得。

## 配置管理端口

在BFE核心配置文件(conf/bfe.conf)中, 配置MonitorPort

```ini
[Server]
MonitorPort = 8421
```

## 获取指标类别列表

访问如下地址，以获取监控项列表：

```
http://<addr>:8421/monitor
```

## 获取指标详情

```
http://<addr>:8421/monitor/<category>
```

## 输出格式

当前支持监控数据格式如下:

* [prometheus](https://prometheus.io/)
* kv
* json (默认格式)

使用format参数指定输出的格式, 示例：

```
http://<addr>:<port>/monitor/proxy_state?format=prometheus
```
