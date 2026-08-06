# mod_static MIME 配置

## 配置简介

`mime_type.data` 是 `mod_static` 模块的 MIME 类型映射配置文件。

## 配置描述

| 配置项    | 类型   | 参数含义                     | 必填 | 补充描述                                                     | 合法性条件                                           |
| --------- | ------ | ---------------------------- | ---- | ------------------------------------------------------------ | ---------------------------------------------------- |
| Version   | String | 配置文件版本                 | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config    | Object | 文件扩展名与 MIME 类型映射表 | Y    | -                                                            | -                                                    |
| Config[k] | String | 文件扩展名                   | Y    | 须以 `.` 开头                                                | -                                                    |
| Config[v] | String | MIME 类型                    | Y    | -                                                            | -                                                    |

## 配置示例

```json
{
    "Config": {
        ".avi": "video/x-msvideo",
        ".doc": "application/msword"
    },
    "Version": "20190101000000"
}
```
