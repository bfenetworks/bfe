# Route Rule Configuration

## Introduction

route_rule.data records route rule config for each product.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Time of generating config file                               |
| ProductRule | Struct<br>Route rules for each product                                 |
| ProductRule{k}        | String<br>Product name                                       |
| ProductRule{v}        | Struct<br>A ordered list of route rules                      |
| ProductRule{v}[].Cond | String<br>Condition expression                               |
| ProductRule{v}[].ClusterName | String<br>Destination cluster name                    |

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
