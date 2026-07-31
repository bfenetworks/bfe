# Route Rule Configuration

## Introduction

route_rule.data records route rule config for each product.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Time of generating config file                               |
| ProductRule | Object<br>Route rules for each product                                 |
| ProductRule{k}        | String<br>Product name                                       |
| ProductRule{v}        | Object<br>A ordered list of route rules                      |
| ProductRule{v}[] | Object<br>A route rule                                       |
| ProductRule{v}[].Cond | String<br>Condition expression, see [Condition](../../condition/condition_grammar.md) |
| ProductRule{v}[].ClusterName | String<br>Destination cluster name                    |
| BasicRule | Object<br>Basic route rules (optional). Static host+path based routing table organized by product |
| BasicRule{k} | String<br>Product name                                       |
| BasicRule{v}[] | Object<br>Basic route rule                                   |
| BasicRule{v}[].Hostname | String<br>Host to match, supports wildcards                |
| BasicRule{v}[].Path | String<br>Path to match                                      |
| BasicRule{v}[].ClusterName | String<br>Destination cluster name                    |

## Example

```json
{
    "Version": "20190101000000",
    "ProductRule": {
        "example_product": [
            {
                "Cond": "req_host_in(\"example.org\")",
                "ClusterName": "cluster_example1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_example2"
            }
        ]
    }
}
```
