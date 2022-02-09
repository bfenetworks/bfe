# 同类系统对比

下面将BFE和一些相关的系统进行对比。

注：由于相关项目在活跃开发，如下信息可能过期或有误，欢迎您在 https://github.com/bfenetworks/bfe/issues 反馈。

## BFE及相关系统的定位

在各开源系统的官网上，几个相关系统的定位描述如下：

+ BFE: BFE是一个开源的七层负载均衡系统。
+ [Nginx](http://nginx.org/en/): Nginx是HTTP服务、反向代理服务、邮件代理服务、通用TCP/UDP代理服务。
+ [Traefik](https://github.com/containous/traefik): Traefik是先进的HTTP反向代理和负载均衡。
+ [Envoy](https://www.envoyproxy.io/): Envoy是开源的边缘和服务代理，为云原生应用而设计。

## 功能对比

### 协议支持

+ 4个系统都支持HTTPS和HTTP/2

### 健康检查

+ BFE和Nginx只支持“被动”模式的健康检查。
+ Traefik只支持“主动”模式的健康检查。
+ Envoy支持主动、被动和混合模式的健康检查。

注：Nginx商业版支持“主动”模式的健康检查。

### 实例级别负载均衡

+ 4个系统都支持实例级别负载均衡

### 集群级别负载均衡

+ BFE、Traefik、Envoy都支持集群级别负载均衡
+ Nginx不支持集群级别负载均衡

注：Envoy基于全局及分布式负载均衡策略

### 对于转发规则的描述方式

+ BFE基于[条件表达式](../condition/condition_grammar.md)
+ Nginx基于正则表达式
+ Traefik支持基于请求内容的分流，但无法支持灵活的与或非逻辑
+ Envoy支持基于域名、Path及Header的转发规则

## 扩展开发能力

### 编程语言

+ BFE和Traefik都基于Go语言
+ Nginx使用C和Lua开发
+ Envoy使用C++开发

### 可插拔架构

+ 4个系统都使用了可插拔架构

### 新功能开发成本

由于编程语言方面的差异，BFE和Traefik的开发成本较低，Nginx和Envoy的开发成本较高。

### 异常处理能力

由于编程语言方面的差异，BFE和Traefik可以对异常（在Go语言中称为Panic）进行捕获处理，从而避免程序的异常结束; 而Nginx和Envoy无法对内存等方面的错误进行捕获，这些错误很容易导致程序崩溃。

## 可运维性

### 内部状态展示

+ BFE对程序内部状态，提供了[丰富的展示](../operation/monitor.md)
+ Nginx和Traefik提供的内部状态信息较少
+ Envoy也提供了丰富的内部状态展示

### 配置热加载

+ 4个系统都提供配置热加载功能
+ Nginx配置生效需重启进程，中断活跃长连接

注：Nginx商业版支持动态配置，在不重启进程的情况下热加载配置生效
