# mod_ai_intent questions 配置

## 配置简介

`intent_questions.data` 是 `mod_ai_intent` 模块的 questions 数据文件，定义意图
分类的问题集（问题名/类型/选项）与置信度门控阈值。路由规则中的
`req_ai_intent_in(<question_name>, ...)` 按问题名消费本文件配置的问题。
`Questions` 允许为空数组：空数组 = 停用意图分类软开关（BFE 加载后所有意图条件
不命中，流量走默认路由，不调用决策服务）。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| ------ | ---- | -------- | ---- | -------- | ---------- |
| Version | String | 配置文件版本 | Y | 内容任何变更（含仅调阈值）都必须更新；版本不变的热更视为无变更跳过 | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| MinConfidence | Float | 全局置信度门控阈值 | N | 默认值为 `0.6`；低于阈值的答案视为 unknown（路由条件不命中） | 取值范围 [0, 1]；须用自有标注数据重新标定 |
| Questions | Array | 问题列表 | Y | 单次分类调用并行评估全部问题；**空数组（0 个问题）= 停用意图分类软开关**：加载后所有意图条件不命中，流量走默认路由，不调用决策服务 | 0–255 项 |
| Questions[] | Object | 问题定义 | Y | - | - |
| Questions[].Name | String | 问题名 | Y | 全部问题中唯一；路由原语第一个参数按它索引答案 | 非空且唯一 |
| Questions[].Type | String | 问题类型 | Y | `choice`：从 Criteria 选项中选一项；`score`：在 Levels 刻度上打分（映射为档位名） | 取值范围为 `choice`、`score` |
| Questions[].Instructions | String | 判定指令 | Y | 发送给决策模型的指令 | 非空 |
| Questions[].Criteria | Object | 选项集合 | Type=`choice` 时必填 | `选项名 -> 选项描述`，与 `Levels` 互斥 | 1–255 项，选项名唯一，选项名不得包含 `\|` |
| Questions[].Levels | Array | 档位集合 | Type=`score` 时必填 | 有序（从低到高），元素为 `{"Name", "Description"}`，与 `Criteria` 互斥 | 1–255 档，Name 唯一 |
| Questions[].MinConfidence | Float | 该问题的门控阈值 | N | 覆盖文件级 `MinConfidence` | 取值范围 [0, 1] |

## 配置示例

```json
{
  "Version": "2026092601",
  "MinConfidence": 0.6,
  "Questions": [
    {
      "Name": "task_type",
      "Type": "choice",
      "Instructions": "这条请求属于哪类研发任务？",
      "Criteria": {
        "coding": "编写或修改代码、调试、重构、代码审查",
        "test_writing": "编写测试用例、单元测试、集成测试、补充断言",
        "doc_writing": "编写文档、README、注释、接口说明、使用示例"
      }
    },
    {
      "Name": "complexity",
      "Type": "score",
      "Instructions": "这个任务的复杂度如何？",
      "MinConfidence": 0.7,
      "Levels": [
        { "Name": "simple",  "Description": "单步即可完成" },
        { "Name": "medium",  "Description": "多步但模式常见" },
        { "Name": "complex", "Description": "需要深入推理或跨模块设计" }
      ]
    }
  ]
}
```
