# Install using "go get"

## Installation 
- Get the source code and install

```
$ go get github.com/baidu/bfe
```

- Executable object file location

```
$ file ${GOPATH}/bin/bfe

output/bin/bfe: ELF 64-bit LSB executable, ...
```

## Run
- Run BFE with example configuration files:

```
$ cd ${GOPATH}/bin/ 
$ ./bfe -c ${GOPATH}/src/github.com/baidu/bfe/conf/
```
