# Installation FAQ

## Go get timeout during installation
- Set GOPROXY enviroment variable as follows (go1.17+):
```bash
$ go env -w GO111MODULE=on
$ go env -w GOPROXY=https://goproxy.cn,direct
```
- For more details, see [https://goproxy.cn](https://goproxy.cn) or [https://goproxy.io](https://goproxy.io)

## Whether compilation on MAC/Windows OS is supported or not ?
- It is supported since BFE v0.7.0 

