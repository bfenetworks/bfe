# Installation FAQ

## Go get timeout during installation
- Set GOPROXY enviromennt as follows (go1.13+):
```
$ go env -w GO111MODULE=on
$ go env -w GOPROXY=https://goproxy.cn,direct
```
- For more details, see [https://goproxy.cn] (https://goproxy.cn) or [https://goproxy.io] (https://goproxy.io)

## Whether support compilation on MAC/Windows or not 
- BFE version 0.7.0+ is supported

