# docker安装

## 安装 && 运行

- 基于示例配置运行BFE:

```bash
docker run -p 8080:8080 -p 8443:8443 -p 8421:8421 bfenetworks/bfe
```

你可以访问 http://127.0.0.1:8080/ 因为没有匹配的配置，将得到 status code 500
你可以访问 http://127.0.0.1:8421/ 查看监控信息

- 自定义配置文件路径

```bash
// 事先准备好你自己的配置放到 (可以参考 配置 章节) /Users/BFE/conf

docker run -p 8080:8080 -p 8443:8443 -p 8421:8421 -v /Users/BFE/Desktop/log:/bfe/log -v /Users/BFE/Desktop/conf:/bfe/conf bfenetworks/bfe
```

## 下一步

* 了解[命令行参数](../operation/command.md)
* 了解[基本功能配置使用](../example/guide.md)
