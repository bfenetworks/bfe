# Terminology

## Product

- The product equals "tenant" in BFE, which has its own configuration, such as forwarding policies, permission, etc.

## Cluster

- A Cluster means a set of backend servers which provide the same functionality. Multiple clusters can be defined within a product.
- Usually，a cluster may span multiple IDC.

## Sub Cluster

- A cluster may be composed of multiple sub clusters conceptually.
- Usually, backend servers within the same IDC are defined as a sub cluster.

## Instance

- A sub cluster contains multiple instances (i.e. backend servers).
- Each instance is identified by IP address and port.
