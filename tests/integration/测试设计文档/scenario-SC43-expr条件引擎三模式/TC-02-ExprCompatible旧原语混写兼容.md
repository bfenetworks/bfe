# TC-02 ExprCompatible 旧原语混写兼容

## 用例编号与名称

TC-02 ExprCompatible 旧原语混写兼容

## 所属场景

SC42 expr 条件引擎三模式

## 版本声明

- `bfe`：当前源码版本（含 expr 条件引擎一期：`bfe_expr/legacy_patcher.go` 的
  `legacyNameMap`/`legacyPatcher` 旧原语名重写、`bfe_expr/legacy_env.go` 的旧原语 Env 方法、
  `bfe_basic/condition/build.go` 的 ExprCompatible 分派）

## 测试目的

验证 **ExprCompatible 模式**下旧原语兼容层的实际行为：

- 旧原语（`req_host_in`、`req_path_prefix_in`）在 expr 引擎下可用且语义与旧引擎一致；
- **新旧混写**：旧原语与新风格 Env 方法调用可在同一表达式中组合（`legacyPatcher` 只重写
  命中 `legacyNameMap` 的调用，其余名字原样参与类型检查）；
- 条件不满足时回落到 `default_t()` 规则。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程（本用例一个实例）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：`cluster_expr_hit` / `cluster_expr_default` 均返回 200，响应体含 cluster 名。
3. `expr.example.org` 已加入 `host_rule.data` 的 `expr_product`。
4. `bfe.conf` 已改写 `ConditionEngineMode = ExprCompatible`。

## 配置构造

- `server_data_conf/route_rule.data`（`expr_product`），两条规则按顺序匹配：

| 规则名 | 条件 | ClusterName |
|--------|------|-------------|
| mix-hit | `req_path_prefix_in("/api", false) && ReqHeader("X-Expr") == "on"` | `cluster_expr_hit` |
| expr-default | `default_t()` | `cluster_expr_default` |

> 规则 1 即新旧混写表达式：旧原语 `req_path_prefix_in`（legacyPatcher 重写为 Env 方法）
> 与 Env 方法 `ReqHeader` 直接组合。

## BFE 请求

请求发往 `http://127.0.0.1:<bfePort>`，Host `expr.example.org`。

| 步骤 | 方法 | 路径 | Header | 期望命中 cluster | 期望结果 |
|------|------|------|--------|------------------|----------|
| 1 | GET | `/api/users` | `X-Expr: on` | `cluster_expr_hit` | 200 |
| 2 | GET | `/api/users` | 不带 `X-Expr` | `cluster_expr_default` | 200 |
| 3 | GET | `/api/users` | `X-Expr: off`（值不等） | `cluster_expr_default` | 200 |
| 4 | GET | `/web` | `X-Expr: on`（路径前缀不命中） | `cluster_expr_default` | 200 |

## 预期结果

- 步骤 1：响应体含 `cluster_expr_hit`（旧原语与新风格混写表达式编译成功且求值为真）；
- 步骤 2/3/4：响应体含 `cluster_expr_default`（`&&` 任一操作数为假即整体为假，回落默认规则）；
- 全部步骤 HTTP 200，BFE 启动日志无条件编译错误。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
