# Install using go

## Prerequisites
- golang 1.13+

## Installation 
- Get the source code and install

```bash
$ go get github.com/baidu/bfe
```

Executable object file location is ${GOPATH}/bin/bfe

!!! tip
    If you encounter an error such as "https fetch: Get ... connect: connection timed out", please set the GOPROXY and try again. See [Installation FAQ](../faq/installation.md)


## Run
- Run BFE with example configuration files:

```bash
$ cd ${GOPATH}/bin/ 
$ ./bfe -c ${GOPATH}/src/github.com/baidu/bfe/conf/
```

## Further reading

- Get started with [Beginner's Guide](../example/guide.md)

