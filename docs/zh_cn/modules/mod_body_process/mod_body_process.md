# mod_body_process

## 模块简介

mod_body_process 提供了一个 body 的流式处理框架。在许多场景中，请求或应答的body是流式的，例如SSE。对于流式的数据，如果需要做某种处理的话，例如内容审查，我们不能整体缓存下来再处理，而只能是一边接收一边处理，实时地一块一块地处理。

在这个流式处理框架中，body数据将依次经过三个步骤：
* decoder    - 实时地将已接收到的数据解析为事件
* processors - 事件处理器序列。每个处理器都是将输入事件转化为输出事件，也可以报错从而终止处理流程。由decoder产生的事件将依次经过事件处理器序列的处理
* encoder    - 将事件重新编码为body数据

用户可能通过配置规则定制请求或应答body的处理流程。目前支持的各种组件：
### decoder
* line - 将数据按行解析，每一行作为一个事件
* json - 从数据中解析json对象，每个json对象作为一个事件
* sse  - 从数据中解析 sse 事件
* 缺省  - 根据contentType自适应选择decoder
### processor
* textfilter - 调用 ToolGood.TextFilter 服务，对内容进行审查
### encoder
* 缺省  - 直接调用事件的 ToBytes() 函数生成 body 数据

## 基础配置

### 配置描述

模块配置文件: conf/mod_body_process/mod_body_process.conf

| 配置项              | 描述                                        |
| ------------------- | ------------------------------------------- |
| Basic.ProductRulePath      | String<br>规则配置的文件路径 |
| Log.OpenDebug       | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[basic]
ProductRulePath = ../data/mod_body_process/body_process_rule.data

[log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>所有产品线的 api-key 鉴权规则配置 |
| Config{k} | String<br>产品线名称|
| Config{v} | Array<br> 产品线下 api-key 鉴权规则列表 |
| Config{v}[] | Object<br> api-key 鉴权规则 |
| Config{v}[].Cond | String<br>匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].RequestProcess | Object<br>请求body的处理流程配置,数据结构见下 |
| Config{v}[].ResponseProcess | Object<br>应答body的处理流程配置，数据结构见下 |

body的处理流程配置的数据结构：
```
// 处理流程
struct {
	Dec  string     // decoder，不配置则使用缺省decoder
	Enc  string     // encoder，不配置则使用缺省encoder
	Proc []ProcConf // 处理器列表
}
// ProcConf
struct {
	Name string     // 处理器名。目前只支持 “textfilter”
	Params []string // 处理器的参数表。textfilter: Params[0] - ToolGood.TextFilter 服务的URL
}
```

### 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "!req_body_json_in(\"model\", \"\", false)",
                "RequestProcess": {
                        "Proc": [ 
                                {"name":"textfilter", "params":["http://172.19.1.136:9191/api/"]} 
                        ]
                },
                "ResponseProcess": {
                        "Proc": [
                                {"name":"textfilter", "params":["http://172.19.1.136:9191/api/"]}
                        ]
                }
            }
        ]
    },
    Version": "20190101000000"
}
```
