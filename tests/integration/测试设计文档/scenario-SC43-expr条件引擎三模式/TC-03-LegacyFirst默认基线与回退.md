# TC-03 LegacyFirst 默认基线与回退

## 用例编号与名称

TC-03 LegacyFirst 默认基线与回退

## 所属场景

SC42 expr 条件引擎三模式

## 版本声明

- `bfe`：当前源码版本（含 expr 条件引擎一期：`bfe_basic/condition/build.go` 的
  LegacyFirst 分派——旧引擎优先、失败回退 `bfe_expr.CompileLegacy`、`bfe_config/bfe_conf`
  的 `ConditionEngineMode` 缺省值 LegacyFirst）

## 测试目的

验证 **LegacyFirst（默认）模式**的两条路径：

- **基线**：`bfe.conf` 不写 `ConditionEngineMode` 时进程按默认 LegacyFirst 启动，
  旧条件表达式行为与旧版完全一致（存量配置零回退）；
- **回退**：新风格表达式条件在 LegacyFirst 下由旧引擎编译失败 → 自动回退 expr 引擎
  编译成功，并按表达式语义路由——即同一进程内两种引擎的条件可并存。

两个验证点使用独立产品线（`legacy_product` / `fallback_product`），规则互不影响。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程（本用例一个实例，**不**改写 `ConditionEngineMode`，
验证缺省默认行为）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：`cluster_expr_hit` / `cluster_expr_default` 均返回 200，响应体含 cluster 名。
3. `legacy.example.org` → `legacy_product`、`fallback.example.org` → `fallback_product`
   已加入 `host_rule.data`。
4. `bfe.conf` **不含** `ConditionEngineMode` 键（或使用框架默认 conf，验证缺省值）。

## 配置构造

- `server_data_conf/route_rule.data`：

| 产品线 | 规则名 | 条件 | ClusterName |
|--------|--------|------|-------------|
| `legacy_product` | legacy-hit | `req_path_prefix_in("/api", false)`（纯旧原语） | `cluster_expr_hit` |
| `legacy_product` | legacy-default | `default_t()` | `cluster_expr_default` |
| `fallback_product` | fallback-hit | `ReqMethod() in ["GET"] && ReqPath() startsWith "/api"`（新风格，旧引擎编译失败） | `cluster_expr_hit` |
| `fallback_product` | fallback-default | `default_t()` | `cluster_expr_default` |

## BFE 请求

请求发往 `http://127.0.0.1:<bfePort>`。

| 步骤 | Host | 方法 | 路径 | 期望命中 cluster | 期望结果 |
|------|------|------|------|------------------|----------|
| 1 | `legacy.example.org` | GET | `/api/users` | `cluster_expr_hit` | 200 |
| 2 | `legacy.example.org` | GET | `/web` | `cluster_expr_default` | 200 |
| 3 | `fallback.example.org` | GET | `/api/users` | `cluster_expr_hit` | 200 |
| 4 | `fallback.example.org` | POST | `/api/users`（方法不在集合内） | `cluster_expr_default` | 200 |
| 5 | `fallback.example.org` | GET | `/web`（路径不命中） | `cluster_expr_default` | 200 |

## 预期结果

- 步骤 1/2：旧原语条件在默认模式下行为不变（基线回归）；
- 步骤 3：新风格表达式条件在 LegacyFirst 下回退 expr 引擎编译成功，按表达式语义命中
  `cluster_expr_hit`（若回退失效，该条件将编译失败导致 BFE 启动失败，用例在启动阶段即失败）；
- 步骤 4/5：表达式求值为假，回落 `cluster_expr_default`；
- 全部步骤 HTTP 200。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
