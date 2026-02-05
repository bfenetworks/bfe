# Docker 安装

本章介绍如何通过 Docker 运行 BFE。

## 方式一：直接运行已有镜像

如果你已经有可用镜像（例如 `ghcr.io/bfenetworks/bfe`，或你自己构建并推送到私有仓库的镜像），可以直接运行：

```bash
docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	<your-image>
```

示例：

```bash
docker run --rm \
  -p 8080:8080 -p 8443:8443 -p 8421:8421 \
  ghcr.io/bfenetworks/bfe:latest
```

你可以访问：
- http://127.0.0.1:8080/ （如果配置未命中，可能返回 500）
- http://127.0.0.1:8421/monitor （监控信息）

## 方式二：从源码构建镜像（推荐）

在仓库根目录执行：

```bash
# 一次构建 prod + debug 两个镜像
make docker

# 可选：指定镜像名（默认 bfe）
make docker BFE_IMAGE_NAME=bfe
```

说明：
- 镜像 tag 来自仓库根目录的 `VERSION` 文件，并会被规范化为以 `v` 开头（例如 `1.8.0` 会变成 `v1.8.0`）。
- `make docker` 是本地构建，不依赖 buildx。

构建后的镜像标签（以 VERSION=1.8.0 为例）：
- `bfe:v1.8.0`（prod）
- `bfe:v1.8.0-debug`（debug）
- `bfe:latest`（始终指向 prod）

## 方式三：构建并推送镜像到仓库（make docker-push）

当你需要将镜像提供给 Kubernetes 集群（或其它机器）拉取时，推荐使用 `make docker-push` 构建并推送多架构镜像（默认 `linux/amd64,linux/arm64`）。

前提条件：
- 你有可用的镜像仓库（例如 GHCR、Harbor、Docker Hub 等）
- 已完成 `docker login <registry>`
- 本地 Docker 支持 buildx（Docker Desktop 通常默认支持）

常用参数：
- `REGISTRY`：必填，镜像仓库前缀（如 `ghcr.io/your-org`）
- `BFE_IMAGE_NAME`：镜像名（默认 `bfe`，也可以是带路径的 `team/bfe`）
- `PLATFORMS`：构建平台（默认 `linux/amd64,linux/arm64`）

示例：推送到 GHCR（得到 `ghcr.io/cc14514/bfe:<tag>`）：

```bash
make docker-push REGISTRY=ghcr.io/cc14514 
```

示例：推送到私有仓库并限制平台（只构建 amd64）：

```bash
make docker-push \
	REGISTRY=registry.example.com \
	BFE_IMAGE_NAME=infra/bfe \
	PLATFORMS=linux/amd64
```

推送完成后镜像示例（以 VERSION=1.8.0 为例）：
- `$(REGISTRY)/$(BFE_IMAGE_NAME):v1.8.0`（prod，多架构）
- `$(REGISTRY)/$(BFE_IMAGE_NAME):v1.8.0-debug`（debug，多架构）
- `$(REGISTRY)/$(BFE_IMAGE_NAME):latest`（prod，多架构）

如果你要用 Kubernetes 示例部署并使用你推送的镜像：请到 `examples/kubernetes/kustomization.yaml` 的 `images:` 中替换 bfe 镜像的 `newName` / `newTag`。

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
