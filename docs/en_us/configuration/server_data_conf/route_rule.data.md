# Introduction

route_rule.data records route rule config for each product. 

# Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Time of generating config file                               |
| ProductRule | Struct<br>Route rules for each product. Key is product name, Value is a ordered list of route rules. Route rule include: <br>- Cond: condition expression<br>- ClusterName: destination cluster name |

# Example

```
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
