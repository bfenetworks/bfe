# Comparison to similar systems

Here comparison will be made between BFE and several similar system.

NOTE: Most of the projects below are under active development. Thus some of the information may become out of date. If that is the case please feedback to https://github.com/bfenetworks/bfe/issues.

## Briefs of BFE and similar systems

The brief descriptions of several systems are as follows:

+ BFE: BFE is an open-source layer 7 load balancer.
+ [Nginx](http://nginx.org/en/): nginx is an HTTP and reverse proxy server, a mail proxy server, and a generic TCP/UDP proxy server.
+ [Traefik](https://github.com/containous/traefik): Traefik is a modern HTTP reverse proxy and load balancer.
+ [Envoy](https://www.envoyproxy.io/): Envoy is an open source edge and service proxy, designed for cloud-native applications.

## Features

### Protocol Support

+ All 4 systems support HTTPS and HTTP/2.

### Health Check

+ BFE and Nginx only support "passive" health check.
+ Traefik only supports "active" health check.
+ Envoy supports active, passive and hybrid health check.

NOTE: Nginx Plus (i.e., the commercial version of nginx) supports "active" health check.

### Instance-Level Load Balancing

+ All 4 systems support instance-level load balancing.

### Cluster-Level Load Balancing

+ BFE, Traefik and Envoy support cluster-level load balancing.
+ Nginx doesn't have this feature.

NOTE: Envoy supports global and distributed load balancing.

### Forwarding Rules

+ BFE provides [Condition Expression](../condition/condition_grammar.md).
+ Nginx uses regular expression.
+ Traefik supports traffic classification based on request content. But it can't support flexible AND or OR logic.
+ Envoy supports rules based on Domain, Path and Header.

## Extensibility

### Language

+ Both BFE and Traefik are written in Golang
+ Nginx is written in C and Lua.
+ Envoy is written in C++.

### Pluggable

+ All 4 systems support pluggable architecture.

### Cost for New Features

Due to difference in language, cost for new features is lower for BFE and Traefik, while cost is higher for Nginx and Envoy.

### Resilience to exception

With recovery mechanism of Golang, Panic can be caught in BFE and Traefik. Both system are immune to sudden crash.

While Nginx and Envoy can do nothing with wrong memory usage. Debugging such a bug is very time-consuming.

## Maintenance

### Observability

+ BFE provides [rich internal status](../operation/monitor.md) for external observation.
+ Nginx and Traefik provide less internal status.
+ Envoy also provides quite a lot internal status.

### Hot-reload of configuration

+ All 4 systems support hot-reload of configuration.
+ In Nginx, process must be restarted for the configuration to take effect, while active connections are terminated.

NOTE: Nginx Plus supports hot-reload of configuration, with no process restart.
