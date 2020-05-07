# 源码编译安装

## 环境准备
- golang 1.13+
- git

## 源码下载
```
git clone https://github.com/baidu/bfe
```

## 编译
- 执行如下命令编译:

```
$ cd bfe
$ make
```

- 执行如下命令运行测试:

```
$ make test
```

- 可执行目标文件位置:

```
$ file output/bin/bfe

output/bin/bfe: ELF 64-bit LSB executable, ...
```

## 运行

- 基于示例配置运行BFE:

```
$ cd output/bin/
$ ./bfe -c ../conf -l ../log
```

## 下一步
了解[基本功能配置使用](../example/guide.md)
