# 条件原语名称的规范

条件原语名称会使用以下规范：

## 条件原语名称前缀

- 针对Request的原语，会以"**req_**"开头
    - 如：req_host_in()

- 针对Response的原语，会以"**res_**"开头
    - 如：res_code_in()

- 针对Session的原语，会以"**ses_**"开头
    - 如：ses_vip_in()

- 针对系统原语，会以"**bfe_**" 开头
    - 如：bfe_time_range()

## 条件原语中比较的动作名称

- **match**：精确匹配
    - 如：req_tag_match()
- **in**：值是否在某个集合中
    - 如：req_host_in()
- **prefix_in**：值的前缀是否在某个集合中
    - 如：req_path_prefix_in()
- **suffix_in**：值的后缀是否在某个集合中
    - 如：req_path_suffix_in()
- **key_exist**：是否存在指定的key
    - 如：req_query_key_exist()
- **value_in**：对给定的key，其value是否落在某个集合中
    - 如：req_query_key_exist()
- **value_prefix_in**：对给定的key，其value的前缀是否在某个集合中
    - 如：req_header_value_prefix_in()
- **value_suffix_in**：对给定的key，其value的后缀是否在某个集合中
    - 如：req_header_value_suffix_in()
- **range**：范围匹配
    - 如：req_cip_range()
- **regmatch**：正则匹配
    - 如：req_url_regmatch()
    - 注：这类条件原语不合理使用将明显影响性能，谨慎使用
- **contain**: 字符串包含匹配
    - 如：req_cookie_value_contain()
