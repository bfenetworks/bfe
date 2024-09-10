# mod_geo

## Introduction

mod_geo creates [variables](../mod_header/mod_header.md) with values depending on the client IP address, using the GEO databases.

## Module Configuration

### Description

conf/mod_geo/mod_geo.conf

| Config Item          | Description                                        |
| ---------------------| ------------------------------------------- |
| Basic.GeoDBPath      | String<br>Path of geo db file |
| Log.OpenDebug        | Boolean<br>Whether enable debug logs<br>Default False |

mod_geo supports GeoDB in MaxMind format which can be downloaded from
https://dev.maxmind.com/geoip/geoip2/geolite2/

### Example

```ini
[Basic]
GeoDBPath = mod_geo/geo.db
```
