# Terminology

- Product
    - The product equals "tenant" in BFE, which has its own configuration, such as routing policy, permission, etc.
    - Field "host" in HTTP header is used to identify product that the incoming message belongs to.

- Cluster
    - Cluster means a set of backend servers which provide same functionality and process similar task.
    - Usually，cluster span multiple IDC/region in scope.

- Sub Cluster
    - A cluster may be composed of multiple sub clusters conceptually.
    - Usally, backend servers within one IDC/region is defined as a sub cluster.

- Instance
    - Instance is backend server. A sub cluster would contain multiple instances.
    - To BFE，each backend instance expose IP address + port to accept request.

