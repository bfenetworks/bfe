# mod_geo 基础配置

## 配置简介

`mod_geo.conf` 是 `mod_geo` 模块的基础配置文件，用于指定地理信息字典路径等。

## 配置描述

| 配置项          | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.GeoDBPath | String  | 地理信息字典的文件路径   | Y    | 当前仅支持 MaxMind 地理信息字典，可在 [MaxMind GeoLite2](https://dev.maxmind.com/geoip/geoip2/geolite2/) 下载 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug   | Boolean | 是否开启 debug 日志      | N    | 默认值`False`                                                | -                                                            |

## 配置示例

```ini
[Basic]
GeoDBPath = mod_geo/geo.db
```
