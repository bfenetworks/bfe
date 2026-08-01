# mod_geo Basic Configuration

## Introduction

`mod_geo.conf` is the basic configuration file of `mod_geo`, used to specify the path of the geo database file.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.GeoDBPath | String | Path of geo database file | Y | Currently only MaxMind geo database is supported, which can be downloaded from [MaxMind GeoLite2](https://dev.maxmind.com/geoip/geoip2/geolite2/); see [FilePath](../00-common.md#3-filepath) type definition | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug log | N | Defaults to `False` | - |

## Example

```ini
[Basic]
GeoDBPath = mod_geo/geo.db
```
