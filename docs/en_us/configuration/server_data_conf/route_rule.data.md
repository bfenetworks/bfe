# Route Rule Configuration

## Introduction

route_rule.data records route rule config for each product.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Time of generating config file | Y | See [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| ProductRule | Object | Route rules for each product | N | Key is product name, value is the route rule table | - |
| ProductRule{k} | String | Product name | Conditional | Required when ProductRule is non-empty | Non-empty |
| ProductRule{v} | []Object | An ordered list of route rules | Conditional | Required when ProductRule is non-empty; contains multiple ordered route rules | Non-empty |
| ProductRule{v}[] | Object | A route rule | Y | Contains Cond and ClusterName | Non-empty |
| ProductRule{v}[].Cond | String | Routing condition | Y | For condition syntax, see [Condition](../../condition/condition_grammar.md) | Non-empty; must be a valid BFE condition expression |
| ProductRule{v}[].ClusterName | String | Destination cluster | Y | Target cluster name for forwarding when matched | Non-empty |
| BasicRule | Object | Basic route rules (optional) | N | Static Host+Path based routing table, organized by product | - |
| BasicRule{k} | String | Product name | Conditional | Required when BasicRule is non-empty | Non-empty |
| BasicRule{v}[] | Object | Basic route rule | Conditional | Required when BasicRule is non-empty | Non-empty |
| BasicRule{v}[].Hostname | String | Host to match | Y | Supports wildcards | Non-empty |
| BasicRule{v}[].Path | String | Path to match | Y | URL path | Non-empty |
| BasicRule{v}[].ClusterName | String | Destination cluster | Y | Target cluster name for forwarding when matched | Non-empty |

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
