# TC-17 Anthropic 非流式 Content-Length 计费（跨协议回退，issue #1364）

## 用例编号与名称

TC-17 Anthropic 非流式 Content-Length 计费（Bearer 请求 + Anthropic 响应，带 Content-Length）

## 所属场景

SC03 RMB 配额扣减

## 版本声明

- `bfe`：当前源码版本（含 issue #1364 修复）

## 测试目的

验证当请求以 openai 风格（`Bearer ak_user_a` + `/v1/chat/completions`）发出、后端实际返回
Anthropic 协议 body（`type:"message"`、`input_tokens`/`output_tokens`），且响应带
`Content-Length` 头时，BFE 在 `tokenReadResponseHandler` 路径上通过跨协议回退识别 usage，
并按完整公式扣减 RMB 配额。

修复前（issue #1364），该形态下 openai 单协议适配器解析不到任何字段，未做跨协议回退，
最终扣减为 0（usage 被清零、也无估算补偿）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动，并预置 `quota:plan_rmb = 10000000000`。
3. mock 后端 `cluster_rmb` 已启动，返回 200、`Content-Type: application/json`，body 为：
   ```json
   {
       "id": "msg_01",
       "type": "message",
       "role": "assistant",
       "content": [{"type": "text", "text": "hi"}],
       "usage": {
           "input_tokens": 2000,
           "output_tokens": 1500,
           "cache_read_input_tokens": 8000
       }
   }
   ```
   （Go net/http 会对短 body 自动补 `Content-Length`，无需额外设置。）
4. 临时 BFE 配置已加载，`cluster_rmb` 的 `ModelTable` 包含模型 `claude-sonnet-4-5`，价格：
   - `input_cost_per_token`: `0.000001` → 定点整数 `100`
   - `output_cost_per_token`: `0.000002` → 定点整数 `200`
   - `cache_read_input_token_cost`: `0.0000005` → 定点整数 `50`
   - `cache_creation_input_token_cost`: `0.0000015` → 定点整数 `150`
5. `ak_user_a` 绑定 RMB 配额计划 `plan_rmb`。

## 配置构造

- `cluster_rmb.AIConf.ModelTable.Models` 增加模型 `claude-sonnet-4-5`（价格见前置条件 4）。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `rmb.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_user_a` |
| Body | `{"model":"claude-sonnet-4-5"}` |

> 注意：请求是 openai 形态（Bearer key、`/v1/chat/completions`），响应是 Anthropic 形态，
> 二者协议错配是本用例的核心设定。

## 预期结果

- 响应状态码：200。
- `cluster_rmb` 收到 1 次命中（200）。
- Redis 中 `quota:plan_rmb` 的余额按完整公式扣减：
  - Anthropic 的 `input_tokens` 不含 cache 命中部分，故
    normal_input = 2000 + 8000 - 8000 = 2000
  - 扣减金额 = 2000 * 100 + 8000 * 50 + 1500 * 200 = 900000
  - 剩余 = `10000000000 - 900000 = 99999100000`
- 修复前本用例扣减为 0。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
