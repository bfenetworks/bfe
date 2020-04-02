# Request Routing

In BFE configuration, a "product" could be composed of multiple clusters. User can define how requests are routed among the clusters. Request routing is based on the content of HTTP request.

## Rule configuration

- Fields in HTTP Header is used to define routing rule for distributing traffic in cluster level, for example:
    - host, path, query, cookie, method, etc.
- BFE provide a "Condition" expression to define how to route request with special header. This is routing rule in cluster level load balance.
- If multiple rules are configured, BFE would match the rules in sequence. Matching procedure stop if one rule is matched.

## Example

- A product "demo", which needs process three kinds of traffic: static content traffic, "post" traffic, other traffic, then we can define three clusters ：

    - demo-static：serve static content 
    - demo-post：serve post request
    - demo-main: serve other traffic

- To BFE configuration, following routing rules can be added:
    - Rule 1: req_path_prefix_in("/static", false) -> demo-static, which means requests with path prefixed with "/static" will be routed to cluster demo-static.
    - Rule 2: req_method_in("POST")&&req_path_prefix_in("/setting",false) -> demo-post, which means that request which use method "POST" and is prefixed with "/setting" will be routed to cluster "demo-post". 
    - Rule 3: default -> demo-main, which means all request which doesn't match above rules will be sent to cluster "demo-main".

