# mod_body_process 规则配置

## 配置简介

`body_process_rule.data` 是 `mod_body_process` 模块的规则配置文件，用于配置各产品线的请求/应答 body 处理流程。

## 配置描述

| 配置项                      | 类型    | 参数含义                   | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------------------- | ------- | -------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                     | String  | 配置文件版本               | Y    | -                                                            | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                      | Object  | 所有产品线的 body 处理规则配置 | Y    | -                                                            | -                                                            |
| Config{k}                   | String  | 产品线名称                 | Y    | -                                                            | -                                                            |
| Config{v}                   | Array   | 产品线下 body 处理规则列表 | Y    | -                                                            | -                                                            |
| Config{v}[]                 | Object  | body 处理规则              | Y    | -                                                            | -                                                            |
| Config{v}[].Cond            | String  | 匹配条件                   | Y    | 语法详见[Condition](../../condition/condition_grammar.md)    | -                                                            |
| Config{v}[].RequestProcess  | Object  | 请求 body 的处理流程配置   | N    | 数据结构见下                                                 | -                                                            |
| Config{v}[].ResponseProcess | Object  | 应答 body 的处理流程配置   | N    | 数据结构见下                                                 | -                                                            |

## body 处理流程配置

处理流程的数据结构如下：

```go
// 处理流程
struct {
    Dec  string     // decoder，不配置则使用缺省 decoder
    Enc  string     // encoder，不配置则使用缺省 encoder
    Proc []ProcConf // 处理器列表
}
// ProcConf
struct {
    Name   string   // 处理器名。目前只支持 "textfilter"
    Params []string // 处理器的参数表。textfilter: Params[0] - ToolGood.TextFilter 服务的 URL
}
```

当前支持的组件：

### decoder

- `line`：将数据按行解析，每一行作为一个事件
- `json`：从数据中解析 json 对象，每个 json 对象作为一个事件
- `sse`：从数据中解析 sse 事件
- 缺省：根据 contentType 自适应选择 decoder

### processor

- `textfilter`：调用 ToolGood.TextFilter 服务，对内容进行审查

### encoder

- 缺省：直接调用事件的 ToBytes() 函数生成 body 数据

## 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "!req_body_json_in(\"model\", \"\", false)",
                "RequestProcess": {
                    "Proc": [
                        {"name": "textfilter", "params": ["http://172.19.1.136:9191/api/"]}
                    ]
                },
                "ResponseProcess": {
                    "Proc": [
                        {"name": "textfilter", "params": ["http://172.19.1.136:9191/api/"]}
                    ]
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
