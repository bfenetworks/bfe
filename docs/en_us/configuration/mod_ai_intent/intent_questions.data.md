# mod_ai_intent Questions Configuration

## Introduction

`intent_questions.data` is the questions data file of the `mod_ai_intent` module.
It defines the question set for intent classification (question name / type /
options) and the confidence gating thresholds. Routing rules consume the
questions configured here via `req_ai_intent_in(<question_name>, ...)`.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Configuration file version | Y | Must be updated on any content change (including threshold-only changes); a reload with an unchanged version is treated as no-op | Type is [Version](../00-common.md#5-version) |
| MinConfidence | Float | Global confidence gating threshold | N | Default `0.6`; answers below the threshold are treated as unknown (routing conditions do not match) | In range [0, 1]; must be re-calibrated with your own labeled data |
| Questions | Array | Question list | Y | All questions are evaluated in a single classification call | - |
| Questions[] | Object | Question definition | Y | - | - |
| Questions[].Name | String | Question name | Y | Unique across all questions; the first argument of the routing primitive addresses answers by it | Non-empty and unique |
| Questions[].Type | String | Question type | Y | `choice`: pick one option from Criteria; `score`: rate on the Levels scale (mapped to a level name) | One of `choice`, `score` |
| Questions[].Instructions | String | Decision instruction | Y | Instruction sent to the decision model | Non-empty |
| Questions[].Criteria | Object | Option set | Required when Type=`choice` | `option name -> option description`; mutually exclusive with `Levels` | 1-255 options, unique option names; option names must not contain `\|` |
| Questions[].Levels | Array | Level set | Required when Type=`score` | Ordered (low to high), elements are `{"Name", "Description"}`; mutually exclusive with `Criteria` | 1-255 levels, unique Names |
| Questions[].MinConfidence | Float | Gating threshold of this question | N | Overrides the file-level `MinConfidence` | In range [0, 1] |

## Configuration Example

```json
{
  "Version": "2026092601",
  "MinConfidence": 0.6,
  "Questions": [
    {
      "Name": "task_type",
      "Type": "choice",
      "Instructions": "Which kind of R&D task is this request?",
      "Criteria": {
        "coding": "writing or modifying code, debugging, refactoring, code review",
        "test_writing": "writing test cases, unit tests, integration tests, assertions",
        "doc_writing": "writing docs, README, comments, API descriptions, usage examples"
      }
    },
    {
      "Name": "complexity",
      "Type": "score",
      "Instructions": "How complex is this task?",
      "MinConfidence": 0.7,
      "Levels": [
        { "Name": "simple",  "Description": "done in a single step" },
        { "Name": "medium",  "Description": "multiple steps but common pattern" },
        { "Name": "complex", "Description": "requires deep reasoning or cross-module design" }
      ]
    }
  ]
}
```
