# SNAP方式安装

## 环境准备

在Linux环境可以使用snap工具安装bfe。如果您的系统还未安装snap工具，参见[安装snap](https://snapcraft.io/docs/installing-snapd)

## 安装

- 执行如下命令:

```bash
$ sudo snap install --edge bfe
```

!!! tip
    配置文件位于/var/snap/bfe/common/conf/ <br>
    日志文件位于/var/snap/bfe/common/log/

## 运行

- 执行如下命令:

```bash
$ sudo /snap/bin/bfe 
```

## 下一步

* 了解[命令行参数](../operation/command.md)
* 了解[基本功能配置使用](../example/guide.md)
