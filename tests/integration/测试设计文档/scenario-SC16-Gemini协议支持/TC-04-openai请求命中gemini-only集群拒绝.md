# TC-04 openai 请求命中 gemini-only 集群拒绝

## 用例编号与名称

TC-04 openai 请求命中 gemini-only 集群拒绝

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 OpenAI 风格请求（`Authorization: Bearer` + `/v1/chat/completions`）被路由到仅支持 `gemini` 协议的集群时，同样被协议一致性校验拒绝。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis。

## 前置条件

1. `cluster_gemini_only.AIConf`：`ModelProtocols=["gemini"]`。
2. Host `gemini.example.org` 路由到 `cluster_gemini_only`。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `gemini.example.org`，头 `Authorization: Bearer ak_user_a`，body `{"model":"gpt-4"}`。

## 预期结果

1. 响应 400，响应体含 `PROVIDER_PROTOCOL_MISMATCH`。
2. `cluster_gemini_only` 命中 0 次。
3. `quota:plan_token` 余额不变。
