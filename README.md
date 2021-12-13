# BFE

English | [中文](README-CN.md)


[![GitHub](https://img.shields.io/github/license/bfenetworks/bfe)](https://github.com/bfenetworks/bfe/blob/develop/LICENSE)
[![Travis](https://img.shields.io/travis/com/bfenetworks/bfe)](https://travis-ci.com/bfenetworks/bfe)
[![Go Report Card](https://goreportcard.com/badge/github.com/bfenetworks/bfe)](https://goreportcard.com/report/github.com/bfenetworks/bfe)
[![GoDoc](https://godoc.org/github.com/bfenetworks/bfe?status.svg)](https://godoc.org/github.com/bfenetworks/bfe/bfe_module)
[![Snap Status](https://build.snapcraft.io/badge/bfenetworks/bfe.svg)](https://build.snapcraft.io/user/bfenetworks/bfe)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/3209/badge)](https://bestpractices.coreinfrastructure.org/projects/3209)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fbfenetworks%2Fbfe.svg?type=shield)](https://app.fossa.com/reports/1f05f9f0-ac3d-486e-8ba9-ad95dabd4768)
[![Slack Widget](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=green)](https://slack.cncf.io)

BFE (Beyond Front End) is a modern layer 7 load balancer from baidu.
![bfe logo](./docs/images/logo/horizontal/color/bfe-horizontal-color.png)

## Introduction

BFE opensource project includes several components, which can be used together as a integrated layer 7 load balancer and traffic management solution.

BFE system consists of data plane and control plane:

- Data plane：responsible for forwarding user's traffic, including below component: 
  - BFE Server：BFE forward engine (this repository, bfenetworks/bfe). BFE Server performs content based routing, load balancing and forwards the traffic to backend servers.
- Control plane：responsible for management and configuration of BFE system, including below components:
  - [API-Server](https://github.com/bfenetworks/api-server)：provides API and handles update, storage and generation of BFE config
  - [Conf-Agent](https://github.com/bfenetworks/conf-agent)：component for loading config, fetches latest config from API-Server and triggers BFE Server to reload it
  - [Dashboard](https://github.com/bfenetworks/dashboard)：provides a graphic interface for user to manage and view major config of BFE

Refer to [Overview](docs/en_us/introduction/overview.md) in BFE document for more information

Besides, we also implement [BFE Ingress Controller](https://github.com/bfenetworks/ingress-bfe) based on BFE, to fulfill Ingress in Kubernetes  

## Advantages

- Multiple protocols supported, including HTTP, HTTPS, SPDY, HTTP2, WebSocket, TLS, FastCGI, etc.
- Content based routing, support user-defined routing rule in advanced domain-specific language.
- Support multiple load balancing policies.
- Flexible plugin framework to extend functionality. Based on the framework, developer can add new features rapidly.
- Efficient, easy and centralized management, with RESTful API and Dashboard support.
- Detailed built-in metrics available for service status monitor.

## Getting Started
- Data plane: BFE Server [build and run](docs/en_us/installation/install_from_source.md)
- Control plane: English document coming soon.  [Chinese version](https://github.com/bfenetworks/api-server/blob/develop/docs/zh_cn/deploy.md) 

## Running the tests
- See [Build and run](docs/en_us/installation/install_from_source.md)

## Documentation
- [English version](https://www.bfe-networks.net/en_us/ABOUT/)
- [Chinese version](https://www.bfe-networks.net/zh_cn/ABOUT/)

## Contributing
- Please create an issue in [issue list](http://github.com/bfenetworks/bfe/issues).
- Contact Committers/Owners for further discussion if needed.
- Following the golang coding standards.
- See the [CONTRIBUTING](CONTRIBUTING.md) file for details.

## Authors
- Owners: [MAINTAINERS](MAINTAINERS.md)
- Contributors: [CONTRIBUTORS](CONTRIBUTORS.md)

## Communication
- BFE community on Slack: [Sign up](https://slack.cncf.io/) CNCF Slack and join bfe channel.
- BFE developer group on WeChat: [Send a request mail](mailto:iyangsj@gmail.com) with your WeChat ID and a contribution you've made to BFE(such as a PR/Issue). We will invite you right away.

## License
BFE is under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.
