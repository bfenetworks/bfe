# 简介 

基于地理信息字典，通过用户IP获取相关的地理信息。

# 配置

## 模块配置文件

  conf/mod_geo/mod_geo.conf

  ```
  [basic]
  GeoDBPath = mod_geo/geo.db
  ```

## 字典文件

  conf/mod_geo/geo.db  
  当前仅支持 MaxMind 地理信息字典，可以在 https://dev.maxmind.com/geoip/geoip2/geolite2/ 进行下载。
  
  