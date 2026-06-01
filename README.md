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
- [Messaging — when to use what](#messaging--when-to-use-what)
- [Core packages](#core-packages)
- [Observability middleware](#observability-middleware)
- [SQL and hexagonal architecture](#sql-and-hexagonal-architecture)
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

## Messaging — when to use what

| Pattern | Use when | go-engine implementation | Example |
|---|---|---|---|
| **REST** | Synchronous request/response; caller needs the result immediately | `GetRestClient(name)` — go-resty + circuit breaker | Handler calls Payment API and returns the charge ID in the same HTTP response |
| **SQS** | Fire-and-forget tasks; AWS-native; ordering not critical; simple retry via visibility timeout | `GetSQSClientByName(name)` | assessment-service enqueues a "generate-report" job after exam submission |
| **Kafka** | High-throughput event streaming; replay; multiple independent consumers; ordered per-key | `GetKafkaProducer()` / `GetKafkaConsumer()` | assessment-service publishes `ExamCompleted`; analytics-service and notification-service consume independently |
| **gRPC** | Low-latency internal service calls; strongly typed contracts; bidirectional streaming | `engine.GrpcServer` (server) · `GetGRPCClient(name)` (client) | calibration-service (Go/R sidecar) exposes `CalibrateIRT(ItemResponses) → Parameters` |
| **RabbitMQ** | Flexible routing (exchanges, routing keys); on-prem or non-AWS deployments | `GetRabbitMQClientByName(name)` | Notification fanout to email, SMS, and push channels via topic exchange |

> **Rule of thumb for EduCore:** use Kafka for domain events (things that happened),
> SQS for work items (things to do), gRPC for service-to-service calls where latency
> matters, and REST for public-facing or third-party APIs.

### Kafka

`messaging/pkg/integration/kafka` — producer, consumer, and health checker built on
segmentio/kafka-go v0.4.51.

#### YAML configuration

```yaml
kafka:
  brokers: ["kafka:9092"]
  group_id: "assessment-service"
  topic: "exam-events"
  dlq_topic: "exam-events-dlq"  # empty → log failed messages instead
  max_retries: 3
  retry_backoff: 1s
  commit_interval: 0            # 0 = synchronous commit after every message
  async: false                  # true = fire-and-forget (higher throughput)
```

#### Publishing events

```go
producer := engine.GetKafkaProducer()

err := producer.Publish(ctx,
    kafka.Message{
        Key:   []byte(examID),        // same key → same partition → ordered
        Value: jsonBytes,
        Headers: map[string]string{
            "event-type": "ExamCompleted",
            "trace-id":   traceID,
        },
    },
)
```

#### Consuming events

```go
consumer := engine.GetKafkaConsumer()

go func() {
    err := consumer.Subscribe(ctx, func(ctx context.Context, msg kafka.Message) error {
        var event ExamCompletedEvent
        if err := json.Unmarshal(msg.Value, &event); err != nil {
            return err // triggers retry → DLQ after MaxRetries
        }
        return processEvent(ctx, event)
    })
    if err != nil {
        log.Error(ctx, err, nil)
    }
}()
```

**Consumer behaviour:**
- Retries the handler up to `max_retries` times with linear backoff.
- On final failure, forwards to `dlq_topic` (with `x-original-topic`, `x-original-offset`, `x-error`, `x-failed-at` headers) or logs the error if no DLQ is configured.
- Commits the offset after every message (success or failure) — at-least-once delivery.
- Returns `nil` when `ctx` is cancelled (graceful shutdown).

#### Health check

```go
builder.RegisterHealthChecker("kafka", engine.GetKafkaProducer().(health.Checker))
// or directly:
builder.RegisterHealthChecker("kafka", kafka.NewChecker(cfg.Brokers))
```

### gRPC server

`messaging/pkg/server/grpc` — gRPC server with reflection enabled.

#### YAML configuration

```yaml
grpc_server:
  puerto: 50051          # field name is "puerto" (legacy Spanish naming)
  enable_logging: true
```

#### Registering and starting

```go
engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithRouter().
    Build()

grpcSrv := engine.GrpcServer
grpcSrv.RegisterService(func(s *grpc.Server) {
    pb.RegisterCalibrationServiceServer(s, &myCalibrationImpl{})
})

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := grpcSrv.Start(ctx); err != nil {
    log.Fatal(err)
}
// Start is non-blocking. Cancel ctx to trigger GracefulStop.
```

#### Adding interceptors

The current `NewServer` creates a plain `*grpc.Server` without interceptors.
To add auth, tracing, or logging interceptors, construct the server manually
and inject it via `WithCustomClient`:

```go
import "google.golang.org/grpc"

grpcSrv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        myAuthInterceptor,
        myLoggingInterceptor,
    ),
)
// Register services directly on grpcSrv, then manage lifecycle manually.
engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithCustomClient("grpc-server", grpcSrv).
    WithRouter().
    Build()
```

## Core packages

`pkg/core` contains two foundational packages used both by the framework itself and by
consumer services that need to build or register their own clients.

### `pkg/core/client` — BaseClient

`BaseClient` is an embeddable struct that provides timeout management, structured logging,
and an optional resilience layer (retry + circuit breaker) to any client implementation.
Embed it instead of reimplementing these cross-cutting concerns in every service client.

```go
import "github.com/skolldire/go-engine/pkg/core/client"

type IRTScorerClient struct {
    client.BaseClient          // logging + timeout + resilience for free
    baseURL string
}

func NewIRTScorerClient(cfg IRTScorerConfig, log logger.Service) *IRTScorerClient {
    return &IRTScorerClient{
        BaseClient: *client.NewBaseClientWithName(
            client.BaseConfig{
                EnableLogging:  true,
                WithResilience: false,
                Timeout:        cfg.Timeout,
            },
            log,
            "irt-scorer",   // appears as "service" in every log entry
        ),
        baseURL: cfg.BaseURL,
    }
}

func (c *IRTScorerClient) Score(ctx context.Context, responses []int) (*IRTScoreResponse, error) {
    raw, err := c.Execute(ctx, "irt-scorer.score", func() (interface{}, error) {
        return callScoringAPI(c.baseURL, responses)
    })
    if err != nil {
        return nil, err
    }
    return client.SafeTypeAssert[*IRTScoreResponse](raw) // safe, non-panicking cast
}
```

**`Execute` context rules:**
- If the caller's context already has a deadline → that deadline is used unchanged.
- If the caller's context has no deadline → `BaseConfig.Timeout` (default 10 s) is applied.

**`SafeTypeAssert[T]`** — converts the `interface{}` returned by `Execute` to a concrete type.
Returns an error (never panics) if the assertion fails.

### Injecting a custom client into the engine

```go
scorer := NewIRTScorerClient(cfg, log)

engine, err := app.NewAppBuilder().
    WithDynamicConfig().
    WithCustomClient("irt-scorer", scorer).  // store under any name
    WithRouter().
    Build()

// Retrieve it anywhere:
raw := engine.GetCustomClient("irt-scorer")
scorer, err := client.SafeTypeAssert[*IRTScorerClient](raw)
```

### `pkg/core/registry` — ClientFactory Registry

`Registry` is the process-wide singleton that the framework uses internally to map
client-type names to `ClientFactory` functions. A factory is invoked lazily when
`Create` is called.

**Behaviour contract:**
- `Register` → error if the type is already registered; does **not** overwrite.
- `Create` → error if the type has not been registered.
- `Unregister` → error if the type is not currently registered.
- All methods are thread-safe (`sync.RWMutex`).

```go
import "github.com/skolldire/go-engine/pkg/core/registry"

reg := registry.GetRegistry() // always the same singleton

// Register a factory (typically at init time or in RegisterDefaultClients):
reg.Register("irt-scorer", func(ctx context.Context, cfg interface{}, log logger.Service) (interface{}, error) {
    scorerCfg := cfg.(IRTScorerConfig)
    return NewIRTScorerClient(scorerCfg, log), nil
})

// Create an instance on demand:
instance, err := reg.Create(ctx, "irt-scorer", IRTScorerConfig{BaseURL: "..."})
scorer := instance.(*IRTScorerClient)
```

> **Note:** most consumer services do not use the `Registry` directly. Use
> `WithCustomClient` / `GetCustomClient` instead (see above). The `Registry` is
> intended for framework-level client wiring inside go-engine itself.

## Observability middleware

`pkg/integration/observability` provides three `cloud.Middleware` decorators that add logging, metrics, and tracing to any `cloud.Client` (the engine's AWS facade). They are composed as a chain around the base client.

```
Request → [Logging] → [Metrics] → [Tracing] → cloud.Client → AWS SDK
             ↓             ↓           ↓
          CloudWatch    OTLP/Grafana  OTLP/Grafana
```

### Composition pattern

```go
import (
    "github.com/skolldire/go-engine/pkg/integration/cloud"
    "github.com/skolldire/go-engine/pkg/integration/observability"
    awsclient "github.com/skolldire/go-engine/aws/pkg/integration/aws"
)

// The engine pre-wires the chain via awsclient.NewWithOptions.
// For manual composition:
base := awsclient.NewWithOptions(awsCfg)

chain := cloud.Chain(base,
    observability.Logging(engine.GetLogger()),
    observability.Metrics(observability.NewTelemetryMetricsRecorder(engine.GetTelemetry())),
    observability.Tracing(myTelemetryTracer),
)
```

### What each middleware records

**`Logging`** — logs every AWS operation to the engine's `logger.Service`:
- Fields on success: `operation`, `service`, `verb`, `path`, `status_code`, `duration_ms`, `aws_request_id` (when present).
- Fields on error: above + `error_code`, `error_message`, `retriable`.
- **Trace correlation**: when an active OTel span is present in the context, `trace.id` and `span.id` are automatically injected into every log entry.

**`Metrics`** — records to `telemetry.Telemetry` (OTLP):
- `aws.request.duration` (histogram, seconds) — tagged with `operation` and `status_code`.
- `aws.request.count` (counter) — every call.
- `aws.request.error` (counter) — calls with status ≥ 400.
- `aws.request.retry` (counter) — explicit retry events.
- `aws.request.throttle` (counter) — calls that hit rate limits.

**`Tracing`** — creates a child OTel span per AWS call:
- Span name: `{service}.{operation}` (e.g. `sqs.send_message`).
- Attributes: `aws.service`, `aws.operation`, `aws.path`, `http.status_code`, `aws.request_id`, `aws.error_code`.

### HTTP-level observability

For HTTP request tracing (not AWS), use `pkg/telemetry/otel.NewMiddleware`:

```go
// In AppBuilder:
engine, err := app.NewAppBuilder().
    WithDynamicConfig().
    WithOTEL(otelCfg).
    WithRouter().
    WithMiddleware(func(r router.Service) {
        r.Use(otel.NewMiddleware(otelCfg))  // traces every HTTP request
    }).
    Build()
```

This middleware is backed by `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` and handles W3C TraceContext propagation (`traceparent` header) automatically.

## SQL and hexagonal architecture

The `database/sql` sub-module (`gormsql.DBClient`) wraps GORM behind 15 methods
(`Create`, `First`, `Find`, `Update`, `Delete`, `Transaction`, `Where`, `Preload`, etc.)
and translates `gorm.ErrRecordNotFound` to `gormsql.ErrNotFound`. It is injected into
the engine via `WithCustomClient` — the domain never sees GORM.

### Enforcement

`make lint-arch` scans `pkg/` for any `"gorm.io/gorm"` import and exits 1 if found.
Run it in CI alongside `make test`.

### Anti-pattern — GORM leaking into the domain

```go
// ❌ assessment-service/internal/usecase/scoring.go
import "gorm.io/gorm"               // VIOLATION: domain depends on infrastructure

type ScoringUseCase struct {
    db *gorm.DB                     // VIOLATION: GORM handle in a use case
}

func (u *ScoringUseCase) GetItem(id string) (*Item, error) {
    var item Item
    u.db.First(&item, "id = ?", id) // VIOLATION: GORM query in business logic
    return &item, nil
}
```

Problems: the use case cannot be unit-tested without a real database; swapping
the persistence layer requires changing domain code; `gorm.ErrRecordNotFound`
leaks into business logic.

### Correct pattern — domain interface, GORM confined to the adapter

**Step 1 — domain port (no go-engine, no GORM)**

```go
// assessment-service/internal/domain/port/output/item_repository.go
package output

type ItemRepository interface {
    FindByID(ctx context.Context, id string) (*Item, error)
    FindByExamID(ctx context.Context, examID string) ([]Item, error)
    Save(ctx context.Context, item *Item) error
}
```

**Step 2 — use case depends only on the interface**

```go
// assessment-service/internal/usecase/scoring.go
package usecase

type ScoringUseCase struct {
    items output.ItemRepository     // interface, not *gorm.DB
}

func (u *ScoringUseCase) GetItem(ctx context.Context, id string) (*Item, error) {
    return u.items.FindByID(ctx, id) // testable with any mock
}
```

**Step 3 — adapter imports GORM and go-engine, implements the port**

```go
// assessment-service/internal/adapter/output/postgres/item_repository.go
package postgres

import (
    "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
    "assessment-service/internal/domain/port/output"
)

type itemModel struct {             // GORM model stays inside the adapter
    ID         string `gorm:"primaryKey"`
    ExamID     string
    Difficulty float64
}

type postgresItemRepository struct {
    db *gormsql.DBClient
}

func NewItemRepository(db *gormsql.DBClient) output.ItemRepository {
    return &postgresItemRepository{db: db}
}

func (r *postgresItemRepository) FindByID(ctx context.Context, id string) (*Item, error) {
    var m itemModel
    if err := r.db.First(ctx, &m, "id = ?", id); err != nil {
        if errors.Is(err, gormsql.ErrNotFound) {
            return nil, ErrItemNotFound   // domain error, not gorm.ErrRecordNotFound
        }
        return nil, err
    }
    return toDomain(&m), nil             // convert persistence model → domain entity
}

func (r *postgresItemRepository) FindByExamID(ctx context.Context, examID string) ([]Item, error) {
    var models []itemModel
    if err := r.db.Where(ctx, &models, "exam_id = ?", examID); err != nil {
        return nil, err
    }
    return toDomainSlice(models), nil
}

func (r *postgresItemRepository) Save(ctx context.Context, item *Item) error {
    return r.db.Create(ctx, toModel(item))
}
```

**Step 4 — wire everything in `main.go`**

```go
import (
    "github.com/skolldire/go-engine/pkg/app"
    gormsql "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func main() {
    // Build the GORM connection outside go-engine (GORM is not auto-initialized).
    gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil { log.Fatal(err) }

    client, err := gormsql.New(gormsql.Config{MaxOpenConnections: 20}, gormDB.Dialector, appLog)
    if err != nil { log.Fatal(err) }

    engine, err := app.NewAppBuilder().
        WithDynamicConfig().
        WithCustomClient("items-db", client).   // store under a name
        WithRouter().
        Build()
    if err != nil { log.Fatal(err) }

    // In the adapter factory / dependency injection root:
    rawDB := engine.GetCustomClient("items-db")
    dbClient, _ := client.SafeTypeAssert[*gormsql.DBClient](rawDB)
    itemRepo := postgres.NewItemRepository(dbClient)

    // Wire itemRepo into use cases…
}
```

### Transactions spanning multiple repositories

```go
// The adapter holds the raw *gormsql.DBClient, which exposes Transaction().
func (r *postgresOrderRepository) PlaceOrder(ctx context.Context, order *Order) error {
    return r.db.Transaction(ctx, func(tx *gorm.DB) error {
        if err := tx.Create(toOrderModel(order)).Error; err != nil {
            return err
        }
        return tx.Model(&inventoryModel{}).
            Where("id = ?", order.ItemID).
            Update("stock", gorm.Expr("stock - 1")).Error
    })
}
```

`gormsql.DBClient.Transaction` wraps `*gorm.DB` in the callback — this is intentional.
Multi-entity transactions must stay in the adapter layer; the domain orchestrates them
via a service method that calls the adapter's transaction-aware method directly.

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
