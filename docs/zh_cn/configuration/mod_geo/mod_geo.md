# 模块简介 

基于地理信息字典，通过用户IP获取相关的地理信息。

# 基础配置

## 配置描述
模块配置文件: conf/mod_geo/mod_geo.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.GeoDBPath            | String<br>地理信息字典的文件路径 |
| Log.OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |

字典文件说明：当前仅支持 MaxMind 地理信息字典, 可在 https://dev.maxmind.com/geoip/geoip2/geolite2/ 下载

## 配置示例
```
[basic]
GeoDBPath = mod_geo/geo.db
```
