# System metrics

BFE has a variety of built-in metrics and supports multiple output formats, which can be obtained through monitor interfaces.

## Configuration
Set monitor port in the BFE core configuration file (conf/bfe.conf).

```
[server]
monitorPort = 8421
```

## Fetch metric categories
Visit the following address for a list of available metrics categories

```
http://<addr>:8421/monitor
```

## Fetch metric data

```
http://<addr>:8421/monitor/<category>
```

## Fetch metric data in specified format

Currently supported format of metrics: 
 * [prometheus](https://prometheus.io/)
 * kv
 * json (default)

Specify the format of the output like below:

```
http://<addr>:8421/monitor/proxy_state?format=prometheus
```

