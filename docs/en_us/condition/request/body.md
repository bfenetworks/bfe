# Condition Primitives Related to Request Body

## req_body_json_in(json_path, value_list, case_insensitive)

* Meaning: Searches for the field specified by `json_path` in the JSON-formatted request body and checks if its value exactly matches any in `value_list`.
* Parameters  

| Parameter        | Description                                    |
| ---------------- | ---------------------------------------------- |
| json_path        | String<br>The path to the JSON field in the request body |
| value_list       | String<br>List of values, separated by ‘&#124;’ |
| case_insensitive | Boolean<br>Whether to ignore case sensitivity   |

* Example

```go
req_body_json_in("model", "deepseek-r1|qwen-plus", true)
```
