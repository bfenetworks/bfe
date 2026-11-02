# TC-01 ExprOnly 纯表达式路由与回落

## 用例编号与名称

TC-01 ExprOnly 纯表达式路由与回落

## 所属场景

SC42 expr 条件引擎三模式

## 版本声明

- `bfe`：当前源码版本（含 expr 条件引擎一期：`bfe_expr` 包、`bfe_basic/condition/build.go`
  的三模式 `Build` 分派、`bfe_config/bfe_conf/conf_basic.go` 的 `ConditionEngineMode`、
  `bfe_server` StartUp 起始处的模式接线）

## 测试目的

验证 **ExprOnly 模式**下新风格表达式作为路由条件的完整行为：

- expr 引擎编译期类型检查生效：表达式按 Env 方法签名求值；
- 集合运算 `in` 与字符串谓词 `startsWith` 语义正确；
- 条件为真路由到目标 cluster，条件为假按规则顺序回落到 `true` 常量规则；
- ExprOnly 模式下旧原语名是未知函数（负向语义由 TC-04(b) 验证，本用例不做）。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程（本用例一个实例）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端已启动：`cluster_expr_hit` / `cluster_expr_default` 均返回 200，响应体含 cluster 名。
3. `expr.example.org` 已加入 `host_rule.data` 的 `expr_product`。
4. `bfe.conf` 已改写 `ConditionEngineMode = ExprOnly`（`common.RewriteConfLine`）。

## 配置构造

- `server_data_conf/route_rule.data`（`expr_product`）：

| 规则名 | 条件 | ClusterName |
|--------|------|-------------|
| expr-hit | `ReqMethod() in ["GET", "HEAD"] && ReqPath() startsWith "/api"` | `cluster_expr_hit` |
| expr-default | `true` | `cluster_expr_default` |

> 回落规则用 expr 原生常量 `true` 而非旧写法 `default_t()`：ExprOnly 按设计拒绝
> 旧原语名（负向语义由 TC-04(b) 验证），`default_t()` 在 ExprOnly 下编译失败；
> `true` 在三种模式下均可编译（LegacyFirst 下回退 expr 引擎）。

## BFE 请求

请求发往 `http://127.0.0.1:<bfePort>`，Host `expr.example.org`。

| 步骤 | 方法 | 路径 | 期望命中 cluster | 期望结果 |
|------|------|------|------------------|----------|
| 1 | GET | `/api/users` | `cluster_expr_hit` | 200 |
| 2 | HEAD | `/api/status` | `cluster_expr_hit` | 200 |
| 3 | POST | `/api/users`（方法不在集合内） | `cluster_expr_default` | 200 |
| 4 | GET | `/web/index`（路径不前缀匹配） | `cluster_expr_default` | 200 |
| 5 | GET | `/api`（前缀边界，等于 "/api"） | `cluster_expr_hit` | 200 |
| 6 | GET | `/apiary/list`（`startsWith "/api"` 命中，前缀非段边界） | `cluster_expr_hit` | 200 |

## 预期结果

- 步骤 1/2/5/6：响应体含 `cluster_expr_hit`（步骤 2 为 HEAD，响应体恒为空，
  改由 mock 后端请求计数判定命中 `cluster_expr_hit`）；
- 步骤 3/4：响应体含 `cluster_expr_default`；
- 全部步骤 HTTP 200，无 BFE 错误日志（条件运行期求值错误按 false 处理，
  步骤 3/4 是正常未命中而非错误）。

## 清理

停止 `bfe` 进程与所有 mock 后端，删除临时目录。
