# 流量抓包分析

利用抓包及分析工具定位分析复杂的网络问题

## 流量抓取

使用tcpdump抓取示例：

```bash
# tcpdump tcp port 8443 -i any -s -w test.pcap
```

## 流量分析

### 明文情况

可使用wireshark软件打开抓包文件分析

### 密文情况

对于基于TLS的加密流量，可配合使用mod_key_log和wireshark进行解密分析。操作步骤:

* Step1: 在bfe开启mod_key_log模块，保存TLS会话密钥到key.log日志文件中
  * 注：修改bfe.conf文件，增加启用mod_key_log模块, 模块配置详见[mod_key_log](../modules/mod_key_log/mod_key_log.md)

```ini
[Server]
Modules = mod_key_log
```

* Step2: 在wireshark中设置Master-Secret日志文件路径为key.log
  * 注：配置路径Edit→Preferences→Protocols→SSL→(Pre)-Master-Secret log filename

* Step3: 使用wireshark打开并解密抓包数据

![WireShark解密https](../../images/wireshark-https.png)
