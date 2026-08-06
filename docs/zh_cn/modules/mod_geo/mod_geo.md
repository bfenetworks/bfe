# mod_geo

## 模块简介

mod_geo基于地理信息字典，通过用户IP获取相关的地理信息。

## 基础配置

模块基础配置文件说明详见 [mod_geo.conf](../../configuration/mod_geo/mod_geo.conf.md)。

## 监控项

| 监控项                  | 描述                              |
| ----------------------- | --------------------------------- |
| ERR_GET_GEO_INFO | 通过地理信息字典查询用户地理位置信息时，失败的次数 |
| ERR_RELOAD_GEO_DATABASE | Reload 地理信息字典失败的次数 |
