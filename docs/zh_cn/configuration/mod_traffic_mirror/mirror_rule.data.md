# mod_traffic_mirror 镜像规则配置

## 配置简介

`mirror_rule.data` 是 `mod_traffic_mirror` 模块的镜像规则文件，按 product 组织规则列表。请求处理时在 `HandleForward` 阶段（选中后端后、转发前）按 `req.Route.Product` 定位规则列表，再逐条匹配 condition，取第一条命中的规则；product 查不到时直接放行（与其他 AI 模块一致，不回退 global product）。

镜像语义：

- **每请求最多镜像一次**：AI fallback 重试会多次进入转发流程，但同一请求只镜像一次；
- **按比例采样**：每条规则可配 0-100 的百分比，按比例随机抽样，100 表示全量镜像（发布前集中验证窗口）；
- **镜像目标是 cluster_table 中的 cluster**：复用现有健康检查、负载均衡（平滑加权轮询）与后端配置；
- **镜像响应直接丢弃**：读空整个响应（SSE 至 `[DONE]`）后丢弃，读空过程解析 `usage` / `error` / `finish_reason` 供统计；
- **主链路零影响**：镜像构建、提交、发送、目标故障全程不影响主请求，所有跳过/失败路径均有计数。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 |
| ------ | ---- | -------- | ---- | -------- |
| Version | String | 规则版本号 | Y | - |
| Config | Map\<String, Array\> | product 到规则列表的映射 | Y | key 为 product 名，须与 `host_rule.data` 解析出的 product 一致 |
| cond | String | 规则匹配条件 | Y | 条件表达式语法见[条件原语](../condition/condition_grammar.md)；AI 语义条件复用 body 条件原语，如 `req_body_json_in("model", ...)`；**空字符串表示匹配该 product 内所有请求** |
| mirrorCluster | String | 镜像目标集群名 | Y | 须已在 `cluster_table.data` 中定义 |
| percentage | Integer | 镜像采样百分比 | N | 默认 100；取值 [0, 100]，按请求随机抽样 |
| removeHeaders | Array\<String\> | 需要剔除的敏感 Header | N | 建议剔除 `Authorization`、`Cookie`、`X-Api-Key` 等鉴权/会话头 |
| setHeaders | Map\<String, String\> | 需要注入的 Header | N | 模块始终额外注入 `X-Bfe-Mirror: true` 与 `X-Bfe-Logid` |
| bodyRewrites | Array | body JSON 字段改写 | N | 用于"生产走模型 A、镜像到部署模型 B 的集群做双跑验证"；**一期仅支持顶层 `model` 字段** |
| bodyRewrites[].path | String | GJSON PATH | Y | 一期仅允许 `model` |
| bodyRewrites[].value | String | 改写后的值 | Y | - |
| pathRewrite | String | 镜像请求路径改写 | N | 非空时替换整个请求路径（query 保留）；默认保持原路径 |

> 说明：
> - 构建镜像副本时自动剔除 hop-by-hop 头（Connection、Keep-Alive、Proxy-* 等）与 `removeHeaders` 黑名单，Content-Length 由客户端重算。
> - body 改写失败（如字段不存在、body 非 JSON）时降级为镜像原始 body，该路径单独计数（`skip_rewrite_total`）。
> - 请求体超过可访问缓冲上限（`GetBytes()` 不完整，默认 2MB、最高 8MB）或超过 `MaxMirrorBodyBytes` 时跳过镜像并计数（`skip_body_limit_total`）。
> - 镜像请求由模块内独立 HTTP 客户端发出，不经过 BFE 监听端口与模块管线，因此不触发鉴权、配额扣减、限流，也不计入任何客户账单。

## 配置示例

```json
{
  "Version": "1.0",
  "Config": {
    "default": [
      {
        "cond": "req_path_prefix_in(\"/v1/chat/completions\", true) && req_body_json_in(\"model\", \"gpt-4o\", false)",
        "mirrorCluster": "cluster_shadow_v2",
        "percentage": 10,
        "removeHeaders": ["Authorization", "Cookie", "X-Api-Key"],
        "setHeaders": {"X-Env": "shadow"},
        "bodyRewrites": [
          {"path": "model", "value": "deepseek-v3"}
        ]
      },
      {
        "cond": "",
        "mirrorCluster": "cluster_shadow_all",
        "percentage": 0
      }
    ]
  }
}
```

默认部署（`conf/mod_traffic_mirror/mirror_rule.data`）使用空规则集，模块加载后不做任何镜像，可在控制面按需下发规则。
