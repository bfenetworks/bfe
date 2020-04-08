# Install from source code

## Prerequisite
- golang 1.13+
- git

## Download source code
```
git clone https://github.com/baidu/bfe
```

## Compile
- Execute the following command to build bfe:

```
$ cd bfe
$ make
```

- Execute the following command to run tests:

```
$ make test
```

- Executable object file location:

```
$ file output/bin/bfe

output/bin/bfe: ELF 64-bit LSB executable, ...
```

## Run

- Run BFE with example configuration files:

```
$ cd output/bin/
$ ./bfe -c ../conf -l ../log
```
