# TC-02 密文上游 Key 注入与调用

## 目的

验证 `cluster_conf.data` 的 `AIConf.Keys[].Key` 以 `enc$v1$` 密文落盘时：BFE 解密进
内存后向上游注入明文 key，上游调用成功；磁盘文件直读无密钥明文。

对应实现：`TestTC02_EncryptedUpstreamKeyInjection`（`scenario-SC26-encrypted-config-fields/`）。

## 前置条件

- keyring（keyID=1）已配置。
- `cluster_conf.data` 由测试生成：`AIConf.Keys[0].Key` 为密文，`Name=k1`、`Weight=100`；
  其余字段（`ModelProtocols=["openai"]` 等）与模板一致。

## 执行步骤

1. 启动 BFE。
2. 直读工作目录 `server_data_conf/cluster_conf.data`：
   - 断言 `Keys[0].Key` 为 `enc$v1$` 前缀且文件中不含明文 provider key；
   - 断言非敏感字段可读（`Name: k1`、`Weight: 100`）。
3. 携带合法下游 api-key 发送请求。

## 预期结果

- 第 1 步启动成功。
- 第 3 步返回 200，mock 后端断言收到的 `Authorization` 头为
  `Bearer <明文 provider key>`：解密后内存结构与明文配置一致，注入行为不变。

## 清理

- 停止 BFE、关闭 mock 后端。
