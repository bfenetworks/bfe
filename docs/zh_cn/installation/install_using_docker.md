# Docker 安装

本章介绍如何通过 Docker 运行 BFE。

## 方式一：直接运行已有镜像

如果你已经有可用镜像（例如 `bfenetworks/bfe`，或你自己构建并推送到私有仓库的镜像），可以直接运行：

```bash
docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	<your-image>
```

你可以访问：
- http://127.0.0.1:8080/ （如果配置未命中，可能返回 500）
- http://127.0.0.1:8421/monitor （监控信息）

## 方式二：从源码构建镜像（推荐）

在仓库根目录执行：

```bash
# 一次构建 prod + debug 两个镜像
make docker

# 可选：指定 conf-agent 版本（默认 0.0.2）
make docker CONF_AGENT_VERSION=0.0.2
```

构建后的镜像标签（以 VERSION=1.8.0 为例）：
- `bfe:v1.8.0`（prod）
- `bfe:v1.8.0-debug`（debug）
- `bfe:latest`（始终指向 prod）

## 自定义配置（挂载本地目录）

镜像内目录约定：
- BFE 配置目录：`/home/work/bfe/conf`
- BFE 日志目录：`/home/work/bfe/log`
- conf-agent 配置目录：`/home/work/conf-agent/conf`
- conf-agent 日志目录：`/home/work/conf-agent/log`

示例：挂载你本地准备好的配置与日志目录（按需修改路径）：

```bash
# 事先准备好你自己的配置：
#   - /Users/BFE/Desktop/conf/         (BFE 配置目录)
#   - /Users/BFE/Desktop/conf-agent/   (conf-agent 配置目录，里面放 conf-agent.toml)
#   - /Users/BFE/Desktop/log/          (BFE 日志目录)

docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	-v /Users/BFE/Desktop/conf:/home/work/bfe/conf \
	-v /Users/BFE/Desktop/log:/home/work/bfe/log \
	-v /Users/BFE/Desktop/conf-agent:/home/work/conf-agent/conf \
	bfe:latest
```

## 下一步

* 了解[命令行参数](../operation/command.md)
* 了解[基本功能配置使用](../example/guide.md)
