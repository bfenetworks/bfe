# TC-07 AIKeyAffinity 未配置时 fail-open

## 用例编号与名称

TC-07 AIKeyAffinity 未配置时 fail-open

## 所属场景

SC10 同 cluster 多 API-Key 会话保持

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 `bfe.conf` 未配置 `[AIKeyAffinity]` 段（或 `Disabled = true`）时，即使 cluster 开启 `SessionAffinity`，请求仍正常转发（fail-open），但不建立任何 Redis 绑定，Key 按加权随机分发。

背景：AI Key 会话保持的 Redis 连接由 `bfe_server` 核心自持（`[AIKeyAffinity]` 段配置），不再借用 `mod_ai_rate_limit` 模块的 Redis；未配置该段时 affinity 静默不生效，与历史「模块未加载拿不到句柄」的行为一致。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动。
3. mock 后端 `cluster_session_affinity` 已启动，返回 200。
4. 临时 BFE 配置已生成并加载，`SessionAffinity = true`，且 `[AIKeyAffinity]` 段 `Disabled = true`（等价于未配置该段，默认即为 `true`）。

## 配置构造

- `cluster_session_affinity.AIConf`：
  - `Keys`: `[{"Name":"key-a","Key":"sk-key-a","Weight":50},{"Name":"key-b","Key":"sk-key-b","Weight":30},{"Name":"key-c","Key":"sk-key-c","Weight":20}]`
  - `KeyPolicy.SessionAffinity`: `true`
  - `KeyPolicy.SessionAffinityTTL`: `300`
- `bfe.conf`：
  - `[AIKeyAffinity]` 段 `Disabled = true`

## BFE 请求

| Host | Path | Authorization | Body | 次数 |
|------|------|---------------|------|------|
| `session-affinity.example.org` | `/v1/chat/completions` | `Bearer ak_session_affinity` | `{"model":"gpt-4"}` | 50 |

执行步骤：

1. 启动 BFE（`[AIKeyAffinity]` 段禁用），`SessionAffinity = true`；
2. 使用同一 `ak_session_affinity` 连续发送 50 次请求；
3. 检查响应状态码、mock 后端收到的 Provider Key 种类，以及 Redis 中是否存在绑定 key。

## 预期结果

- 50 次请求全部返回 200（fail-open 不影响转发）。
- Redis 中不存在绑定 key `bfe:ai:key_affinity:cluster_session_affinity:session_affinity_key_id`。
- mock 后端收到的 Provider Key 至少 2 种（无 affinity，按加权随机分发）。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
