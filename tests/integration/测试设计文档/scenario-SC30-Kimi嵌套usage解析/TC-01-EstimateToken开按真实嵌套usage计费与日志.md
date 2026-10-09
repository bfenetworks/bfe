# TC-01 EstimateToken 开：按真实嵌套 usage 计费与日志（issue #1401）

## 1. 测试目的

验证 `EstimateToken=true` 下，Kimi 形态 OpenAI 流式（上游忽略注入的 `stream_options.include_usage`，finish chunk 内嵌 `choices[0].usage`）经 parser 回退后：

- 计费按**真实 usage**（77207/168/77375）而非估算伪造值（prompt=请求体/4、completion=chunk/4）；
- 访问日志分项 = 真实值，`ai_cache_read_tokens=73472`（`cached_tokens` 映射）；
- 注入缓解仍生效（backend 收包带 `include_usage=true`），嵌套 chunk 原样透传客户端。

## 2. 前置条件

- BFE 已启动并加载 `cluster_nested_usage`，`EstimateToken = true`、`InjectStreamUsage = true`，加载 `mod_access_pb3`。
- mock 后端固定回放：内容 chunk → 嵌套 finish chunk → `[DONE]`（忽略收包中的 `stream_options`）。
- Redis `quota:plan_rmb` 初始余额 10000000000。

## 3. 测试步骤

1. 客户端 `POST /v1/chat/completions`，`stream:true`，不带 `stream_options`，模型 `gpt-4`，Bearer `ak_user_a`，Host `nested-usage.example.org`。
2. 完整读取 SSE 流至连接关闭。

## 4. 预期结果

1. 响应 200，SSE 流依次包含内容 chunk、嵌套 finish chunk（含 `"total_tokens":77375"`）、`[DONE]`——嵌套 chunk 逐字节透传。
2. mock 后端收包体 `stream_options.include_usage == true`（注入生效；上游忽略之并仍回嵌套形态——facet-2 触发条件成立）。
3. Redis 余额 = 10000000000 − 23313300（77207×300 + 168×900，定点 1 单位=1e-8 元）。
4. pb3 访问日志末条：`ai_input_tokens=77207`、`ai_output_tokens=168`、`ai_total_tokens=77375`、`ai_cache_read_tokens=73472`。

## 5. 修复前行为（判别说明）

回退未生效时：嵌套 usage 解析全 0 → `isguess=true` → 估算路径生效——日志/计费落估算伪造值（prompt=请求体字节/4≈25、completion=(内容 chunk+`[DONE]`)/4），`ai_input_tokens≠77207` 断言即失败；cache 分项为 0。
