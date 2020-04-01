# 模块简介 

根据自定义条件，修改请求或响应的头部。

# 基础配置
## 配置描述
| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| DataPath            | String<br>规则配置的的文件路径 |
| OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |
## 配置示例
```
[basic]
DataPath = ../conf/mod_header/header_rule.data
```
# 规则配置
## 配置描述
| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的 Header 规则 |
| Config.{k} | String<br>产品线名称 |
| Config.{v} | Object<br>产品线下的 Header 规则列表 |
| Config.{v}[] | Object<br>Header 规则详细信息 |
| Config.{v}[].Cond | String<br>描述匹配请求或连接的条件 |
| Config.{v}[].Last | Boolean<br>如果规则条件匹配成功后，是否继续匹配下一条规则 |
| Config.{v}[].Actions | Object<br>匹配成功后的动作|
| Config.{v}[].Actions.Cmd | String<br>匹配成功后执行的指令 |
| Config.{v}[].Actions.Params | Object<br>执行指令的相关参数列表 |
| Config.{v}[].Actions.Params[] | String<br>参数信息 |
## 配置示例
```
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "cond": "req_path_prefix_in(\"/header\", false)",
                "actions": [
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Log-Id",
                            "%bfe_log_id"
                        ]
                    }，
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Vip",
                            "%bfe_vip"
                        ]
                    }，
                    {
                        "cmd": "RSP_HEADER_SET",
                        "params": [
                            "X-Proxied-By",
                            "bfe"
                        ]
                    }
                ],
                "last": true
            }
        ]
    }
}
```
# Header 动作说明

| 动作名称        | 含义       | 参数列表说明 |
| -------------- | ---------- | --------- |
| REQ_HEADER_SET | 设置请求头 | HeaderName, HeaderValue | 
| REQ_HEADER_ADD | 添加请求头 | HeaderName, HeaderValue |
| REQ_HEADER_DEL | 删除请求头 | HeaderName |
| RSP_HEADER_SET | 设置响应头 | HeaderName, HeaderValue |
| RSP_HEADER_ADD | 添加响应头 | HeaderName, HeaderValue |
| RSP_HEADER_DEL | 删除响应头 | HeaderName |
  
# 内置变量说明
可以通过 %variable 使用变量，参见下文示例  

| 变量名         | 含义       |
| -------------- | ---------- |
| bfe_client_ip | 客户端IP |
| bfe_client_port | 客户端端口 |
| bfe_request_host | 请求Host |
| bfe_session_id | 会话ID |
| bfe_log_id | 请求ID |
| bfe_cip | 客户端IP (CIP) |
| bfe_vip | 服务端IP (VIP) |
| bfe_server_name | 添加请求头 |
| bfe_cluster | 目的集群 |
| bfe_backend_info | 后端信息 |
| bfe_ssl_resume | 是否tls/ssl会话复用 |
| bfe_ssl_cipher | tls/ssl加密套件 |
| bfe_ssl_version | tls/ssl协议版本 |
| bfe_protocol | 访问协议 |
| client_cert_serial_number | 客户端证书序列号 |
| client_cert_subject_title | 客户端证书Subject title |
| client_cert_subject_common_name | 客户端证书Subject Common Name |
| client_cert_subject_organization | 客户端证书Subject Organization |

