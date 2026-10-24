# TC-06 TTL 过期

## 用例编号与名称

TC-06 TTL 过期

## 所属场景

SC20 AI 缓存精确匹配

## 版本声明

- `bfe`：当前源码版本（含 `mod_ai_cache`）

## 测试目的

验证缓存项按规则 `cacheTTL` 过期：TTL 耗尽后相同问题不再命中，重新回源上游。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis（miniredis）已启动，支持时钟快进（`FastForward`）。
3. mock 后端 `cluster_cache` 已启动，返回 200 与非流式答案。
4. 临时 BFE 配置已生成并加载，包含 `mod_ai_cache` 规则：`cacheTTL=1`，其余同 TC-01。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body |
|------|------|------|---------------|------|
| 1 | `cache.example.org` | `/v1/chat/completions` | `Bearer ak_cache` | `{"model":"deepseek-chat","messages":[{"role":"user","content":"what is bfe?"}]}` |
| 2（Redis 时钟快进 2s 后） | 同上 | 同上 | 同上 | 同上 |

## 预期结果

- 步骤 1 返回 200，缓存项写入 Redis（TTL=1s）。
- Redis 时钟快进 2s 后，步骤 2 返回 200 但**不命中**（无 `X-Bfe-Ai-Cache: hit` 头），上游命中 2 次。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
