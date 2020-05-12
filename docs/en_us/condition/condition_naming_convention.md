# Condition Naming Convention

## Name prefix of condition primitive:
- Name prefix of request related primitive is "**req_**"
  - e.g. **req_host_in()**
- Name prefix of response related primitive is "**res_**"
  - e.g. **res_code_in()**
- Name prefix of session related primitive is "**ses_**"
  - e.g. **ses_vip_in()**
- Name prefix of system related primitive is "**bfe_**"
  - e.g. **bfe_time_range**()

## Name of compare actions:
- **match**：exact match
  - In this situation, the only one parameter is given
  - eg. **req_tag_match()**
- **in**：if the value is in the configured set
  - eg. **req_host_in()**
- **prefix_in**：if the value prefix is in the configured set
  - eg. **req_path_prefix_in()**
- **suffix_in**：if the value suffix is in the configured set
  - eg. **req_path_suffix_in()**
- **key_exist**：if the key exists
  - eg. **req_query_key_exist()**
- **value_in**：for the configured key, judge if the value is in the configured value set
  - eg. **req_query_key_exist()**
- **value_prefix_in**：for the configured key, judge if the value prefix is in the configured set
  - eg. **req_header_value_prefix_in()**
- **value_suffix_in**：for the configured key, judge if the value suffix is in the configured set
  - eg. **req_header_value_suffix_in()**
- **range**: range match
  - eg. **req_cip_range()**
- **regmatch**：regular match
  - eg. **req_url_regmatch()**
- **contain**: string match
  - eg. **req_cookie_value_contain()**

