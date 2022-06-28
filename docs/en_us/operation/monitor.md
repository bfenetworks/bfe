# System metrics

BFE has a variety of built-in metrics which can be exposed in various formats.

## Configure monitor port

Set monitor port in the BFE core configuration file (conf/bfe.conf).

```ini
[Server]
MonitorPort = 8421
```

## Fetch metric categories

Visit the following address for a list of available metric categories

```
http://<addr>:8421/monitor
```

## Fetch metrics

```
http://<addr>:8421/monitor/<category>
```

## Fetch metric data in specified format

Currently supported formats:

* [prometheus](https://prometheus.io/)
* kv
* json (default)

Specify the format of the output like below:

```
http://<addr>:8421/monitor/proxy_state?format=prometheus
```
