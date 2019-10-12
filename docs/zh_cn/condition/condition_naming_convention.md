# 条件原语名称的规范

在条件原语名称的定义中，会使用以下规范：

- 条件原语前缀:
  - 针对Request的原语，会以”**req_**“开头
    - 如：**req_host_in()**
  - 针对Response的原语，会以”**res_**“开头
    - 如：**res_code_in()**
  - 针对Session的原语，会以"**ses_**"开头
    - 如：**ses_vip_in()**
  - 针对系统原语，会以“**bfe_**" 开头
    - 如：**bfe_time_range**()

- 条件原语前缀比较的“动作”名称:
  - **match**：精确匹配
    - 这种情况下，参数中会提供唯一的一个值供比较
  - **in**：值是否在某个集合中
    - 只要集合中存在这个值，就可以
  - **prefix_in**：值的前缀是否在某个集合中
  - **suffix_in**：值的后缀是否在某个集合中
  - **key_exist**：是否存在指定的key
    - 一般用于query、cookie、header的比较
  - **value_in**：对给定的key，其value是否落在某个集合中
  - **value_prefix_in**：对给定的key，其value的前缀是否落在某个集合中
  - **value_suffix_in**：对给定的key，其value的后缀是否落在某个集合中
  - **range**:范围匹配
    - 一般用于ip、time的比较
  - **regmatch**：正则匹配
    - 这种方式不鼓励
  - **dictmatch**：和字典的内容匹配
  - **contain**: 包括字符串
