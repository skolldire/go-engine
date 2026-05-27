# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run all tests
go test ./... -v

# Run a single package's tests
go test ./pkg/clients/cognito/... -v

# Run a single test
go test ./pkg/clients/cognito/... -v -run TestMFA

# Clear test cache before running
go clean -testcache && go test ./... -v

# Initialize/setup (runs init.sh)
make init

# Full cycle: init + test
make all
```

## Architecture

`go-engine` is a reusable Go framework library (module `github.com/skolldire/go-engine`) consumed by applications via `go get`. It is not a runnable binary itself.

### Core assembly: `pkg/app`

The entry point for consumers is `app.NewAppBuilder()`, which implements a fluent builder pattern:

```
NewAppBuilder()
  .WithContext(ctx)
  .WithConfigs()        // loads config/application.yaml via Viper → populates Engine.Conf
  .WithInitialization() // constructs all clients from config → populates Engine.Services
  .WithRouter()         // creates chi-based HTTP router → populates Engine.Router
  .Build()              // returns *Engine or error
```

`WithDynamicConfig()` is an alternative to `WithConfigs()` that also starts a file watcher for live config reloads.

**`Engine`** (`pkg/app/entity.go`) is the central struct returned to consumers. It exposes typed getters for every registered client (e.g., `GetSQSClientByName`, `GetRedisClientByName`, `GetRestClient`).

**`ServiceRegistry`** (`pkg/app/registry.go`) is a composition struct inside `Engine` that holds all named client maps (`RESTClients`, `SQSClients`, `DynamoDBClients`, etc.). Thread-safe lazy initialization via `sync.Once`.

**`ConfigRegistry`** holds the four config buckets consumed by Clean Architecture layers: `Repositories`, `UseCases`, `Handlers`, `Batches`.

### Configuration: `pkg/config`

- `pkg/config/viper` — wraps Spf13/Viper; reads `config/application.yaml`. The `viper.Config` struct is the single source of truth for all service configurations.
- `pkg/config/dynamic` — `FeatureFlags` with file-watch-based live updates via fsnotify.

### Client packages

Each client package follows the same structure:
- `entity.go` — `Config` struct + interface definition (`Service`)
- `service.go` — `NewClient(cfg, log) Service` constructor + implementation

| Directory | AWS/External service |
|---|---|
| `pkg/clients/cognito` | Cognito: auth, MFA (TOTP/SMS), JWT validation, session management |
| `pkg/clients/sqs`, `sns`, `ses`, `s3`, `ssm` | AWS messaging and storage |
| `pkg/clients/rest` | HTTP client via go-resty with circuit breaker |
| `pkg/clients/grpc` | gRPC client |
| `pkg/clients/rabbitmq` | RabbitMQ via amqp091-go |
| `pkg/database/dynamo` | DynamoDB |
| `pkg/database/gormsql` | GORM (Postgres/MySQL/SQLite/SQLServer) |
| `pkg/database/redis` | Redis via go-redis/v9 |
| `pkg/database/mongodb` | MongoDB |
| `pkg/database/memcached` | Memcached |

Multiple named instances of the same client type are supported via `[]map[string]Config` in the Viper config (e.g., `sqs_clients`, `redis_clients`). The single-instance fields (`sqs`, `redis`, etc.) are legacy and exist for backward compatibility.

### Integration: `pkg/integration`

- `pkg/integration/aws` — `awsclient.Client` facade that wraps AWS SDK with observability (metrics + tracing).
- `pkg/integration/aws/adapters` — adapts the facade for Lambda, SQS, and other specific use cases.
- `pkg/integration/inbound` — normalizes inbound events (e.g., `NormalizeAPIGatewayEvent`).
- `pkg/integration/observability` — `MetricsRecorder` backed by OpenTelemetry.
- `pkg/integration/cloud` — cloud-agnostic abstraction layer.

### Utilities: `pkg/utilities`

| Package | Purpose |
|---|---|
| `logger` | Logrus wrapper with ECS format |
| `telemetry` | OpenTelemetry metrics + tracing (OTLP/gRPC export) |
| `circuit_breaker` | gobreaker wrapper |
| `retry_backoff` | Exponential backoff retry |
| `task_executor` | Bounded concurrency worker pool |
| `validation` | go-playground/validator global instance |
| `error_handler` | Centralized error types |
| `resilience` | Combined resilience primitives |

### Server: `pkg/server/grpc`

gRPC server (separate from the gRPC client in `pkg/clients/grpc`). Initialized when `grpc_server` is present in config.

### Conventions

- Every package exposes a `Service` interface and a `NewClient`/`NewService` constructor.
- `pkg/core/client` provides `SafeTypeAssert[T]` used for safe type assertions across the framework.
- `pkg/core/registry` holds a global client factory registry used by `RegisterDefaultClients`.
- `pkg/testutil` provides `MockLogger` for use in tests.
- Test files use `testify/assert` and `testify/mock`; mocks live in `mocks_test.go` or `helpers_test.go` alongside the package under test.
