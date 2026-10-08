# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run every module's tests with the race detector
make test

# Lint every module (golangci-lint, root config)
make lint

# Architectural gate: the core must resolve no adapter
make lint-arch

# External dependency footprint per module
make lint-deps

# go mod tidy in every module + go work sync
make tidy

# Run a single package's tests (from inside its module)
cd aws && go test ./pkg/clients/cognito/... -v

# Run a single test
cd aws && go test ./pkg/clients/cognito/... -v -run TestMFA

# Initialize/setup (runs init.sh)
make init
```

## Architecture

`go-engine` is a reusable Go framework consumed via `go get`. It is not a
runnable binary. Since `v0.30.0` it is a **set of modules**, one per adapter
family, bound together for local development by the `go.work` at the root.

| Module | Directory | Contains |
|---|---|---|
| `github.com/skolldire/go-engine` | `/` | the core |
| `github.com/skolldire/go-engine/aws` | `/aws` | AWS clients + providers + `aws/preset` |
| `github.com/skolldire/go-engine/messaging` | `/messaging` | Kafka, RabbitMQ, gRPC + providers |
| `github.com/skolldire/go-engine/http` | `/http` | REST client + provider |
| `github.com/skolldire/go-engine/database/{sql,redis,mongodb,memcached}` | `/database/*` | one module per engine; their drivers share no dependency |
| `github.com/skolldire/go-engine/preset/full` | `/preset/full` | the everything preset |

**The rule:** the core knows no adapter; adapters know the core. It is enforced
structurally — the core is its own module, so importing an adapter would require
a `require` line in its `go.mod` — and `make lint-arch` reads that manifest.

Every provider lives in its family's module. It has to: `provider/sqs` imports
both the core and the AWS adapter, so keeping it at the root would create a
core → aws → core cycle between modules. `provider/otel` is the exception that
stays at the root, because it depends only on `pkg/telemetry/otel`.

### Core assembly: `pkg/engine`

The entry point for consumers is `engine.New(ctx, opts...)`:

```go
eng, err := engine.New(ctx,
    engine.WithRouter(),
    engine.WithHealth(),
    engine.WithProvider(sqs.New("orders"), redis.New("cache")),
)
```

Options carry no ordering rules — `New` applies steps in dependency order.
Presets bundle them: `preset/http` (core only), `aws/preset` (AWS only),
`preset/full` (everything the YAML declares).

- `engine.Provider` — `Name`, `ConfigKey`, `Init(ctx, RawConfig, Deps)`, `Close(ctx)`.
- `engine.Config` uses `mapstructure:",remain"`: the core types only `router`,
  `log` and `health`; every other section travels raw and each provider decodes
  its own.
- Typed retrieval is `engine.Get[T](eng, name)`, or the `From` helper each
  provider exposes (`sqs.From(eng, "orders")`).
- A section with no registered provider builds nothing. That is deliberate;
  `eng.ComponentNames()` lists what was actually built.

### Core packages (root module)

| Package | Purpose |
|---|---|
| `pkg/engine` | the Provider core, config loading, lifecycle |
| `pkg/router` | chi router, JWT middleware, health/ping/pprof routes |
| `pkg/health` | health service and checkers |
| `pkg/core/client` | `BaseClient`, `SafeTypeAssert[T]`, middleware chain |
| `pkg/core/registry` | client factory registry |
| `pkg/telemetry/otel` | OTel provider, OTLP export, and the `Telemetry` recording surface |
| `pkg/integration/{cloud,observability}` | cloud-agnostic client abstraction + its middleware |
| `pkg/config/dynamic` | file-watching feature flags (opt-in; the engine does not wire it) |
| `pkg/utilities/*` | logger, circuit_breaker, retry_backoff, resilience, task_executor, validation, error_handler, app_profile, helpers, file_utils |
| `pkg/testutil` | `MockLogger` and context helpers — the doubles that carry no adapter dependency |
| `provider/otel` | OTel as an `engine.Provider` |
| `preset/http` | preset for a plain HTTP service |

### Family modules

| Directory | Service |
|---|---|
| `aws/pkg/clients/{cognito,sqs,sns,ses,s3,ssm}` | AWS clients; Cognito covers auth, MFA, JWT validation |
| `aws/pkg/database/dynamo` | DynamoDB |
| `aws/pkg/integration/aws` | AWS facade with observability, plus `adapters/` and `inbound/` |
| `messaging/pkg/integration/{kafka,rabbitmq,grpc}`, `messaging/pkg/server/grpc` | brokers and gRPC |
| `http/pkg/rest` | HTTP client via go-resty, with retry and circuit breaker composed in |
| `database/sql/pkg/database/gormsql` | GORM (Postgres/MySQL/SQLite/SQLServer) |
| `database/{redis,mongodb,memcached}/pkg/database/*` | the remaining stores |

Each family also holds `<module>/provider/<name>` and, where useful, its own
`testutil` with the mocks for its clients.

Multiple named instances of a client are declared as `[]map[string]Config` in
the YAML (`sqs_clients`, `redis_clients`, …). The singular form is not read by
any provider.

### Conventions

- Every client package exposes a `Service` interface and a `NewClient`/`NewService` constructor, with `entity.go` for types and `service.go` for the implementation.
- Every adapter also exposes an `engine.Provider` in its family's `provider/` directory.
- `pkg/core/client` provides `SafeTypeAssert[T]` used for safe type assertions across the framework.
- `pkg/testutil` provides `MockLogger`; adapter mocks live in their family's `testutil`.
- Comments are in English and explain the *why*, not the *what*.
- Test files use `testify/assert` and `testify/mock`; mocks live in `mocks_test.go` or `helpers_test.go` alongside the package under test.
