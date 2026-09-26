# mod_ai_intent Basic Configuration

## Introduction

`mod_ai_intent.conf` is the basic configuration file of the `mod_ai_intent` module.
It specifies the decision service address, the questions data file path, and
cache/breaker parameters. `mod_ai_intent` implements semantic-routing intent
classification: it calls a decision service (System One protocol,
`POST /v1/systemone`) for AI requests and makes the answers available to routing
rules via the `req_ai_intent_in` condition. Classification is lazy (triggered
only when a rule references an intent condition). If the decision service is
unavailable or confidence is below the threshold, the intent is unknown, intent
conditions do not match, and traffic falls through to default rules; request
forwarding is never blocked.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DecisionServiceAddr | String | Decision service address (System One protocol) | Y | E.g. `http://127.0.0.1:8000` (Laya) | - |
| Basic.QuestionsPath | String | Path of the questions data file | Y | Hot-reloadable; bump Version on any change | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.TimeoutMs | Integer | Timeout of a single intent classification (ms) | N | Default `300`; use `2000` when connecting to a local CPU decision service (Laya) in dev | Greater than 0 |
| Basic.MaxStateChars | Integer | Truncation length (in characters) of the last user message sent for classification | N | Default `2000` | Greater than 0 |
| Basic.CacheSize | Integer | Maximum number of entries in the in-process LRU cache | N | Default `10000` | Greater than or equal to 0 |
| Basic.CacheTTLSeconds | Integer | TTL of cache entries (seconds) | N | Default `1800` | Greater than 0 |
| Basic.ExplicitIntentHeader | String | Name of the explicit intent request header | N | Default `X-AI-Intent`; clients declare intent explicitly (`<question>=<option>`), valid values are adopted directly and take precedence over model classification | - |
| Breaker.FailureThreshold | Integer | Breaker: consecutive failure/timeout threshold | N | Default `5`; while the breaker is open the intent is always unknown | Greater than 0 |
| Breaker.ProbeIntervalMs | Integer | Breaker: probe recovery interval (ms) | N | Default `5000` | Greater than 0 |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

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
