# 源码编译安装

## 环境准备

- golang 1.17+
- git 2.0+
- glibc-static 2.17+

## 源码下载

```bash
$ git clone https://github.com/bfenetworks/bfe
```

## 编译

- 执行如下命令编译:

```bash
$ cd bfe
$ make
```

!!! tip
    如果遇到超时错误"https fetch: Get ... connect: connection timed out", 请设置代理后重试，详见[安装常见问题](../faq/installation.md)

- 执行如下命令运行测试:

```bash
$ make test
```

- 可执行目标文件位置:

```bash
$ file output/bin/bfe
output/bin/bfe: ELF 64-bit LSB executable, ...
```

## 运行

- 基于示例配置运行BFE:

```bash
$ cd output/bin/
$ ./bfe -c ../conf -l ../log
```

## 下一步

* 了解[命令行参数](../operation/command.md)
* 了解[基本功能配置使用](../example/guide.md)
