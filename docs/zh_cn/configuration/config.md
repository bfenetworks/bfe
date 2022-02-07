# 配置概述

## 说明

本文档是关于对 BFE Server 进行配置的概述。

在 [BFE API Server](https://github.com/bfenetworks/api-server) 和 [BFE Dashboard](https://github.com/bfenetworks/dashboard) 的文档中，说明了如何通过 BFE 控制面来进行 BFE 的配置。

## BFE配置分类

- 常规配置：在运行期间修改，需重启生效。
- 动态配置：在运行期间修改，热加载生效。

## BFE配置格式

- 常规配置：一般基于INI格式
- 动态配置：一般基于JSON格式 (注：特殊的证书、字典文件等例外)

## BFE配置组织

BFE的核心配置是bfe.conf (conf/bfe.conf)，为便于维护, 配置按功能分类存放在相应目录 conf/&#60;feature&#62;/

| 功能类别     | 文件位置 |
| ------------ | -------- |
| 服务基础配置 | conf/bfe.conf |
| 接入协议配置 | conf/tls_conf/ 目录 |
| 流量路由配置 | conf/server_data_conf/ 目录 |
| 负载均衡配置 | conf/cluster_conf/ 目录 |
| 扩展模块配置 | conf/mod_&#60;name&#62; 目录 |

## BFE配置热加载

详见[配置热加载](../operation/reload.md)
