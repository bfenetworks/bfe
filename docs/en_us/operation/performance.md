# Performance

BFE has built-in CPU profile interfaces, which can be used in conjunction with the FlameGraph tool to locate and analyze performance problems.

## Configs

Use the same port as the monitor
```
[server]
monitorPort = 8421
```

## Tools

* FlameGragh

```
$ git clone https://github.com/brendangregg/FlameGraph
```

Which contains stackcollpase-go.pl and flamegraph.pl tools

## Step

* Get performance sampling data
```
$ go tool pprof -seconds=60 -raw -output=bfe.pprof  http://<addr>:<port>/debug/pprof/profile
```
Note: seconds=60 means capturing 60 seconds of stack samples

* Transform and draw FlameGraph

```
$ ./stackcollpase-go.pl bfe.pporf > bfe.flame
$ ./flamegraph.pl bfe.flame > bfe.svg
```

* Open bfe.svg in browser

