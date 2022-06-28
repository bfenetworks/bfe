# go方式安装

## 环境准备

* golang 1.15+

## 安装

- 获取并安装

```bash
$ go get github.com/bfenetworks/bfe
```

可执行目标文件位置: ${GOPATH}/bin/bfe

!!! tip
    如果遇到超时错误"https fetch: Get ... connect: connection timed out", 请设置代理后重试，详见[安装常见问题](../faq/installation.md)

## 运行

- 基于示例配置运行BFE:

```bash
$ cd ${GOPATH}/bin/ 
$ ./bfe -c ${GOPATH}/src/github.com/bfenetworks/bfe/conf/
```

## 下一步

* 了解[命令行参数](../operation/command.md)
* 了解[基本功能配置使用](../example/guide.md)
