# mod_ai_intent 基础配置

## 配置简介

`mod_ai_intent.conf` 是 `mod_ai_intent` 模块的基础配置文件，用于指定决策服务地址、
questions 数据文件路径及缓存/熔断等参数。`mod_ai_intent` 实现语义路由的意图分类：
对 AI 请求调用决策服务（System One 协议 `POST /v1/systemone`）得到意图答案，
供路由规则通过 `req_ai_intent_in` 条件消费。意图分类为懒解析（规则引用意图条件
时才触发），决策服务不可用或置信度不足时意图为 unknown，路由规则不命中并回落
到默认规则，不影响主请求转发。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| ------ | ---- | -------- | ---- | -------- | ---------- |
| Basic.DecisionServiceAddr | String | 决策服务地址（System One 协议） | Y | 例：`http://127.0.0.1:8000`（Laya） | - |
| Basic.QuestionsPath | String | questions 数据文件路径 | Y | 支持热加载，变更需 bump Version | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.TimeoutMs | Integer | 单次意图分类超时（毫秒） | N | 默认值为 `300`；连接本机 CPU 决策服务（Laya）联调时建议 `2000` | 大于 0 |
| Basic.MaxStateChars | Integer | 参与分类的 last user message 截断长度（字符） | N | 默认值为 `2000` | 大于 0 |
| Basic.CacheSize | Integer | 进程内 LRU 缓存最大条目数（条） | N | 默认值为 `10000` | 大于等于 0 |
| Basic.CacheTTLSeconds | Integer | 缓存条目 TTL（秒） | N | 默认值为 `1800` | 大于 0 |
| Basic.ExplicitIntentHeader | String | 显式意图请求头名称 | N | 默认值为 `X-AI-Intent`；客户端显式声明意图（`<question>=<option>`），合法值直接采信且优先于模型分类 | - |
| Breaker.FailureThreshold | Integer | 熔断：连续失败/超时阈值 | N | 默认值为 `5`，熔断期间意图一律为 unknown | 大于 0 |
| Breaker.ProbeIntervalMs | Integer | 熔断：探测恢复间隔（毫秒） | N | 默认值为 `5000` | 大于 0 |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | 默认值为 `False` | - |

## 配置示例

```ini
[basic]
DecisionServiceAddr = http://127.0.0.1:8000
QuestionsPath = mod_ai_intent/intent_questions.data
TimeoutMs = 300
MaxStateChars = 2000
CacheSize = 10000
CacheTTLSeconds = 1800
ExplicitIntentHeader = X-AI-Intent

[breaker]
FailureThreshold = 5
ProbeIntervalMs = 5000

[log]
OpenDebug = false
```
