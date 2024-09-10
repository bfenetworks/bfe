# Request Tag Related Primtives

## req_tag_match(tagName, tagValue)

* Description: Judge if request tag matches configured value

* Parameters

| Parameter | Description |
| --------- | ----------- |
| tagName | String<br>tag name |
| tagValue | String<br>tag value |

* Example

```go
req_tag_match("clientIP", "blocklist")
```
