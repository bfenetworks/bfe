# BFE

[![GitHub](https://img.shields.io/github/license/bfenetworks/bfe)](https://github.com/bfenetworks/bfe/blob/develop/LICENSE)
[![Travis](https://img.shields.io/travis/com/bfenetworks/bfe)](https://travis-ci.com/bfenetworks/bfe)
[![Go Report Card](https://goreportcard.com/badge/github.com/bfenetworks/bfe)](https://goreportcard.com/report/github.com/bfenetworks/bfe)
[![GoDoc](https://godoc.org/github.com/bfenetworks/bfe?status.svg)](https://godoc.org/github.com/bfenetworks/bfe/bfe_module)
[![Snap Status](https://snapcraft.io/bfe/badge.svg)](https://snapcraft.io/bfe)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/3209/badge)](https://bestpractices.coreinfrastructure.org/projects/3209)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fbfenetworks%2Fbfe.svg?type=shield)](https://app.fossa.com/reports/1f05f9f0-ac3d-486e-8ba9-ad95dabd4768)
[![Slack Widget](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=green)](https://slack.cncf.io)

[English](README.md) | 中文

BFE (Beyond Front End) 是百度开源的现代化、企业级的七层负载均衡系统

<img src="./docs/images/logo/horizontal/color/bfe-horizontal-color.png" alt="bfe logo" width="300" />

BFE 是 [云原生计算基金会 (Cloud Native Computing Foundation, CNCF)](https://cncf.io/) 的一个沙箱项目

![cncf-logo](./docs/images/cncf-logo.png)

## 简介

BFE开源项目包含多个组件，可以搭建完整的七层负载均衡和流量接入解决方案

BFE系统包括数据平面和控制平面：

- 数据平面：负责用户流量的转发，包含下列组件
  - BFE Server：BFE 核心转发引擎，即为本项目（bfenetworks/bfe）。BFE Server 将用户流量经过内容路由、负载均衡，最终转发给合适的后端业务集群
- 控制平面：负责BFE系统的配置和管理等，包含下列组件
  - [API-Server](https://github.com/bfenetworks/api-server)：对外提供 API 接口，完成 BFE 配置的变更、存储和生成
  - [Conf-Agent](https://github.com/bfenetworks/conf-agent)：配置加载组件，从API-Server获取最新配置，并触发 BFE Server 进行配置热加载
  - [Dashboard](https://github.com/bfenetworks/dashboard)：为用户提供了图形化操作界面，以对 BFE 的主要配置进行管理和查看

BFE的架构说明见[概览](docs/zh_cn/introduction/overview.md)文档

此外，我们也基于 BFE 实现了 [BFE Ingress Controller](https://github.com/bfenetworks/ingress-bfe)，用于支持在 Kubernetes 中使用 Ingress


## 🚀 快速开始

快速开始面向想尽快跑起来的用户：先构建 Docker 镜像，再用 Kubernetes 示例快速部署。

### 1）构建 Docker 镜像

在仓库根目录执行：

```bash
make docker
```

说明：
- `make docker` 会构建 prod + debug 两个镜像，镜像 tag 来自 `VERSION` 文件。
- 如需自定义镜像名，可通过 `BFE_IMAGE_NAME` 覆盖，例如：

```bash
make docker BFE_IMAGE_NAME=your-registry/bfe
```

如果你希望 Kubernetes 部署使用你本地构建的镜像：需要将镜像推送到集群节点可访问的镜像仓库（或在本地集群中加载镜像），并在 `examples/kubernetes/kustomization.yaml` 的 `images:` 中将 bfe 镜像替换为你的镜像地址与 tag。

Docker 镜像的构建与推送（包含 `make docker-push`）详见：
- [docs/zh_cn/installation/install_using_docker.md](docs/zh_cn/installation/install_using_docker.md)

### 2）使用 Kubernetes 示例快速部署（kustomize）

```bash
cd examples/kubernetes
kubectl apply -k .
kubectl apply -f whoami-deploy.yaml
```

更多部署细节（包含镜像统一替换/mirror、初始化说明、清理与 finalizers 排障等）请参考：
- [examples/kubernetes/README-CN.md](examples/kubernetes/README-CN.md)

## 特性及优点

- 丰富协议支持：支持HTTP、HTTPS、SPDY、HTTP/2、WebSocket、TLS、gRPC、FastCGI等
- 基于请求内容的路由：支持高级条件表达式定制转发规则，转发规则易于理解及维护
- 高级负载均衡：支持全局/分布式负载均衡，实现就近访问、跨可用区容灾及过载保护等
- 灵活的模块框架：支持高效率定制开发第三方扩展模块
- 高效易用的管理：支持转发集群配置集中管理，提供Dashboard和RESTful API
- 一流的可见性：提供丰富详尽的监控指标，提供各类日志供问题诊断、数据分析及可视化
[了解更多详情](https://www.bfe-networks.net/zh_cn/introduction/overview/)

## 开始使用

- 数据平面：BFE核心转发引擎的[编译及运行](docs/zh_cn/installation/install_from_source.md)
- 控制平面：请参考控制平面的[部署说明](https://github.com/bfenetworks/api-server/blob/develop/docs/zh_cn/deploy.md)
- Kubernetes 部署示例（kustomize）：[examples/kubernetes/README-CN.md](examples/kubernetes/README-CN.md)

## 运行测试

- 请参考[编译及运行](docs/zh_cn/installation/install_from_source.md)

## 文档

- [英文版](https://www.bfe-networks.net/en_us/ABOUT/)
- [中文版](https://www.bfe-networks.net/zh_cn/ABOUT/)

## 书籍

- [《深入理解BFE》](https://github.com/baidu/bfe-book) ：介绍网络接入的相关技术原理，说明BFE的设计思想，以及如何基于BFE搭建现代化的网络接入平台。现已开放全文阅读。

## 参与贡献

- 请首先在[issue列表](http://github.com/bfenetworks/bfe/issues)中创建一个issue
- 如有必要，请联系项目维护者/负责人进行进一步讨论
- 请遵循golang编程规范
- 详情请参阅[参与贡献指南](CONTRIBUTING.md)

## 作者

- 项目维护者: [MAINTAINERS](MAINTAINERS.md)
- 项目贡献者: [CONTRIBUTORS](CONTRIBUTORS.md)

## 社区交流

- [开源BFE用户论坛](https://github.com/bfenetworks/bfe/discussions)

- **开源BFE微信公众号**：扫码关注公众号“BFE开源项目”，及时获取项目最新信息和技术分享

  <table>
  <tr>
  <td><img src="./docs/images/qrcode_for_gh.jpg" width="100"></td>
  </tr>
  </table>

- **开源BFE用户微信群**：扫码加入，探讨和分享对BFE的建议、使用心得、疑问等

  <table>
  <tr>
  <td><img src="https://raw.githubusercontent.com/clarinette9/bfe-external-resource/main/wechatQRCode.png" width="100"></td>
  </tr>
  </table>

- **开源BFE开发者微信群**: [发送邮件](mailto:iyangsj@gmail.com)说明您的微信号及贡献(例如PR/Issue)，我们将及时邀请您加入

## 许可

BFE基于Apache 2.0许可证，详见[LICENSE](LICENSE)文件说明
