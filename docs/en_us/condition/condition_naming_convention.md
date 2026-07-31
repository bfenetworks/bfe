# Naming convention of condition primitives

BFE adopts the following naming convention for condition primitives.

## Name prefix of condition primitives

- Name prefix of the request primitive is "**req_**"
    - e.g. req_host_in()

- Name prefix of the response primitive is "**res_**"
    - e.g. res_code_in()

- Name prefix of the session primitive is "**ses_**"
    - e.g. ses_vip_range()

- Name prefix of the system primitive is "**bfe_**"
    - e.g. bfe_time_range()

## Name of comparison operations

- **match**: exact match
    - eg. req_tag_match()

- **in**: whether an element exists in a set or not
    - eg. req_host_in()

- **prefix_in**: whether the prefix exists in a set or not
    - eg. req_path_prefix_in()

- **suffix_in**: whether the suffix exists in a set or not
    - eg. req_path_suffix_in()

- **key_exist**: whether the specified key exists or not
    - eg. req_query_exist() (checks whether the whole query string is non-empty, not whether a specific key exists)

- **value_in**: whether the value exists in a set or not
    - eg. req_query_value_in()

- **value_prefix_in**: whether the value prefix exists in a set or not
    - eg. req_header_value_prefix_in()

- **value_suffix_in**: whether the value suffix exists in a set or not
    - eg. req_header_value_suffix_in()

- **value_contain**: whether the value contains a substring
    - eg. req_query_value_contain()

- **value_regmatch**: whether the value matches a regular expression
    - eg. req_query_value_regmatch()

- **hash_in**: hash-modulo match
    - eg. req_cip_hash_in()

- **range**: range match
    - eg. req_cip_range()

- **regmatch**: use regular expression to match
    - eg. req_url_regmatch()
    - Warning:  Inappropriate use can significantly affect performance

- **contain**: string match
    - eg. req_cookie_value_contain()

- **element_prefix_in**: path element prefix match
    - eg. req_path_element_prefix_in()

- **tag_match**: tag match
    - eg. req_tag_match()
