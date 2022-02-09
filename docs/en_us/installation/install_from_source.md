# Install from source code

## Prerequisites

- golang 1.17+
- git 2.0+
- glibc-static 2.17+

## Download source code

```bash
$ git clone https://github.com/bfenetworks/bfe
```

## Build

- Execute the following command to build bfe:

```bash
$ cd bfe
$ make
```

!!! tip
    If you encounter an error such as "https fetch: Get ... connect: connection timed out", please set the GOPROXY and try again. See [Installation FAQ](../faq/installation.md)

- Execute the following command to run tests:

```bash
$ make test
```

- Executable object file location:

```bash
$ file output/bin/bfe
output/bin/bfe: ELF 64-bit LSB executable, ...
```

## Run

- Run BFE with example configuration files:

```bash
$ cd output/bin/
$ ./bfe -c ../conf -l ../log
```

## Further reading

- Get familiar with [Command options](../operation/command.md)
- Get started with [Beginner's Guide](../example/guide.md)
