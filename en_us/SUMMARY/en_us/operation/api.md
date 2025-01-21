# Management API

BFE provides a set of management APIs for metrics exposing, configurations reloading, debugging and profiling etc. It should not publicly exposing the APIs, keeping them restricted to internal networks.

## Configuration

Set management port in BFE core configuration file(conf/bfe.conf)

```
[Server]
MonitorPort = 8421
```

## Endpoints

All the following endpoints must be accessed with a `GET` HTTP request.

| Path             | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| /monitor         | Lists all the monitor categories. See [System metrics](monitor.md) |
| /monitor/{name}  | Returns the metrics information of the monitor category specified by `name`. |
| /reload          | Lists all the reload entries. See [Configuration reload](reload.md) |
| /reload/{name}   | Reloads the configuration specified by `name`. |
| /debug/pprof/    | See the [pprof Index](https://golang.org/pkg/net/http/pprof/#Index) Go documentation.|
| /debug/cmdline   | See the [pprof Cmdline](https://golang.org/pkg/net/http/pprof/#Cmdline) Go documentation. |
| /debug/profile   | See the [pprof Profile](https://golang.org/pkg/net/http/pprof/#Profile) Go documentation. |
| /debug/symbol    | See the [pprof Symbol](https://golang.org/pkg/net/http/pprof/#Symbol) Go documentation. |
| /debug/trace     | See the [pprof Trace](https://golang.org/pkg/net/http/pprof/#Trace) Go documentation. |
