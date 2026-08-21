# AGENTS.md — BFE Server

This file guides AI coding agents working on the `bfe/` codebase (the data-plane BFE Server).

## Project overview

BFE (Beyond Front End) is a modern layer-7 load balancer originated at Baidu and a CNCF sandbox project. This repository contains **BFE Server**, the data-plane component that forwards user traffic based on content-based routing, load balancing, and a flexible plugin framework.

BFE system context (for orientation only):
- **Data plane**: BFE Server (this repo) — forwards traffic.
- **Control plane**: API-Server, Conf-Agent, Dashboard (separate repos) — manage and distribute configuration.
- **Kubernetes integration**: BFE Ingress Controller (separate repo).

## High-level architecture

Entry point: `bfe.go`
- Parses flags (`-c conf_root`, `-l log_dir`, `-t test_conf`, etc.).
- Loads server config via `bfe_config/bfe_conf.BfeConfigLoad`.
- Starts the server via `bfe_server.StartUp`.

Core request flow:
1. Listener (`bfe_server/`) accepts HTTP/HTTPS/HTTP2/WebSocket/SPDY/FastCGI connections.
2. TLS handshake and session handling (`bfe_tls/`).
3. HTTP request parsing (`bfe_http/`, `bfe_http2/`, `bfe_bufio/`).
4. Routing: host table → cluster table (`bfe_route/`, `bfe_config/bfe_route_conf/`).
5. Module pipeline execution in fixed order (`bfe_modules/`, registered in `bfe_modules/bfe_modules.go`).
6. Backend selection and load balancing (`bfe_balance/`).
7. Proxying and response handling (`bfe_server/reverseproxy.go`, `bfe_server/response.go`).

## Directory structure and module relationships

| Directory | Responsibility |
|-----------|----------------|
| `bfe_server/` | HTTP(S) listeners, connection handling, reverse proxy, TLS termination, module registration, status/monitoring. |
| `bfe_route/` | Host and cluster routing tables, trie-based lookups (`trie/`), server data config (`server_data_conf.go`). |
| `bfe_balance/` | Backend instances (`backend/`), GSLB (`bal_gslb/`) and SLB (`bal_slb/`) balancing policies, balance table. |
| `bfe_config/` | Configuration loading: `bfe_conf/`, `bfe_route_conf/`, `bfe_cluster_conf/`, `bfe_tls_conf/`. |
| `bfe_http/`, `bfe_http2/`, `bfe_websocket/`, `bfe_spdy/`, `bfe_fcgi/`, `bfe_stream/` | Protocol implementations. |
| `bfe_tls/` | TLS handshake, certificates, session cache, server rules. |
| `bfe_module/` | Plugin framework: module interface, callback/handler lists, filters. |
| `bfe_modules/` | Built-in modules (access, WAF, redirect, rewrite, AI routing, rate limiting, etc.). Registered in `bfe_modules/bfe_modules.go`. |
| `bfe_basic/` | Condition parser/primitives used by modules. Generated parser code lives in `bfe_basic/condition/parser`. |
| `bfe_net/`, `bfe_bufio/`, `bfe_util/`, `bfe_debug/` | Shared network, buffered I/O, utilities, debug flags. |
| `conf/` | Sample runtime configuration files. |
| `docs/`, `examples/`, `tests/` | Documentation, deployment examples, and integration tests. |

## Build/test conventions

- **Go version**: 1.22 (`go.mod`).
- **Module**: `github.com/bfenetworks/bfe`.
- **Build**: `make` (or `make all`) → prepare, compile, package.
  - `make build` builds the `bfe` binary.
  - `make strip` builds without symbols.
- **Test**: `make test` runs `go test -cover ./...` and `go vet ./...`.
- **Prepare**: `make prepare` installs `goyacc` and regenerates `bfe_basic/condition/parser` via `go generate`.
- **Lint/static analysis**: `make check` runs `staticcheck`; `make license-check` / `make license-fix` use `license-eye`.
- **Pre-commit**: Install with `pre-commit install`; `gofmt` is required.
- **Docker**: `make docker` builds prod + debug images; `make docker-push REGISTRY=...` builds and pushes multi-arch images.
- **Release**: `make release` builds tar.gz packages for darwin/amd64, linux/amd64, linux/arm64, windows/amd64.

## Common modification patterns

### Add or modify a BFE module
1. Create a package under `bfe_modules/mod_<name>/`.
2. Implement `bfe_module.BfeModule` and the required callbacks/handlers.
3. Add config loader under `bfe_config/` if new config files are needed.
4. Add module registration in `bfe_modules/bfe_modules.go` in the correct execution order; document ordering requirements in comments.
5. Add sample config under `conf/` and update config documentation.
6. Add unit tests using `testing` + `testify`.
7. Run `make test` before submitting.

### AI gateway module changes

The AI gateway modules under `bfe_modules/mod_ai_*` and `bfe_modules/mod_body_process` have ordering and lifecycle interdependencies:

- `mod_ai_route` runs early to select the target cluster/model.
- `mod_ai_token_auth` runs at `HandleFoundProduct` for API Key validation and quota plan binding; it also performs final quota deduction at `HandleRequestFinish`.
- `mod_body_process` runs at `HandleReadResponse` and is responsible for parsing token usage from streaming (SSE) responses. If you modify RMB quota deduction, ensure streaming scenarios still work when `mod_body_process` is loaded.
- For RMB quota details, see `docs/zh_cn/sys_design/rmb_quota.md`.

### Routing changes
- Host/cluster tables: `bfe_route/`.
- Config loaders: `bfe_config/bfe_route_conf/`.
- Sample configs: `conf/`.

### Load-balancing changes
- Backend model: `bfe_balance/backend/`.
- Balancing policies: `bfe_balance/bal_gslb/` and `bfe_balance/bal_slb/`.
- Balance table: `bfe_balance/bal_table.go`.

### Protocol support changes
- HTTP: `bfe_http/`, `bfe_server/http_conn.go`.
- HTTP/2: `bfe_http2/`.
- WebSocket: `bfe_websocket/`.
- TLS: `bfe_tls/`.

### Condition/rule language changes
- Grammar/parser: `bfe_basic/condition/parser`.
- Run `make prepare` after grammar changes to regenerate parser code.

## Agent guidelines

- **Preserve module order** in `bfe_modules/bfe_modules.go`. Many modules have explicit ordering requirements; keep the comments up to date.
- **Regenerate generated code** after parser/grammar changes (`make prepare`).
- **Keep tests idiomatic**: use `testing` and `testify/assert`/`require`. Place `_test.go` files next to the code under test.
- **License headers**: all new source files need the Apache 2.0 header. Use `make license-fix` if unsure.
- **Do not hand-edit vendored or generated files**.
- **Run `make test`** as the minimal local verification.
- **Configuration changes must be reflected in `conf/`** so that `make package` and Docker builds produce a usable default setup.

## Useful references

- `README.md` / `README-CN.md` — project overview and quick start.
- `CONTRIBUTING.md` — workflow, commit sign-off, code style.
- `Makefile` — build, test, Docker, and release targets.
- `docs/en_us/introduction/overview.md` — detailed architecture documentation.
