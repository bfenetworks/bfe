# Install using go

## Prerequisites

- golang 1.15+

## Installation

- Get the source code and install

```bash
$ go get github.com/bfenetworks/bfe
```

Executable object file location is ${GOPATH}/bin/bfe

!!! tip
    If you encounter an error such as "https fetch: Get ... connect: connection timed out", please set the GOPROXY and try again. See [Installation FAQ](../faq/installation.md)

## Run

- Run BFE with example configuration files:

```bash
$ cd ${GOPATH}/bin/ 
$ ./bfe -c ${GOPATH}/src/github.com/bfenetworks/bfe/conf/
```

## Further reading

- Get familiar with [Command options](../operation/command.md)
- Get started with [Beginner's Guide](../example/guide.md)
