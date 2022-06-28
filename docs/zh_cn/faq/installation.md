# 安装常见问题

## 安装时遇到go get超时错误

- 设置GOPROXY环境变量(go1.15及以上版本)

```bash
$ go env -w GO111MODULE=on
$ go env -w GOPROXY=https://goproxy.cn,direct
```

- 详见[https://goproxy.cn](https://goproxy.cn)或[https://goproxy.io](https://goproxy.io)

## 是否支持在MAC/Windows环境编译

- BFE 0.7.0+版本已支持
