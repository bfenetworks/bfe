# go get方式安装

## 安装 
- 获取并安装

```
$ go get github.com/baidu/bfe
```

- 可执行目标文件位置

```
$ file ${GOPATH}/bin/bfe

output/bin/bfe: ELF 64-bit LSB executable, ...
```

## 运行
- 基于示例配置运行BFE:

```
$ cd ${GOPATH}/bin/ 
$ ./bfe -c ${GOPATH}/src/github.com/baidu/bfe/conf/
```

## 下一步
了解[基本功能配置使用](../example/guide.md)
                                           
