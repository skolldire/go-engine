# go-engine

[![Go Version](https://img.shields.io/github/go-mod/go-version/skolldire/go-engine)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/skolldire/go-engine)](https://goreportcard.com/report/github.com/skolldire/go-engine)
[![Go Reference](https://pkg.go.dev/badge/github.com/skolldire/go-engine.svg)](https://pkg.go.dev/github.com/skolldire/go-engine)
[![CI](https://github.com/skolldire/go-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/skolldire/go-engine/actions/workflows/ci.yml)

Go framework for enterprise microservices following Clean Architecture. Integrates AWS, databases, messaging, and observability through a fluent builder. **Not a runnable binary** — it is a library consumed by services via `go get`.

---

## Table of Contents

- [Module structure](#module-structure)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [YAML configuration](#yaml-configuration)
- [Builder API](#builder-api)
- [Engine getters](#engine-getters)
- [Health checks](#health-checks)
- [Utilities](#utilities)
- [Lambda](#lambda)
- [Recommended project structure](#recommended-project-structure)
- [Tests](#tests)
- [Repository conventions](#repository-conventions)
- [Changelog](CHANGELOG.md) · [Contributing](.github/CONTRIBUTING.md)

---

## Module structure

The repository uses **multiple Go modules** to keep optional dependencies out of the root `go.mod`.

| Module | Path | Contents |
|---|---|---|
| `github.com/skolldire/go-engine` | `/` (root) | Core: builder, router, health, config, utilities |
| `github.com/skolldire/go-engine/aws` | `aws/` | Cognito, SQS, SNS, SES, S3, SSM, DynamoDB, AWS facade |
| `github.com/skolldire/go-engine/messaging` | `messaging/` | Kafka, RabbitMQ, gRPC client/server |
| `github.com/skolldire/go-engine/database/redis` | `database/redis/` | Redis client (go-redis/v9) |
| `github.com/skolldire/go-engine/database/mongodb` | `database/mongodb/` | MongoDB client |
| `github.com/skolldire/go-engine/database/memcached` | `database/memcached/` | Memcached client |

> Sub-modules are referenced via `replace` directives in the root `go.mod` during local development.
> SQL/GORM is **not a sub-module** — it is injected via `WithCustomClient`.

### Root module package tree

```
pkg/
├── app/                    # Core: Engine, AppBuilder, ServiceRegistry
│   ├── builder.go          # Fluent builder — entry point for all consumers
│   ├── entity.go           # Engine struct + all getter methods
│   ├── registry.go         # ServiceRegistry and ConfigRegistry
│   ├── service.go          # Client initialization (Init, InitializeRouter)
│   ├── build/              # Request/response construction helpers
│   └── router/             # chi wrapper: Config, Service interface, App
├── health/                 # Health checks + HTTP handler
│   ├── entity.go           # Interfaces, response types, constants
│   ├── checkers.go         # SQLChecker, RedisChecker, HTTPChecker
│   ├── service.go          # HealthService (concurrent execution, timeout)
│   └── handler.go          # HTTPHandler: /health, /live, /ready, /deps
├── config/
│   ├── viper/              # Reads config/application.yaml → viper.Config
│   └── dynamic/            # FeatureFlags with file-watch (fsnotify)
├── clients/
│   └── rest/               # HTTP client (go-resty + circuit breaker)
├── core/
│   ├── client/             # SafeTypeAssert[T]
│   └── registry/           # Global client factory registry
├── integration/
│   ├── observability/      # MetricsRecorder (OpenTelemetry)
│   └── cloud/              # Cloud-agnostic abstraction layer
├── telemetry/otel/         # OTLP/gRPC provider (metrics + traces)
├── testutil/               # MockLogger for use in tests
└── utilities/
    ├── logger/             # Logrus wrapper with ECS format
    ├── telemetry/          # OpenTelemetry helpers
    ├── circuit_breaker/    # gobreaker wrapper
    ├── retry_backoff/      # Exponential backoff retry
    ├── task_executor/      # Bounded concurrency worker pool
    ├── validation/         # go-playground/validator (global instance)
    ├── error_handler/      # Centralized error types
    ├── resilience/         # Combined resilience primitives
    └── helpers/            # General utility functions
```

---

## Installation

```bash
# Root module
go get github.com/skolldire/go-engine

# Optional sub-modules (only if needed)
go get github.com/skolldire/go-engine/aws
go get github.com/skolldire/go-engine/messaging
go get github.com/skolldire/go-engine/database/redis
go get github.com/skolldire/go-engine/database/mongodb
go get github.com/skolldire/go-engine/database/memcached
```

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/skolldire/go-engine/pkg/app"
    "github.com/skolldire/go-engine/pkg/health"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
    go func() { <-sig; cancel() }()

    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithDynamicConfig().           // reads config/application.yaml
        WithHealth(health.Config{Timeout: 5 * time.Second}).
        RegisterHealthChecker("db", health.NewSQLChecker(myDB)).
        WithInitialization().          // constructs all clients from YAML
        WithRouter().                  // chi router; auto-mounts GET /health
        Build()
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }

    router := engine.GetRouter()
    router.AddRoute("GET", "/users", usersHandler)

    if err := engine.Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

---

## YAML configuration

File: `config/application.yaml`. All sections are optional — omit what you do not use.

```yaml
log:
  level: "info"          # debug | info | warn | error
  path: "logs/app.log"   # empty → stdout

router:
  port: "8080"
  read_timeout: 10       # seconds
  write_timeout: 30
  idle_timeout: 120
  shutdown_timeout: 30
  enable_cors: false
  cors_config:
    allow_origins: ["*"]
    allow_methods: ["GET","POST","PUT","DELETE","OPTIONS"]
    allow_headers: ["Authorization","Content-Type"]

aws:
  region: "us-east-1"
  endpoint: ""           # LocalStack: "http://localhost:4566"

cognito:
  region: "us-east-1"
  user_pool_id: "us-east-1_XXXXXXXXX"
  client_id: "your-client-id"
  client_secret: ""
  enable_logging: true
  timeout: 30

# Single-instance clients (legacy — prefer the plural form)
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  max_retries: 3

sqs:
  endpoint: "http://localhost:4566"
  wait_time: 20

dynamo:
  endpoint: "http://localhost:4566"
  table_prefix: "dev_"

# Multi-instance clients (format: []map[name]Config)
redis_clients:
  - cache:
      addr: "localhost:6379"
      db: 0
  - session:
      addr: "localhost:6379"
      db: 1

sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"
      wait_time: 20
  - notifications:
      endpoint: "http://localhost:4566"
      wait_time: 10

sns_clients:
  - alerts:
      endpoint: "http://localhost:4566"

dynamo_clients:
  - main:
      endpoint: "http://localhost:4566"
      table_prefix: "app_"

s3_clients:
  - assets:
      region: "us-east-1"
      bucket: "my-assets"

ses_clients:
  - transactional:
      region: "us-east-1"
      from_email: "noreply@example.com"

ssm_clients:
  - config:
      region: "us-east-1"

rest:
  - payments-api:
      base_url: "https://payments.internal"
      timeout: 10
      headers:
        Content-Type: "application/json"

grpc_client:
  - auth-service:
      address: "auth:50051"
      timeout: 5

rabbitmq_clients:
  - events:
      url: "amqp://guest:guest@localhost:5672/"
      exchange: "events"

mongodb_clients:
  - analytics:
      uri: "mongodb://localhost:27017"
      database: "analytics"

memcached_clients:
  - cache:
      servers: ["localhost:11211"]

kafka:
  brokers: ["localhost:9092"]
  group_id: "my-service"
  topic: "events"

grpc_server:
  port: "50051"

feature_flags:
  enabled: true
  file_path: "config/features.yaml"
  watch: true

telemetry:
  service_name: "my-service"
  otlp_endpoint: "localhost:4317"

# Per-layer configs (Clean Architecture)
repositories: {}   # map[string]interface{}
cases: {}          # use cases
endpoints: {}      # handlers
processors: {}     # batch jobs
```

---

## Builder API

`pkg/app/builder.go` — all methods return `*AppBuilder` for chaining.

| Method | What it does | Requires |
|---|---|---|
| `NewAppBuilder()` | Creates the builder with background context | — |
| `WithContext(ctx)` | Replaces the root context | — |
| `WithConfigs()` | Reads `config/application.yaml` once (no file-watch) | — |
| `WithDynamicConfig()` | Reads YAML + starts fsnotify for live-reload | — |
| `SetLogger(log)` | Injects an external logger (overrides the one from `WithDynamicConfig`) | — |
| `WithHealth(cfg)` | Creates the `HealthService` with the given timeout | Logger |
| `RegisterHealthChecker(name, c)` | Registers a checker; initializes `HealthService` with defaults if needed | Logger |
| `WithInitialization()` | Instantiates all clients defined in the YAML | Config |
| `WithRouter()` | Creates the chi router; mounts `GET /health` if `HealthService` exists | Config |
| `WithMiddleware(fn)` | Applies a global middleware to the router | Router |
| `WithOTEL(cfg)` | Initializes the OTEL provider and registers its `Shutdown` hook | — |
| `WithCustomClient(name, client)` | Stores an arbitrary client in `Services.CustomClients` | — |
| `WithGracefulShutdown()` | No-op — graceful shutdown is built into `Router.Run()` | — |
| `Build()` | Validates and returns `*Engine` | Router |
| `GetErrors()` | Returns accumulated errors without building | — |

**Typical call order:**
```
WithContext → WithDynamicConfig → WithHealth → RegisterHealthChecker(s)
→ WithInitialization → WithOTEL → WithRouter → WithMiddleware → Build
```

`WithHealth` and `RegisterHealthChecker` may be called before or after `WithRouter`. The `GET /health` mount is idempotent and fires as soon as both the health service and the router are available.

---

## Engine getters

`pkg/app/entity.go` — access to all initialized components.

### Core

| Method | Return type |
|---|---|
| `GetContext()` | `context.Context` |
| `GetLogger()` | `logger.Service` |
| `GetConfig()` | `*viper.Config` |
| `GetRouter()` | `router.Service` |
| `GetErrors()` | `[]error` |
| `GetServices()` | `*ServiceRegistry` |
| `GetConfigs()` | `*ConfigRegistry` |
| `GetValidator()` | `*validator.Validate` |
| `GetFeatureFlags()` | `*dynamic.FeatureFlags` |
| `GetHealthService()` | `health.Service` |
| `GetOTELProvider()` | `pkgotel.Provider` |
| `GetTelemetry()` | `telemetry.Telemetry` |
| `Run()` | `error` |

### Named clients (multi-instance)

| Method | Return type |
|---|---|
| `GetRestClient(name)` | `rest.Service` |
| `GetGRPCClient(name)` | `grpcClient.Service` |
| `GetSQSClientByName(name)` | `sqs.Service` |
| `GetSNSClientByName(name)` | `sns.Service` |
| `GetDynamoDBClientByName(name)` | `dynamo.Service` |
| `GetRedisClientByName(name)` | `*redis.RedisClient` |
| `GetS3ClientByName(name)` | `s3.Service` |
| `GetSESClientByName(name)` | `ses.Service` |
| `GetSSMClientByName(name)` | `ssm.Service` |
| `GetMemcachedClientByName(name)` | `memcached.Service` |
| `GetMongoDBClientByName(name)` | `mongodb.Service` |
| `GetRabbitMQClientByName(name)` | `rabbitmq.Service` |
| `GetCustomClient(name)` | `interface{}` |

### Other clients

| Method | Return type | Note |
|---|---|---|
| `GetCognito()` | `cognito.Service` | |
| `GetCloudClient()` | `awsclient.Client` | AWS facade with observability |
| `GetGRPCServer()` | `grpcServer.Service` | |
| `GetKafkaProducer()` | `kafka.Producer` | |
| `GetKafkaConsumer()` | `kafka.Consumer` | |
| `GetSQSClient()` | `sqs.Service` | Legacy — prefer `GetSQSClientByName` |
| `GetRedisClient()` | `*redis.RedisClient` | Legacy — prefer `GetRedisClientByName` |

### Per-layer configs

```go
configs := engine.GetConfigs()
// or directly:
engine.GetRepositoryConfig("users-repo")  // interface{}
engine.GetUseCaseConfig("create-user")
engine.GetHandlerConfig("users-handler")
engine.GetBatchConfig("nightly-sync")
```

---

## Health checks

`pkg/health` — configurable health checks for ECS Fargate. The builder auto-registers `GET /health` when both `WithRouter` and `WithHealth`/`RegisterHealthChecker` have been called.

### Usage

```go
engine, err := app.NewAppBuilder().
    WithDynamicConfig().
    WithHealth(health.Config{Timeout: 5 * time.Second}).
    RegisterHealthChecker("postgres", health.NewSQLChecker(sqlDB)).
    RegisterHealthChecker("redis",    health.NewRedisChecker(redisClient)).
    RegisterHealthChecker("payments", health.NewHTTPChecker("https://payments.internal/ping", 2*time.Second)).
    WithRouter().
    Build()
```

### Available checkers

| Constructor | Accepts | Fails when |
|---|---|---|
| `health.NewSQLChecker(db)` | Any type with `Ping(ctx) error` | Ping returns an error |
| `health.NewRedisChecker(client)` | Any type with `Ping(ctx) error` | Ping returns an error |
| `health.NewHTTPChecker(url, timeout)` | URL and timeout | Status ≥ 500 or no response |

### Custom checker

```go
type myChecker struct{}

func (c *myChecker) Check(ctx context.Context) error {
    return nil // or a descriptive error
}

builder.RegisterHealthChecker("my-dep", &myChecker{})
```

### `GET /health` response

**200 — healthy:**
```json
{
  "status": "healthy",
  "checks": [
    { "name": "postgres", "status": "ok",  "latency_ms": 3 },
    { "name": "redis",    "status": "ok",  "latency_ms": 1 }
  ],
  "latency_ms": 4,
  "timestamp": "2026-06-01T12:00:00Z"
}
```

**503 — unhealthy:**
```json
{
  "status": "unhealthy",
  "checks": [
    { "name": "postgres", "status": "ok",    "latency_ms": 3 },
    { "name": "redis",    "status": "error", "error": "dial tcp: connection refused", "latency_ms": 5001 }
  ],
  "latency_ms": 5001,
  "timestamp": "2026-06-01T12:00:00Z"
}
```

### Additional endpoints (manual mount)

```go
// For specific liveness/readiness sub-routes:
router.Mount("/internal", health.NewHTTPHandler(engine.GetHealthService()).Routes())
// GET /internal/live  → 200 while the process is running
// GET /internal/ready → 200 if all dependencies are healthy
// GET /internal/deps  → JSON with per-dependency status ("up"/"down")
```

---

## Utilities

### Router

```go
r := engine.GetRouter()

r.Use(func(next http.Handler) http.Handler {   // global middleware
    return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
        next.ServeHTTP(w, req)
    })
})
r.AddRoute("GET",  "/users", listUsers)
r.AddRoute("POST", "/users", createUser)
r.Mount("/v2", v2Handler)                      // sub-router
r.Router()                                     // access the underlying *chi.Mux
```

### Feature flags

```go
flags := engine.GetFeatureFlags()
flags.IsEnabled("new-checkout")   // bool
flags.GetString("api-version")    // string
flags.GetInt("rate-limit")        // int
flags.Set("dark-mode", true)      // in-memory update
```

### Circuit breaker

```go
import "github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"

cb := circuit_breaker.New(circuit_breaker.Config{
    MaxRequests: 5,
    Interval:    time.Minute,
    Timeout:     30 * time.Second,
})
result, err := cb.Execute(func() (interface{}, error) { return callExternalAPI() })
```

### Retry with backoff

```go
import "github.com/skolldire/go-engine/pkg/utilities/retry_backoff"

r := retry_backoff.New(retry_backoff.Config{
    MaxRetries:   3,
    InitialDelay: time.Second,
    MaxDelay:     10 * time.Second,
})
err := r.Execute(func() error { return unstableOperation() })
```

### Task executor (worker pool)

```go
import "github.com/skolldire/go-engine/pkg/utilities/task_executor"

ex := task_executor.New(task_executor.Config{MaxConcurrency: 10, QueueSize: 100})
ex.Submit(func() { processItem(item) })
ex.Wait()
ex.Shutdown()
```

### SQL / GORM (manual injection)

SQL is not auto-initialized by the engine. Inject it via `WithCustomClient`:

```go
import "gorm.io/gorm"

db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})

engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithCustomClient("main-db", db).
    WithRouter().
    Build()

db := engine.GetCustomClient("main-db").(*gorm.DB)
```

---

## Lambda

```go
import (
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/skolldire/go-engine/aws/pkg/integration/inbound"
)

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    req, err := inbound.NormalizeAPIGatewayEvent(&event)
    if err != nil {
        return events.APIGatewayProxyResponse{StatusCode: 500}, nil
    }
    _ = req
    return events.APIGatewayProxyResponse{StatusCode: 200, Body: `{"ok":true}`}, nil
}

func main() { lambda.Start(handler) }
```

---

## Recommended project structure

```
my-service/
├── cmd/
│   └── api/
│       └── main.go
├── config/
│   ├── application.yaml
│   └── features.yaml
├── internal/
│   ├── domain/        # domain entities and errors
│   ├── repository/    # repository implementations
│   ├── usecase/       # business logic
│   └── handler/       # HTTP handlers (receive engine.GetRouter())
├── go.mod
└── go.sum
```

---

## Tests

```bash
# All tests in the root module
go test ./... -v

# Specific package
go test ./pkg/health/... -v

# With coverage
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

# Clear cache before running
go clean -testcache && go test ./...
```

Tests use `testify/assert` and `testify/mock`. Mocks live in `mocks_test.go` or `helpers_test.go` alongside the package under test. `pkg/testutil` provides a reusable `MockLogger`.

---

## Repository conventions

- Every package exposes a `Service` interface and a `NewClient`/`NewService` constructor.
- `entity.go` — structs, interfaces, constants. `service.go` — implementation. `service_test.go` — tests.
- Multiple instances of the same client type are configured via `[]map[string]Config` in the YAML (e.g. `redis_clients`). Singular fields (`redis`, `sqs`) are legacy.
- `pkg/core/client.SafeTypeAssert[T]` — use this for safe type assertions when retrieving clients via `GetCustomClient`.
- Comments document the **why**, not the what — well-named identifiers handle the what.

---

## License

MIT. See [LICENSE](LICENSE).

## Contributing

See [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md).

## Support

Open an issue on GitHub.
