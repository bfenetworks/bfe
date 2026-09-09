# TC-08 gemini 上游 401 key 轮换兜底

## 用例编号与名称

TC-08 gemini 上游 401 key 轮换兜底

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：当前源码版本（2026-09-09 gemini 协议支持起）

## 测试目的

验证 gemini 集群多 key 场景下，上游对某个 key 返回 401（认证失败）时，BFE 将该 key 标记为死亡并在同一请求内轮换到其它 key 重试（`aiClusterInvoke` key 级兜底）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis。

## 前置条件

1. `cluster_gemini_multikey.AIConf`：`ModelProtocols=["gemini"]`，Key `goog-multi-a` / `goog-multi-b`（权重各 50），`KeyPolicy{Strategy:"weighted_random", MaxRetries:3}`。
2. mock 后端对 `x-goog-api-key: goog-multi-a` 返回 401，其余返回 200。

## BFE 请求

循环 POST `/v1beta/models/gemini-2.5-flash:generateContent`（Host `multikey.example.org`，`x-goog-api-key: ak_user_a`），直至上游观测到两个 key 均被选（加权随机）或达到 100 次上限。

## 预期结果

1. 所有请求最终响应 200（401 的 key 被轮换跳过）。
2. 上游观测到 `goog-multi-a` 与 `goog-multi-b` 均至少出现一次。
