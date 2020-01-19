# 简介 

根据自定义条件，修改请求或响应的头部。

# 配置

## 模块配置文件

  conf/mod_header/mod_header.conf

  ```
  [basic]
  DataPath = ../conf/mod_header/header_rule.data
  ```

## 规则配置文件

  conf/mod_header/header_rule.data

  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Version | String | 配置文件版本                                                 |
  | Config  | Map&lt;String, Array&lt;HeaderRule&gt;&gt; | 各产品线的规则配置 |
  
### HeaderRule

  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Cond | String | 条件原语                                                 |
  | Actions  | Array&lt;Action&gt; | 执行动作列表 |

### Action

  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Cmd | String | 动作名称，详见下表                                                 |
  | Params  | Array&lt;String&gt; | 动作参数列表 |

## 内置动作说明

  | 动作名称        | 描述       | 参数列表说明 |
  | -------------- | ---------- | --------- |
  | REQ_HEADER_SET | 设置请求头 | HeaderName, HeaderValue | 
  | REQ_HEADER_ADD | 添加请求头 | HeaderName, HeaderValue |
  | REQ_HEADER_DEL | 删除请求头 | HeaderName |
  | RSP_HEADER_SET | 设置响应头 | HeaderName, HeaderValue |
  | RSP_HEADER_ADD | 添加响应头 | HeaderName, HeaderValue |
  | RSP_HEADER_DEL | 删除响应头 | HeaderName |
  
## 内置变量说明

可以通过 %variable 使用变量，参见下文示例

  | 变量名         | 描述       |
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
  
# 示例

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
