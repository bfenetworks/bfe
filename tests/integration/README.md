# BFE 集成测试

本目录承载 `bfe` 的**真实进程级集成测试**。与仓库中 `integration-test/` 目录不同的是：

- 仅启动真实的 `bfe` 进程，不引入 `ai-gateway-api`、`conf-agent` 等外部组件；
- 测试所需的 BFE 配置文件（如 `ai_route.data`、`cluster_table.data` 等）直接由测试代码或静态 `testdata` 提供；
- 请求通过真实 HTTP 发送到 BFE 监听端口，验证转发行为与后端命中统计。

## 目录结构

```text
bfe/tests/integration/
├── README.md                                  # 本文档
├── common/                                    # 公共 harness
│   ├── process_env.go                         # 编译/启动/停止真实 BFE 进程
│   ├── bfe_config_builder.go                  # 生成临时 BFE 配置
│   ├── mock_backend.go                        # 本地 mock AI 后端
│   └── util.go                                # 工具函数
├── implementation/                            # Go 实现代码（ASCII 目录名）
│   └── scenario-SC01-route-table-lookup/
│       ├── sc01_route_table_lookup_test.go
│       └── testdata/                          # 静态 BFE 配置模板
└── 测试设计文档/                               # 中文测试设计文档
    ├── 测试场景总体说明.md
    └── scenario-SC01-路由表查找与绑定/
        ├── 场景说明.md
        └── TC-*.md
```

## 运行方式

在 `bfe/` 目录下执行：

```bash
# 运行全部集成测试
go test ./tests/integration/... -v

# 运行单个场景
go test ./tests/integration/implementation/scenario-SC01-route-table-lookup/... -v

# 运行单个测试例
go test ./tests/integration/implementation/scenario-SC01-route-table-lookup/ -run TestTC01 -v
```

首次运行会自动编译 `bfe` 二进制并缓存到 `bfe/tests/integration/.integration-test-bin/`。

## 当前覆盖

| 场景 | 说明 |
|------|------|
| SC01 路由表查找与绑定 | 验证 `mod_ai_route` 在多级路由表（apikey/entity/global）中的搜索与回退顺序，以及 fallback 时 body 回绕行为 |

## 参考文档

- `document-ai-gateway/BFE设计/v0.3.0/BFE的mod_ai_route集成测试方案/BFE的mod_ai_route集成测试方案v1.0.0.md`
- `integration-test/方案说明/总体说明/集成测试方案说明.md`
