# Request Tag Related Primtives

## req_tag_match(tagName, tagValue)

* Description: Judge if request tag matches configured value
    * Note: Tags may be set during request processing. For example, after the dictionary module processes the request, a clientIP tag with value "blocklist" may be set.

* Parameters

| Parameter | Description |
| --------- | ----------- |
| tagName | String<br>tag name |
| tagValue | String<br>tag value |

* Example

```go
req_tag_match("clientIP", "blocklist")
```
