# BFE

[English](README.md) | 中文

[![GitHub](https://img.shields.io/github/license/bfenetworks/bfe)](https://github.com/bfenetworks/bfe/blob/develop/LICENSE)
[![Travis](https://img.shields.io/travis/com/bfenetworks/bfe)](https://travis-ci.com/bfenetworks/bfe)
[![Go Report Card](https://goreportcard.com/badge/github.com/bfenetworks/bfe)](https://goreportcard.com/report/github.com/bfenetworks/bfe)
[![GoDoc](https://godoc.org/github.com/bfenetworks/bfe?status.svg)](https://godoc.org/github.com/bfenetworks/bfe/bfe_module)
[![Snap Status](https://build.snapcraft.io/badge/bfenetworks/bfe.svg)](https://build.snapcraft.io/user/bfenetworks/bfe)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/3209/badge)](https://bestpractices.coreinfrastructure.org/projects/3209)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fbfenetworks%2Fbfe.svg?type=shield)](https://app.fossa.com/reports/1f05f9f0-ac3d-486e-8ba9-ad95dabd4768)
[![CLA assistant](https://cla-assistant.io/readme/badge/bfenetworks/bfe)](https://cla-assistant.io/bfenetworks/bfe)
[![Slack Widget](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=green)](https://slack.cncf.io)

BFE是百度开源的现代化七层负载均衡系统

## 特性及优点
- 丰富协议支持：支持HTTP、HTTPS、SPDY、HTTP/2、WebSocket、TLS、gRPC、FastCGI等
- 基于请求内容的路由：支持高级条件表达式定制转发规则，转发规则易于理解及维护
- 高级负载均衡：支持全局/分布式负载均衡，实现就近访问、跨可用区容灾及过载保护等
- 灵活的模块框架：支持高效率定制开发第三方扩展模块
- 一流的可见性：提供丰富详尽的监控指标，提供各类日志供问题诊断、数据分析及可视化
[了解更多详情](https://www.bfe-networks.net/zh_cn/introduction/overview/)

## 开始使用
- [编译及运行](docs/zh_cn/installation/install_from_source.md)

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
  <td><img src="https://bfeopensource.bj.bcebos.com/wechatQRCode.png" width="100"></td>
  </tr>
  </table>

- **开源BFE开发者微信群**: [发送邮件](mailto:yangsijie@baidu.com)说明您的微信号及贡献(例如PR/Issue)，我们将及时邀请您加入

## 许可
BFE基于Apache 2.0许可证，详见[LICENSE](LICENSE)文件说明
