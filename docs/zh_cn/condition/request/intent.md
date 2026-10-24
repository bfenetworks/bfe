# 请求意图相关原语

依赖 `mod_ai_intent` 模块：意图分类由模块调用决策服务完成，原语求值时按需触发
（懒解析）。决策服务不可用、答案置信度低于阈值（unknown）或问题未配置时，
本类原语恒为 false。

## req_ai_intent_in(question_name, value_list, min_confidence)

* 含义： 判断 AI 意图分类结果中，问题 `question_name` 的答案是否匹配
  `value_list` 之一，且该问题答案的置信度不低于 `min_confidence`
* 参数

| 参数 | 描述 |
| ---- | ---- |
| question_name | String<br>问题名，对应 `intent_questions.data` 中 `Questions[].Name` |
| value_list | String<br>选项列表，多个之间使用'&#124;'连接；`choice` 问题填选项名，`score` 问题填档位名 |
| min_confidence | Float<br>可选尾参（可省略）。该问题答案的置信度门槛；省略时使用该问题的 `MinConfidence` 配置（"答案有效"）；显式 `0` 表示不做置信检查 |

* 前置条件： `mod_ai_intent` 模块已加载，`intent_questions.data` 中已配置该问题；
  客户端也可通过显式意图头（默认 `X-AI-Intent: <question>=<option>`）直接声明答案
* 示例

```go
req_ai_intent_in("task_type", "test_writing|doc_writing", 0.9)
```
