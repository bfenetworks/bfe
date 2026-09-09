# TC-05 双头并存时 Authorization 优先

## 用例编号与名称

TC-05 双头并存时 Authorization 优先

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 `Authorization` 与 `x-goog-api-key` 并存时，按既有优先级识别为 openai（`DetectProtocolAndKey` 探测链：Authorization → x-api-key → x-goog-api-key），请求作为 openai 处理并向多协议集群注入 Bearer cluster key。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis。

## 前置条件

`cluster_both_protocols.AIConf`：`ModelProtocols=["openai","gemini"]`。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `both.example.org`，同时携带 `Authorization: Bearer ak_user_a` 与 `x-goog-api-key: ak_user_a`。

## 预期结果

1. 响应 200。
2. `cluster_both_protocols` 命中 1 次；上游 `Authorization` 为 `Bearer <cluster key>`（`sk-both-openai-key` 或 `goog-both-gemini-key` 均可，key 为加权随机选择）。
