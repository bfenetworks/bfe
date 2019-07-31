# IP地址相关

请求clientip相关的条件原语

- **req_cip_range("startIP", "endIP")**
  - 判断请求的clientip是否在 [startIP, endIP] 的区间内
  - 在区间内，返回true，不在区间内，返回false 
  - startIP格式:  "202.196.64.1"
  - 暂只支持IPv4 格式地址
- **req_cip_dictmatch(dictfile)**
  - 判断clientip是否在词表中
  - dictfile，词表的文件名
  - 注：目前暂不支持
- **req_cip_trusted()**
  - 判断clientip是否为trust ip
- **req_cip_hash_in(patterns)**
  - 对cip哈希取模，判断是否匹配patterns之一（模值0～9999）
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接（如 “0-100|1000-1100|9999”）

请求vip相关的条件原语

- **req_vip_in("vip1|vip2|vip3")**
  - 判断访问VIP是否在指定VIP列表中
  - 在列表内，返回true，不在列表内，返回false 
  - 同时支持IPv4 、IPV6格式地址
- **req_vip_range("startVIP", "endVIP")**
  - 判断访问VIP是否在指定 [startIP, endIP] 的区间内
  - 在区间内，返回true，不在区间内，返回false 
  - 同时支持IPv4 、IPV6格式地址
