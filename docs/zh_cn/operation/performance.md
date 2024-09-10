# 性能数据采集

BFE内置了CPU profile接口，配合使用火焰图工具，用于定位分析性能问题

## 配置管理端口

在BFE核心配置文件(conf/bfe.conf)中, 配置MonitorPort

```ini
[Server]
MonitorPort = 8421
```

## 工具准备

* FlameGragh

```bash
$ git clone https://github.com/brendangregg/FlameGraph
```

其中包含stackcollpase-go.pl和flamegraph.pl

## 操作步骤

* 获取性能采样数据

```bash
$ go tool pprof -seconds=60 -raw -output=bfe.pprof  http://<addr>:<port>/debug/pprof/profile
```

注：seconds=60 表示抓取60s的采样数据

* 转换并绘制火焰图

```bash
$ ./stackcollpase-go.pl bfe.pporf > bfe.flame
$ ./flamegraph.pl bfe.flame > bfe.svg
```

* 在浏览器中打开bfe.svg查看

![火焰图示例](../../images/bfe-flamegraph.svg)
