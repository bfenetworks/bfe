# TLS Session Ticket Key配置

## 配置简介

session_ticket_key.data配置记录了session ticket key信息。

## 配置描述

| 配置项           | 描述                                                           |
| ---------------- | -------------------------------------------------------------- |
| Version          | String<br>配置文件版本                                         |
| SessionTicketKey | String<br>Session Ticket密钥，仅包含字符a-z0-9且长度48的字符串 |

## 配置示例

```json
{
    "Version": "20190101000000",
    "SessionTicketKey": "08a0d852ef494143af613ef32d3c39314758885f7108e9ab021d55f422a454f7c9cd5a53978f48fa1063eadcdc06878f"
}
```
