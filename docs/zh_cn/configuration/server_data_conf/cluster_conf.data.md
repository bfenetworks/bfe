# 集群转发配置

## 1. 配置简介

`cluster_conf.data` 是 BFE 的集群转发配置文件，用于声明各集群的后端、健康检查、负载均衡、AI 服务接入等参数。

---

## 2. 顶层结构

```json
{
    "Version": "20190101000000",
    "Config": { /* 集群名称 -> 集群转发配置 */ }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Version | string | Y | 配置文件版本，通常采用时间戳格式 | 参见 [Version](../00-common.md#5-配置文件版本version) |
| Config | object | Y | 各集群的转发配置参数；键为集群名称，值为集群转发配置 | 非空 |

---

## 3. 集群转发配置

以下配置项均位于 `Config[<cluster_name>]` 名字空间下。

```json
{
    "BackendConf": { /* 后端基础配置 */ },
    "CheckConf": { /* 健康检查配置 */ },
    "GslbBasic": { /* GSLB 基础配置 */ },
    "ClusterBasic": { /* 集群基础配置 */ },
    "HTTPSConf": { /* 后端 HTTPS 配置，可选 */ },
    "AIConf": { /* AI 服务配置，可选 */ }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| BackendConf | object | N | 后端基础配置 |
| CheckConf | object | N | 健康检查配置 |
| GslbBasic | object | N | GSLB 基础配置 |
| ClusterBasic | object | N | 集群基础配置 |
| HTTPSConf | object | N | 后端服务 HTTPS 配置；仅当 `BackendConf.Protocol` 为 `https` 时生效 |
| AIConf | object | N | AI 服务配置；用于大模型转发、API-Key 选择、模型定价等 |

---

## 4. 后端基础配置

```json
{
    "BackendConf": {
        "Protocol": "http",
        "TimeoutConnSrv": 2000,
        "TimeoutResponseHeader": 60000,
        "MaxIdleConnsPerHost": 2,
        "MaxConnsPerHost": 0,
        "SlowStartTime": 0,
        "RetryLevel": 0,
        "OutlierDetectionHttpCode": "",
        "FCGIConf": { /* 仅 Protocol=fcgi 时生效 */ }
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Protocol | string | N | 后端服务的协议；默认 `http` | 仅支持 `http`、`https`、`fcgi`、`tcp`、`ws`、`h2c` |
| TimeoutConnSrv | integer | N | 连接后端的超时时间，单位毫秒；默认 `2000` | >= 0 |
| TimeoutResponseHeader | integer | N | 从后端读响应头的超时时间，单位毫秒；默认 `60000` | >= 0 |
| MaxIdleConnsPerHost | integer | N | BFE 实例与每个后端的最大空闲长连接数；默认 `2` | >= 0 |
| MaxConnsPerHost | integer | N | BFE 实例与每个后端的最大长连接数；默认 `0`，表示无限制 | >= 0 |
| SlowStartTime | integer | N | 后端实例慢启动时间，单位秒；默认 `0`，表示不开启 | >= 0 |
| RetryLevel | integer | N | 请求重试级别；默认 `0` | `0`：连接后端失败时重试；`1`：连接后端失败、转发 GET 请求失败时均重试 |
| OutlierDetectionHttpCode | string | N | 后端响应状态码异常检查；默认 `""`，表示不开启 | 类型为 [HTTPStatusCodePattern](../00-common.md#9-http-状态码模式httpstatuscodepattern) |
| FCGIConf | object | N | FastCGI 协议的配置；仅当 `Protocol` 为 `fcgi` 时生效 | - |
| FCGIConf.Root | string | 条件 | FastCGI Root 文件夹位置；`FCGIConf` 配置时必填 | 非空 |
| FCGIConf.EnvVars | map[string]string | N | 自定义 FastCGI 环境变量 | - |

---

## 5. 健康检查配置

```json
{
    "CheckConf": {
        "Schem": "HTTP",
        "HostType": "HOST",
        "Uri": "/health_check",
        "Host": "",
        "StatusCode": 0,
        "StatusCodeRange": "",
        "FailNum": 5,
        "SuccNum": 1,
        "CheckTimeout": 0,
        "CheckInterval": 1000
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Schem | string | N | 健康检查协议；默认 `HTTP` | 仅支持 `HTTP`、`HTTPS`、`TCP`、`TLS` |
| HostType | string | N | 健康检查请求 Host 类型；默认 `HOST` | `HOST` 使用 `CheckConf.Host`；`ADDR` 使用后端实例地址 |
| Uri | string | N | 健康检查请求 URI；默认 `"/health_check"` | - |
| Host | string | N | 健康检查请求 HOST；默认 `""` | - |
| StatusCode | integer | N | 期待返回的响应状态码；默认 `0`，表示任意状态码均符合预期 | >= 0 |
| StatusCodeRange | string | N | 期待返回的响应状态码范围 | 类型为 [HTTPStatusCodePattern](../00-common.md#9-http-状态码模式httpstatuscodepattern) |
| FailNum | integer | N | 转发请求连续失败 `FailNum` 次后，将后端置为不可用并启动健康检查；默认 `5` | > 0 |
| SuccNum | integer | N | 健康检查连续成功 `SuccNum` 次后，将后端置为可用；默认 `1` | > 0 |
| CheckTimeout | integer | N | 健康检查的超时时间，单位毫秒；默认 `0`，表示无超时 | >= 0 |
| CheckInterval | integer | N | 健康检查的间隔时间，单位毫秒；默认 `1000` | > 0 |

---

## 6. GSLB 基础配置

```json
{
    "GslbBasic": {
        "CrossRetry": 0,
        "RetryMax": 2,
        "BalanceMode": "EPP",
        "HashConf": {
            "HashStrategy": 1,
            "HashHeader": "",
            "SessionSticky": false
        },
        "EPPAddr": ["epp-a.internal:9002", "epp-b.internal:9002"],
        "EPPCheck": {
            "CheckInterval": "2s",
            "FailThreshold": 3,
            "Cooldown": "45s",
            "SuccessThreshold": 2
        },
        "EPPTimeout": {
            "Connect": "500ms",
            "Call": "3s"
        },
        "EPPTLS": {
            "Insecure": false,
            "CAFile": "/bfe/conf/epp/epp_ca.crt"
        },
        "EPPBreaker": {
            "WindowSize": 100,
            "MinVolume": 20,
            "ErrorRatePercent": 50,
            "OpenTimeout": "30s"
        }
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| CrossRetry | integer | N | 跨子集群最大重试次数；默认 `0` | >= 0 |
| RetryMax | integer | N | 子集群内最大重试次数；默认 `2`；`BalanceMode` 为 `EPP` 时同样约束 EPP 调度路径的重试 | >= 0 |
| BalanceMode | string | N | 负载均衡模式；默认 `WRR` | `WRR`（加权轮询）、`WLC`（加权最小连接数）、`EPP`（基于外部策略，见 [6.1](#61-epp-调度配置balancemode--epp)） |
| HashConf | object | N | 会话保持的 HASH 策略配置；`BalanceMode` 为 `EPP` 时不生效（后端由 EPP 决策），保留便于模式回切 | - |
| HashConf.HashStrategy | integer | N | 哈希策略；默认 `1`（ClientIpOnly） | `0` ClientIdOnly；`1` ClientIpOnly；`2` ClientIdPreferred；`3` RequestURI |
| HashConf.HashHeader | string | N | 会话保持的 hash 请求头；Cookie 格式为 `"Cookie:key"` | - |
| HashConf.SessionSticky | boolean | N | 是否开启会话保持；默认 `false` | - |
| EPPAddr | []string | 条件 | EPP 服务端地址列表；`BalanceMode` 为 `EPP` 时必填 | 非空列表；元素为 `host:port`；列表内不允许重复 |
| EPPCheck | object | N | EPP 健康检查与故障转移滞回参数；仅 `BalanceMode` 为 `EPP` 时生效，缺省按默认值启用 | 见 [6.1.2](#612-eppcheck-元素) |
| EPPTimeout | object | N | EPP 调用超时参数；仅 `BalanceMode` 为 `EPP` 时生效 | 见 [6.1.3](#613-epptimeout-元素) |
| EPPTLS | object | N | EPP 连接 TLS 参数；仅 `BalanceMode` 为 `EPP` 时生效；缺省时保持 TLS 但跳过证书校验（兼容旧部署） | 见 [6.1.4](#614-epptls-元素) |
| EPPBreaker | object | N | EPP 调用熔断参数；仅 `BalanceMode` 为 `EPP` 时生效，缺省按默认值启用 | 见 [6.1.5](#615-eppbreaker-元素) |

### 6.1 EPP 调度配置（BalanceMode = "EPP"）

`BalanceMode` 为 `EPP` 时，BFE 通过 [ext-proc](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/ext_proc.proto) gRPC 协议将调度决策委托给 EPP 服务：BFE 在请求消息中携带 cluster 名（`llm-d.ai/inference-pool` metadata），EPP 在响应 dynamic metadata 的 `envoy.lb → x-gateway-destination-endpoint` 中返回选中的后端地址；EPP 不可用时按既有降级语义回退本地负载均衡（WRR）。EPP 相关指标见 `/monitor/epp_metrics`。

#### 6.1.1 EPPAddr 语义

`EPPAddr` 是**有序**地址列表：

- `[0]` = 该 cluster 的**主** EPP 实例；`[1]` = 同实例组内的**备** EPP 实例；超过 2 个为扩容预留，按序消费。
- 请求优先发往 `[0]`；仅在 `[0]` 连续健康检查失败达到阈值后切换到后续地址（见 6.1.2 滞回）。
- 单元素列表合法：无备实例（测试/单实例组场景），实例故障时降级本地负载均衡。
- 该列表通常由 ai-gateway-api 按"实例组主备分配"派生下发。

#### 6.1.2 EPPCheck 元素

```json
{
    "Disabled": false,
    "CheckInterval": "2s",
    "FailThreshold": 3,
    "Cooldown": "45s",
    "SuccessThreshold": 2
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Disabled | boolean | N | 是否关闭后台健康检查；默认 `false`；开启后仅依赖请求级错误重试，生产环境不建议 | - |
| CheckInterval | string | N | 健康检查探活周期（gRPC health，`grpc.health.v1`，朝 EPPAddr 探测）；默认 `"2s"` | 合法 duration 字符串，> 0 |
| FailThreshold | integer | N | 活跃地址连续失败该次数后故障转移到下一地址；默认 `3` | > 0 |
| Cooldown | string | N | 故障转移后的冷却期，期内不回切（防抖动）；默认 `"45s"` | 合法 duration 字符串，> 0 |
| SuccessThreshold | integer | N | 冷却期后，更高优先级地址需连续成功该次数才回切；默认 `2` | > 0 |

#### 6.1.3 EPPTimeout 元素

```json
{
    "Connect": "500ms",
    "Call": "3s"
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Connect | string | N | 建立 gRPC 连接/stream 的超时；默认 `"500ms"` | 合法 duration 字符串，> 0 |
| Call | string | N | 首消息（RequestHeaders 的 Send+Recv 往返）超时；超时即视为本次 EPP 调用失败，走失败回退路径；默认 `"3s"` | 合法 duration 字符串，> 0 |

#### 6.1.4 EPPTLS 元素

```json
{
    "Insecure": false,
    "CAFile": "/bfe/conf/epp/epp_ca.crt"
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Insecure | boolean | N | 是否跳过 EPP 服务端证书校验；默认 `false`；`true` 仅用于测试环境 | - |
| CAFile | string | 条件 | 校验 EPP 服务端证书的 CA 文件路径；`Insecure` 为 `false` 时必填 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；加载时校验文件可读 |

注意：**EPP 连接始终使用 TLS 传输**。未配置 `EPPTLS`（整个字段缺省）时保持 TLS 但跳过服务端证书校验，兼容升级前的既有 EPP 部署（会输出迁移告警日志）；生产环境应显式配置 `EPPTLS` 启用证书校验。CA 证书文件可经 server_data_conf 的 extra_files 通道下发到固定路径。

#### 6.1.5 EPPBreaker 元素

```json
{
    "Disabled": false,
    "WindowSize": 100,
    "MinVolume": 20,
    "ErrorRatePercent": 50,
    "OpenTimeout": "30s"
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Disabled | boolean | N | 是否关闭熔断；默认 `false` | - |
| WindowSize | integer | N | 滑动窗口大小（最近调用结果数）；默认 `100` | >= 1 |
| MinVolume | integer | N | 触发评估的最小调用量；默认 `20` | >= 1 且 <= `WindowSize` |
| ErrorRatePercent | integer | N | 窗口内错误率达到该百分比即熔断（OPEN），短路全部 EPP 调用并降级本地负载均衡；默认 `50` | [1, 100] |
| OpenTimeout | string | N | OPEN 持续该时长后进入半开（放行探测请求），探测成功关闭熔断并清空窗口、失败重新 OPEN；默认 `"30s"` | 合法 duration 字符串，> 0 |

配置热更新不会重置 OPEN 状态（避免配置重载导致击穿）。

---

## 7. 集群基础配置

```json
{
    "ClusterBasic": {
        "TimeoutReadClient": 30000,
        "TimeoutWriteClient": 60000,
        "TimeoutReadClientAgain": 60000,
        "ReqWriteBufferSize": 512,
        "ReqFlushInterval": 0,
        "ResFlushInterval": -1,
        "CancelOnClientClose": false,
        "DisableHostHeader": false,
        "DisableHealthCheck": false
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| TimeoutReadClient | integer | N | 读用户请求 body 的超时时间，单位毫秒；默认 `30000` | >= 0 |
| TimeoutWriteClient | integer | N | 写响应的超时时间，单位毫秒；默认 `60000` | >= 0 |
| TimeoutReadClientAgain | integer | N | 连接闲置超时时间，单位毫秒；默认 `60000` | >= 0 |
| ReqWriteBufferSize | integer | N | 请求的写 buffer 大小，单位 Bytes；默认 `512` | > 0 |
| ReqFlushInterval | integer | N | 刷新请求的间隔时间，单位毫秒；默认 `0` | >= 0 |
| ResFlushInterval | integer | N | 刷新响应的间隔时间，单位毫秒；默认 `-1` | - |
| CancelOnClientClose | boolean | N | 当服务端正在读后端响应时，客户端断连是否取消阻塞；默认 `false` | - |
| DisableHostHeader | boolean | N | 是否禁用 BFE 自动添加/覆盖的 Host 请求头；默认 `false` | - |
| DisableHealthCheck | boolean | N | 是否禁用该集群的健康检查；默认 `false` | - |

---

## 8. 后端服务 HTTPS 配置

```json
{
    "HTTPSConf": {
        "RSHost": "www.example.org",
        "BFEKeyFile": "",
        "BFECertFile": "",
        "RSCAList": [],
        "RSInsecureSkipVerify": false
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| RSHost | string | N | 后端服务实例的 hostname；用于验证服务端证书 | 非空；须为有效主机名 |
| BFEKeyFile | string | 条件 | BFE 向后端转发 HTTPS 请求时使用的私钥文件；双向认证时必填 | 类型为 [FilePath](../00-common.md#3-文件路径filepath) |
| BFECertFile | string | 条件 | BFE 向后端转发 HTTPS 请求时使用的证书文件；双向认证时必填 | 类型为 [FilePath](../00-common.md#3-文件路径filepath) |
| RSCAList | []string | 条件 | 后端服务端证书 CA 列表；`BackendConf.Protocol` 为 `https` 且需要验证服务端证书时必填 | 每个元素为有效的 pem 格式证书路径 |
| RSInsecureSkipVerify | boolean | N | 是否跳过服务端证书验证；默认 `false` | - |

---

## 9. AI 服务配置

`AIConf` 用于声明大模型后端的接入信息、API-Key 策略、模型映射与定价表。通常由 `ai-gateway-api` 自动生成并下发。

```json
{
    "AIConf": {
        "Type": 0,
        "Provider": "deepseek",
        "MatchPrefix": "",
        "StripPrefix": false,
        "ModelProtocols": ["openai"],
        "Keys": [ /* AIConf.Keys 元素 */ ],
        "KeyPolicy": { /* AIConf.KeyPolicy 元素 */ },
        "ModelMapping": {},
        "ModelTable": { /* AIConf.ModelTable 元素 */ }
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Type | integer | N | AI 服务类型；当前保留字段，请保持为 `0` | 仅支持 `0` |
| Provider | string | N | 该集群在 `model_prices` 中对应的 provider 名称；用于成本统计 | - |
| Keys | []object | N | 后端大模型服务的 API-Key 列表 | 元素见 [9.1 AIConf.Keys 元素](#91-aiconfkeys-元素) |
| KeyPolicy | object | N | API-Key 选择策略与重试退避配置 | 元素见 [9.2 AIConf.KeyPolicy 元素](#92-aiconfkeypolicy-元素) |
| ModelMapping | map[string]string | N | 原请求 model -> 后端服务 model 的映射；命中则重写请求 model | 键值均非空 |
| MatchPrefix | string | N | 需要匹配的 provider/model 前缀；用于 OpenRouter 等聚合 provider 场景 | `StripPrefix=true` 时必填；须以 `/` 结尾 |
| StripPrefix | boolean | N | 是否裁剪 `MatchPrefix` 指定前缀；默认 `false` | - |
| ModelProtocols | []string | N | 该集群 provider 支持的模型访问协议列表；为空时默认仅支持 `openai` | 元素取值须为 `openai` 或 `anthropic` |
| ModelTable | object | N | 该集群的模型定价表；当前货币固定为 `RMB` | 元素见 [9.3 AIConf.ModelTable 元素](#93-aiconfmodeltable-元素) |

### 9.1 AIConf.Keys 元素

```json
{
    "Name": "key-primary",
    "Key": "sk-example-api-key",
    "Weight": 100
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Name | string | Y | API-Key 名称/标识，用于日志、监控、运维识别 | 非空 |
| Key | string | Y | 实际用于后端认证的密钥 | 非空 |
| Weight | integer | Y | 加权随机选择时的权重；`0` 表示不接收流量 | `[0, 100]`；多 Key 时权重总和须为 `100` |

### 9.2 AIConf.KeyPolicy 元素

```json
{
    "Strategy": "weighted_random",
    "MaxRetries": 3,
    "RetryBackoffInitial": 500,
    "RetryBackoffMax": 5000,
    "SessionAffinity": true,
    "SessionAffinityTTL": 600,
    "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
    "SessionAffinityPenaltyEnable": true
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Strategy | string | N | Key 选择策略；当前仅支持 `weighted_random` | 仅支持 `weighted_random` |
| MaxRetries | integer | N | 一次 `aiClusterInvoke` 调用内，除首次选择外的最大重试次数；`0` 表示不重试 | >= 0 |
| RetryBackoffInitial | integer | N | 首次重试的退避时间，单位毫秒 | >= 0 |
| RetryBackoffMax | integer | N | 最大退避时间，单位毫秒 | >= 0，且须 >= `RetryBackoffInitial` |
| SessionAffinity | boolean | N | 是否开启基于 Redis + `ClientKeyId` 的会话级 Key 亲和性；默认 `false` | - |
| SessionAffinityTTL | integer | N | `ClientKeyId -> KeyName` 绑定的空闲超时时间，单位秒；命中绑定后会刷新 TTL，持续请求则绑定保持；默认 `600` | > 0 |
| SessionAffinityRedisPrefix | string | N | Redis 绑定键的前缀；默认 `"bfe:ai:key_affinity"` | 非空 |
| SessionAffinityPenaltyEnable | boolean | N | 是否开启 Key 惩罚：选择时跳过近期返回 429/401/403 的 Key；默认 `true` | - |

**会话级 Key 亲和性说明：**

- 开启后，BFE 使用 `AiBasicInfo.ClientKeyId` 作为会话标识，在 Redis 中维护 `{prefix}:{cluster_name}:{client_key_id} -> <key_name>` 的绑定。
- 同一 `ClientKeyId` 的后续请求优先命中已绑定的 Key；命中后会通过 `Expire` 刷新绑定 TTL，因此只要会话持续有请求，绑定就会一直保持。
- `SessionAffinityTTL` 是**空闲超时时间**：只有当 `ClientKeyId` 在 TTL 时间内没有请求时，绑定才会自动释放。
- 当绑定 Key 被惩罚、被删除或权重为 `0` 时，自动重新选择并写入新绑定。
- Redis 不可用时自动降级为无亲和的加权随机选择，不影响请求成功率。
- 若 `Keys` 中只有一个有效 Key，直接短路返回，不会访问 Redis。

### 9.3 AIConf.ModelTable 元素

```json
{
    "Currency": "RMB",
    "TimeZone": "Asia/Shanghai",
    "Tiers": [ /* AIConf.ModelTable.Tiers 元素 */ ],
    "Models": [ /* AIConf.ModelTable.Models 元素 */ ]
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Currency | string | Y | 货币类型；当前固定为 `RMB` | - |
| TimeZone | string | N | 计算时段所使用的时区；默认 `Asia/Shanghai` | 须为合法 IANA 时区名 |
| Tiers | []object | N | 时段 tier 定义列表；不填时按固定价格计费 | 元素见 [9.3.1 Tiers 元素](#931-tiers-元素) |
| Models | []object | Y | 模型定价条目列表 | 元素见 [9.3.2 Models 元素](#932-models-元素) |

#### 9.3.1 Tiers 元素

```json
{
    "Name": "peak",
    "TimeRanges": [
        { "Weekdays": [1, 2, 3, 4, 5], "Start": "09:00", "End": "12:00" },
        { "Weekdays": [1, 2, 3, 4, 5], "Start": "14:00", "End": "18:00" }
    ]
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Name | string | Y | Tier 名称；**初期只支持 `peak`** | 非空；当前仅支持 `peak` |
| TimeRanges | []object | Y | 该 tier 生效的时间范围列表；命中任意一个即属于该 tier；按列表顺序匹配 | 元素见 [9.3.1.1 TimeRanges 元素](#9311-timeranges-元素) |

##### 9.3.1.1 TimeRanges 元素

```json
{ "Weekdays": [1, 2, 3, 4, 5], "Start": "09:00", "End": "12:00" }
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Weekdays | []integer | N | 生效的星期几；`0`=周日，`1`=周一，...，`6`=周六；为空表示每天 | 元素取值范围 `0-6` |
| Start | string | Y | 开始时间，格式 `HH:MM` | 合法时间格式；须 < `End` |
| End | string | Y | 结束时间，格式 `HH:MM`；区间为左闭右开 `[Start, End)` | 合法时间格式；须 > `Start` |

#### 9.3.2 Models 元素

```json
{
    "Provider": "deepseek",
    "Model": "deepseek-v4-pro",
    "BaseModel": "deepseek-v4-pro",
    "Mode": "chat",
    "Capabilities": ["chat", "reasoning", "tools", "prompt_caching"],
    "SupportedParameters": ["temperature", "max_tokens"],
    "Limits": { /* 限制对象 */ },
    "Prices": { /* 默认价格 */ },
    "TierPrices": { /* 分时段价格 */ }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Provider | string | Y | Provider 名 | 非空 |
| Model | string | Y | 模型名，用于匹配请求中的 `target_model` | 非空 |
| BaseModel | string | Y | 归一化模型名 | 非空 |
| Mode | string | N | 请求模式，例如 `chat` | - |
| Capabilities | []string | N | 能力列表，例如 `["chat", "reasoning"]` | - |
| SupportedParameters | []string | N | 支持的请求参数列表，例如 `["temperature", "max_tokens"]` | - |
| Limits | map[string]integer | N | 限制对象，例如 `context_window` 等 | - |
| Prices | map[string]number | N | 默认价格对象；未命中任何 tier 时使用 | - |
| TierPrices | map[string]map[string]number | N | 分时段价格对象；tier name -> 价格表。**初期 tier name 只支持 `peak`**；tier 内未配置的键 fallback 到 `Prices` | 内部键名须为 `prices` 枚举键名 |

---

## 10. 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "cluster_example": {
            "BackendConf": {
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0,
                "OutlierDetectionHttpCode": "5xx|403"
            },
            "CheckConf": {
                "Schem": "http",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000
            }
        },
        "https_cluster_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
                "Uri": "/",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 30000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            },
            "HTTPSConf":{
                "RSHost": "www.example.org",
                "BFEKeyFile": "../conf/tls_conf/backend_rs/r_bfe_dev_prv.pem",
                "BFECertFile": "../conf/tls_conf/backend_rs/r_bfe_dev.crt",
                "RSCAList": [
                    "../conf/tls_conf/backend_rs/bfe_r_ca.crt",
                    "../conf/tls_conf/backend_rs/bfe_i_ca.crt"
                ],
                "RSInsecureSkipVerify": false
            }
        },
        "fcgi_cluster_example": {
            "BackendConf": {
                "Protocol": "fcgi",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "MaxConnsPerHost": 0,
                "RetryLevel": 0,
                "FCGIConf": {
                    "Root": "/home/work",
                    "EnvVars": {
                        "VarKey": "VarVal"
                    }    
                }
            },
            "CheckConf": {
                "Schem": "http",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 1,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            }
        },
        "ai_cluster_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            },
            "AIConf": {
                "Type": 0,
                "Provider": "deepseek",
                "MatchPrefix": "openrouter/",
                "StripPrefix": true,
                "ModelProtocols": ["openai"],
                "Keys": [
                    {
                        "Name": "key-primary",
                        "Key": "sk-example-api-key-primary",
                        "Weight": 70
                    },
                    {
                        "Name": "key-secondary",
                        "Key": "sk-example-api-key-secondary",
                        "Weight": 30
                    }
                ],
                "KeyPolicy": {
                    "Strategy": "weighted_random",
                    "MaxRetries": 3,
                    "RetryBackoffInitial": 500,
                    "RetryBackoffMax": 5000,
                    "SessionAffinity": true,
                    "SessionAffinityTTL": 600,
                    "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
                    "SessionAffinityPenaltyEnable": true
                },
                "ModelMapping": {
                    "gpt-4": "backend-gpt-4-model"
                },
                "ModelTable": {
                    "Currency": "RMB",
                    "Models": [
                        {
                            "Provider": "deepseek",
                            "Model": "deepseek-v3",
                            "BaseModel": "deepseek-v3",
                            "Mode": "chat",
                            "Capabilities": ["chat", "reasoning", "tools"],
                            "SupportedParameters": ["temperature", "max_tokens"],
                            "Limits": {
                                "context_window": 128000,
                                "max_input_tokens": 128000,
                                "max_output_tokens": 8192
                            },
                            "Prices": {
                                "input_cost_per_token": 0.000002,
                                "output_cost_per_token": 0.000008
                            }
                        }
                    ]
                }
            }
        },
        "ai_cluster_anthropic_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            },
            "AIConf": {
                "Type": 0,
                "Provider": "my-anthropic",
                "ModelProtocols": ["anthropic"],
                "Keys": [
                    {
                        "Name": "key-primary",
                        "Key": "sk-ant-api03-example",
                        "Weight": 100
                    }
                ],
                "KeyPolicy": {
                    "Strategy": "weighted_random",
                    "MaxRetries": 3,
                    "RetryBackoffInitial": 500,
                    "RetryBackoffMax": 5000
                },
                "ModelTable": {
                    "Currency": "RMB",
                    "Models": [
                        {
                            "Provider": "my-anthropic",
                            "Model": "claude-3-5-sonnet",
                            "BaseModel": "claude-3-5-sonnet",
                            "Mode": "chat",
                            "Capabilities": ["chat", "tools"],
                            "SupportedParameters": ["temperature", "max_tokens"],
                            "Limits": {
                                "context_window": 200000,
                                "max_input_tokens": 200000,
                                "max_output_tokens": 8192
                            },
                            "Prices": {
                                "input_cost_per_token": 0.000003,
                                "output_cost_per_token": 0.000015,
                                "cache_read_input_token_cost": 0.000000375,
                                "cache_creation_input_token_cost": 0.00000375
                            }
                        }
                    ]
                }
            }
        },
        "ai_cluster_deepseek_tiered_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            },
            "AIConf": {
                "Type": 0,
                "Provider": "deepseek",
                "ModelProtocols": ["openai"],
                "Keys": [
                    {
                        "Name": "key-primary",
                        "Key": "sk-example-api-key",
                        "Weight": 100
                    }
                ],
                "KeyPolicy": {
                    "Strategy": "weighted_random",
                    "MaxRetries": 3,
                    "RetryBackoffInitial": 500,
                    "RetryBackoffMax": 5000
                },
                "ModelTable": {
                    "Currency": "RMB",
                    "TimeZone": "Asia/Shanghai",
                    "Tiers": [
                        {
                            "Name": "peak",
                            "TimeRanges": [
                                { "Weekdays": [1, 2, 3, 4, 5], "Start": "09:00", "End": "12:00" },
                                { "Weekdays": [1, 2, 3, 4, 5], "Start": "14:00", "End": "18:00" }
                            ]
                        }
                    ],
                    "Models": [
                        {
                            "Provider": "deepseek",
                            "Model": "deepseek-v4-pro",
                            "BaseModel": "deepseek-v4-pro",
                            "Mode": "chat",
                            "Capabilities": ["chat", "reasoning", "tools", "prompt_caching"],
                            "SupportedParameters": ["temperature", "max_tokens"],
                            "Limits": {
                                "context_window": 128000,
                                "max_input_tokens": 128000,
                                "max_output_tokens": 8192
                            },
                            "Prices": {
                                "input_cost_per_token": 0.0000045,
                                "output_cost_per_token": 0.0000135,
                                "cache_read_input_token_cost": 0.00000015
                            },
                            "TierPrices": {
                                "peak": {
                                    "input_cost_per_token": 0.000009,
                                    "output_cost_per_token": 0.000027,
                                    "cache_read_input_token_cost": 0.0000003
                                }
                            }
                        }
                    ]
                }
            }
        },
        "epp_cluster_example": {
            "BackendConf": {
                "Protocol": "http",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 60000,
                "MaxIdleConnsPerHost": 2,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "http",
                "Uri": "/healthcheck",
                "StatusCode": 200,
                "FailNum": 5,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "BalanceMode": "EPP",
                "EPPAddr": [
                    "epp-a.internal:9002",
                    "epp-b.internal:9002"
                ],
                "EPPCheck": {
                    "CheckInterval": "2s",
                    "FailThreshold": 3,
                    "Cooldown": "45s",
                    "SuccessThreshold": 2
                },
                "EPPTimeout": {
                    "Connect": "500ms",
                    "Call": "3s"
                },
                "EPPTLS": {
                    "Insecure": false,
                    "CAFile": "../conf/epp/epp_ca.crt"
                },
                "EPPBreaker": {
                    "WindowSize": 100,
                    "MinVolume": 20,
                    "ErrorRatePercent": 50,
                    "OpenTimeout": "30s"
                },
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000
            }
        }
    }
}
```

---

## 11. 注解

### 11.1 StatusCodeRange

- 响应状态码范围。如果配置了 `StatusCode`，则会忽略此验证条件。
- 合法的配置项举例：
  1. `"3xx"`、`"4xx"`、`"5xx"` 其中之一。
  2. 特定的 HTTP 返回码，与 `StatusCode` 功能一致。
  3. `"|"` 符号连接的上述 (1) 或 (2)，例如：
     - `"503|4xx"`
     - `"501|409|30x"`
