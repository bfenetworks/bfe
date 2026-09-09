# TC-03 gemini 请求命中 openai-only 集群拒绝

## 用例编号与名称

TC-03 gemini 请求命中 openai-only 集群拒绝

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 gemini 风格请求被路由到仅支持 `openai` 协议的集群时，被 `AIConf.ModelProtocols` 一致性校验拒绝（400 `PROVIDER_PROTOCOL_MISMATCH`），不产生上游命中与配额扣减。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis。

## 前置条件

1. `cluster_openai_only.AIConf`：`ModelProtocols=["openai"]`。
2. `ai_route.data` 中 Host `openai.example.org` 路由到 `cluster_openai_only`。

## BFE 请求

POST `http://<bfe>/v1beta/models/gemini-2.5-flash:generateContent`，Host `openai.example.org`，头 `x-goog-api-key: ak_user_a`。

## 预期结果

1. 响应 400，响应体含 `PROVIDER_PROTOCOL_MISMATCH`。
2. `cluster_openai_only` 命中 0 次。
3. `quota:plan_token` 余额不变（拒绝请求不计费）。
