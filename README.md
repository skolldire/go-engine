# go-engine

[![Go Version](https://img.shields.io/github/go-mod/go-version/skolldire/go-engine)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![CI](https://github.com/skolldire/go-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/skolldire/go-engine/actions/workflows/ci.yml)

Go framework for enterprise microservices. Provides a fluent builder that wires AWS clients, databases, messaging, health checks, and observability into a single `*Engine` handle. Not a runnable binary — consumed via `go get`.

---

## Modules

| Module | Import path | Details |
|---|---|---|
| **core** | `github.com/skolldire/go-engine` | AppBuilder, Engine, health, resilience, error_handler, app_profile, OTEL, observability |
| **aws** | `github.com/skolldire/go-engine/aws` | Cognito, SQS, SNS, SES, S3, SSM, DynamoDB, AWS facade |
| **messaging** | `github.com/skolldire/go-engine/messaging` | Kafka, RabbitMQ, gRPC client/server |
| **database/sql** | `github.com/skolldire/go-engine/database/sql` | GORM wrapper (`gormsql.DBClient`) |
| **database/redis** | `github.com/skolldire/go-engine/database/redis` | Redis client (go-redis/v9) |
| **database/mongodb** | `github.com/skolldire/go-engine/database/mongodb` | MongoDB client |
| **database/memcached** | `github.com/skolldire/go-engine/database/memcached` | Memcached client |

Each module has its own README with configuration reference and usage examples:
[`aws/`](aws/README.md) · [`messaging/`](messaging/README.md) · [`database/sql/`](database/sql/README.md) · [`database/redis/`](database/redis/README.md) · [`database/mongodb/`](database/mongodb/README.md) · [`database/memcached/`](database/memcached/README.md)

---

## Quick Start

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/skolldire/go-engine/pkg/app"
    "github.com/skolldire/go-engine/pkg/health"
    pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
    go func() { <-sig; cancel() }()

    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithDynamicConfig().                           // reads config/application.yaml + file-watch
        WithOTEL(pkgotel.OTELConfig{                  // optional: distributed tracing + metrics
            ServiceName:      "my-service",
            ExporterEndpoint: "localhost:4317",
            Enabled:          true,
        }).
        WithInitialization().                          // builds all clients declared in YAML
        WithRouter().                                  // chi router; mounts GET /health automatically
        WithHealth(health.Config{Timeout: 5 * time.Second}).
        RegisterHealthChecker("postgres", myDBChecker).
        RegisterHealthChecker("redis", myRedisChecker).
        WithJWTAuth(router.JWTAuthConfig{           // validate Bearer tokens
            JWKSEndpoint: "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
            Issuer:       "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
            Audience:     "your-client-id",
            SkipPaths:    []string{"/health", "/ping", "/live", "/ready"},
        }).
        Build()
    if err != nil {
        os.Exit(1)
    }

    engine.GetRouter().AddRoute("GET", "/users", usersHandler)
    engine.Run()
}
```

> `WithConfigs()` is legacy (no file-watch). Always use `WithDynamicConfig()`.

---

## Configuration (application.yaml)

```yaml
log:
  level: "info"        # debug | info | warn | error

router:
  port: "8080"
  read_timeout: 10     # seconds
  write_timeout: 30
  enable_cors: false

aws:
  region: "us-east-1"
  endpoint: ""         # LocalStack: "http://localhost:4566"

# Multi-instance clients — format: []map[name]Config
redis_clients:
  - cache:
      host: "localhost"
      port: 6379

sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"
      wait_time: 20

kafka:
  brokers: ["kafka:9092"]
  group_id: "my-service"
  topic: "events"

feature_flags:
  enabled: true
  file_path: "config/features.yaml"
  watch: true
```

Full schema: see [CLAUDE.md](CLAUDE.md).

---

## Builder API

| Method | What it does |
|---|---|
| `WithContext(ctx)` | Sets the root context |
| `WithDynamicConfig()` | Reads YAML + starts fsnotify file-watch |
| `WithConfigs()` | Reads YAML once — **legacy**, no file-watch |
| `SetLogger(log)` | Injects an external logger |
| `WithInitialization()` | Builds all clients declared in YAML |
| `WithRouter()` | Creates chi router; auto-mounts `GET /health` |
| `WithMiddleware(fn)` | Adds a global HTTP middleware |
| `WithOTEL(cfg)` | Initializes OTLP provider + registers Shutdown hook |
| `WithHealth(cfg)` | Creates the HealthService |
| `RegisterHealthChecker(name, checker)` | Adds a named checker; initializes HealthService if needed |
| `WithCustomClient(name, client)` | Stores any client in `Services.CustomClients` |
| `WithJWTAuth(cfg)` | Registers JWT Bearer validation middleware; must be called after `WithRouter` |
| `WithGracefulShutdown()` | No-op — graceful shutdown is built into `Router.Run()` |
| `Build()` | Returns `*Engine` or accumulated errors |

| `WithJWTAuth(cfg)` | Registers JWT Bearer validation middleware; must be called after `WithRouter` |

---

## Engine getters

```go
engine.GetLogger()                     // logger.Service
engine.GetRouter()                     // router.Service  (AddRoute, Use, Mount)
engine.GetConfig()                     // *viper.Config
engine.GetHealthService()              // health.Service
engine.GetOTELProvider()               // pkgotel.Provider
engine.GetFeatureFlags()               // *dynamic.FeatureFlags
engine.GetValidator()                  // *validator.Validate

// Named clients (populated from YAML)
engine.GetRestClient("api1")           // rest.Service
engine.GetRedisClientByName("cache")   // *redis.RedisClient
engine.GetSQSClientByName("orders")    // sqs.Service
engine.GetDynamoDBClientByName("main") // dynamo.Service
engine.GetS3ClientByName("assets")     // s3.Service
engine.GetSESClientByName("tx")        // ses.Service
engine.GetSSMClientByName("cfg")       // ssm.Service
engine.GetMongoDBClientByName("db")    // mongodb.Service
engine.GetRabbitMQClientByName("evts") // rabbitmq.Service
engine.GetGRPCClient("auth")           // grpcClient.Service
engine.GetKafkaProducer()              // kafka.Producer
engine.GetKafkaConsumer()              // kafka.Consumer
engine.GetCognito()                    // cognito.Service
engine.GetCustomClient("my-db")        // interface{}
```

---

## Health checks

`GET /health` is mounted automatically when `WithRouter` + `WithHealth`/`RegisterHealthChecker` are both called.

```go
// Available checkers — no external import needed for SQL and Redis:
health.NewSQLChecker(db)                          // any type with Ping(ctx) error
health.NewRedisChecker(client)                    // any type with Ping(ctx) error
health.NewHTTPChecker("https://svc/ping", 2*time.Second)

// Custom checker:
type myChecker struct{}
func (c *myChecker) Check(ctx context.Context) error { return nil }
builder.RegisterHealthChecker("my-dep", &myChecker{})
```

Response shape → see [`pkg/health/`](pkg/health/).

---

## Resilience

`pkg/utilities/resilience` wraps retry + circuit breaker into a single `Config`:

```go
import "github.com/skolldire/go-engine/pkg/utilities/resilience"
import "github.com/skolldire/go-engine/pkg/utilities/retry_backoff"
import "github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"

cfg := resilience.Config{
    RetryConfig: &retry_backoff.Config{
        MaxRetries:   3,
        InitialDelay: time.Second,
        MaxDelay:     10 * time.Second,
    },
    CircuitBreakerConfig: &circuit_breaker.Config{
        Name:        "payments",
        MaxRequests: 5,
        Timeout:     30 * time.Second,
    },
}
svc := resilience.NewResilienceService(cfg, log)
result, err := svc.Execute(ctx, func() (interface{}, error) {
    return callExternalAPI()
})
```

All database and HTTP clients accept `WithResilience: true` in their `Config` to enable this automatically.

---

## Error handling

`pkg/utilities/error_handler` provides typed API errors with HTTP code, structured fields, and JSON serialization:

```go
import "github.com/skolldire/go-engine/pkg/utilities/error_handler"

// Constructors
err := error_handler.NewNotFoundError("item not found", originalErr)
err := error_handler.NewBadRequestError("invalid payload", originalErr)
err := error_handler.NewUnauthorizedError("token expired", originalErr)
err := error_handler.NewInternalError("unexpected failure", originalErr)

// In an HTTP handler:
error_handler.HandleApiErrorResponse(err, w, engine.GetLogger())
```

Error codes: `ER-400`, `ER-401`, `ER-403`, `ER-404`, `ER-409`, `ER-422`, `ER-500`.

---

## App profile

`pkg/utilities/app_profile` reads the `SCOPE` environment variable to determine the deployment profile:

```go
import "github.com/skolldire/go-engine/pkg/utilities/app_profile"

app_profile.GetScopeValue()   // raw SCOPE value; defaults to "local"
app_profile.IsLocalProfile()  // SCOPE == "local"
app_profile.IsTestProfile()   // SCOPE ends with "test"
app_profile.IsProdProfile()   // SCOPE ends with "prod"
app_profile.IsStageProfile()  // SCOPE ends with "stage"
```

The router uses `IsProdProfile()` to decide whether to mount `/debug/pprof` routes — they are only active on non-production profiles.

---

## Observability middleware (AWS facade)

`pkg/integration/observability` wraps any `cloud.Client` to add logging, metrics, and tracing for AWS calls. See [Observability middleware](#) in the full docs.

| Middleware | Records |
|---|---|
| `Logging(log)` | operation, duration_ms, status_code, trace.id + span.id when OTel span is active |
| `Metrics(recorder)` | aws.request.duration (histogram), aws.request.count, aws.request.error, aws.request.throttle |
| `Tracing(tracer)` | child OTel span per AWS call with aws.service, aws.operation, http.status_code |

---

## OpenTelemetry

```go
pkgotel "github.com/skolldire/go-engine/pkg/telemetry/otel"

cfg := pkgotel.OTELConfig{
    ServiceName:      "assessment-service",
    ServiceVersion:   "1.0.0",
    ExporterEndpoint: "otel-collector:4317",  // OTLP/gRPC
    SamplingRate:     1.0,
    Enabled:          true,
}
// Wire via builder:
builder.WithOTEL(cfg)

// HTTP middleware (auto-propagates W3C traceparent header):
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
engine.GetRouter().Use(pkgotel.NewMiddleware(cfg))
```

---

## JWT authentication

`pkg/app/router` provides a chi middleware that validates RS256 Bearer tokens offline using JWKS public key caching. No Cognito SDK call is made per request — keys are fetched once and cached (default TTL: 1 h).

```go
import "github.com/skolldire/go-engine/pkg/app/router"

// 1. Wire in the builder (after WithRouter):
engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithRouter().
    WithJWTAuth(router.JWTAuthConfig{
        JWKSEndpoint: "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
        Issuer:       "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
        Audience:     "your-app-client-id",
        SkipPaths:    []string{"/health", "/ping", "/live", "/ready"},
        CacheTTL:     time.Hour,
    }).
    Build()

// 2. Read claims in any handler:
func getUser(w http.ResponseWriter, r *http.Request) {
    claims := router.ClaimsFromContext(r.Context())
    if claims == nil {
        // should not happen on non-skipped routes, but handle defensively
        http.Error(w, "unauthenticated", http.StatusUnauthorized)
        return
    }
    fmt.Fprintf(w, "user=%s groups=%v", claims.Sub, claims.Groups)
}

// 3. Restrict routes to specific Cognito groups:
r := engine.GetRouter()
r.With(router.RequireGroup("admins")).Get("/admin/users", adminHandler)
r.With(router.RequireGroup("teachers", "admins")).Get("/content", contentHandler)
```

**`Claims` fields:** `Sub`, `Email`, `Username` (`cognito:username`), `Groups` (`cognito:groups`), `TokenUse` (`"id"` or `"access"`), `Raw` (full payload map for custom attributes like `custom:school_id`).

**Error responses** use the same `error_handler.CommonApiError` shape as the rest of the API (`{"code","msg","details":{"reason":...}}`). The `details.reason` field carries a stable machine-readable value:
- Missing / malformed header → `401 {"code":"ER-401","msg":"authentication token is missing","details":{"reason":"missing_token"}}`
- Invalid token → `401 {"code":"ER-401","msg":"authentication token is invalid","details":{"reason":"invalid_token"}}`
- Expired token → `401 {"code":"ER-401","msg":"authentication token has expired","details":{"reason":"expired_token"}}`
- Wrong group → `403 {"code":"ER-403","msg":"access forbidden: insufficient permissions","details":{"reason":"forbidden"}}`

## WithCustomClient — external clients

```go
// Wire a GORM connection not managed by the engine:
import gormsql "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
import "github.com/skolldire/go-engine/pkg/core/client"

dbClient, _ := gormsql.New(gormsql.Config{MaxOpenConnections: 20}, dialector, log)

engine, _ := app.NewAppBuilder().
    WithDynamicConfig().
    WithCustomClient("main-db", dbClient).
    WithRouter().
    Build()

// Retrieve:
raw := engine.GetCustomClient("main-db")
db, _ := client.SafeTypeAssert[*gormsql.DBClient](raw)
```

See [`database/sql/README.md`](database/sql/README.md) for the hexagonal architecture pattern.

---

## Messaging — when to use what

| Pattern | Use when | go-engine | Example |
|---|---|---|---|
| **REST** | Sync request/response; caller needs the result now | `GetRestClient(name)` | Payment charge, third-party API |
| **SQS** | Fire-and-forget tasks; AWS-native; simple retry | `GetSQSClientByName(name)` | Enqueue report generation after exam |
| **Kafka** | High-throughput events; replay; multiple consumers | `GetKafkaProducer/Consumer()` | ExamCompleted → analytics + notifications |
| **gRPC** | Low-latency internal calls; typed contracts | `GetGRPCClient(name)` / `engine.GrpcServer` | Calibration sidecar (IRT parameters) |
| **RabbitMQ** | Flexible routing; on-prem or non-AWS | `GetRabbitMQClientByName(name)` | Notification fanout (email + SMS + push) |

Details: [`messaging/README.md`](messaging/README.md)

---

## Architecture enforcement

```bash
make lint-arch   # fails if gorm.io/gorm is imported in pkg/ (must stay in database/sql)
make lint        # runs golangci-lint
make test        # go test ./...
```

---

## Repository conventions

- `entity.go` — structs, interfaces, constants. `service.go` — implementation. `service_test.go` — tests.
- Multi-instance clients use `[]map[name]Config` in YAML (e.g. `redis_clients`). Singular fields are legacy.
- `client.SafeTypeAssert[T](raw)` — safe type assertion on `interface{}` values from `GetCustomClient`.
- Comments explain *why*, not *what*. No godoc that restates the function name.
- Minimum test coverage: 80% per package. Use `testify/assert` + `testify/mock`.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Contributing

See [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md).

## License

MIT.
