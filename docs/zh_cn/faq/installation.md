# 安装常见问题

## 安装时遇到go get超时错误
- 使用[goproxy.cn](https://goproxy.cn)或[goproxy.io](https://goproxy.io)代理，对于go1.13及以上版本：
```
$ go env -w GO111MODULE=on
$ go env -w GOPROXY=https://goproxy.cn,direct
```
- 具体见[https://goproxy.cn](https://goproxy.cn)、[https://goproxy.io](https://goproxy.io)

## 是否支持在MAC/Windows环境编译
- BFE 0.7.0+版本已支持

## 如何在OpenBSD环境下安装BFE
- 具体见[OpenBSD安装示例](https://github.com/baidu/bfe/blob/develop/docs/zh_cn/example/install_on_openbsd.md)