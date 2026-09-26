# Request Intent Primitives

Depends on the `mod_ai_intent` module: intent classification is performed by the
module calling a decision service, and is triggered lazily on first evaluation of
these primitives. If the decision service is unavailable, the answer confidence is
below the threshold (unknown), or the question is not configured, these primitives
always evaluate to false.

## req_ai_intent_in(question_name, value_list, min_confidence)

* Meaning: checks whether the answer of `question_name` in the AI intent
  classification matches one of `value_list`, and the answer confidence is not
  lower than `min_confidence`
* Parameters

| Parameter | Description |
| --------- | ----------- |
| question_name | String<br>Question name, corresponding to `Questions[].Name` in `intent_questions.data` |
| value_list | String<br>Option list; multiple values are joined with '&#124;'. Use option names for `choice` questions and level names for `score` questions |
| min_confidence | Float<br>Optional trailing parameter (may be omitted). Confidence threshold of the answer; when omitted, the question's `MinConfidence` config is used ("answer valid"); an explicit `0` disables the confidence check |

* Prerequisites: the `mod_ai_intent` module is loaded and the question is configured
  in `intent_questions.data`; clients may also declare answers explicitly via the
  intent header (default `X-AI-Intent: <question>=<option>`)
* Example

```go
req_ai_intent_in("task_type", "test_writing|doc_writing", 0.9)
```
