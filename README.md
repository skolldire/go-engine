# go-engine

[![Go Version](https://img.shields.io/github/go-mod/go-version/skolldire/go-engine)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![CI](https://github.com/skolldire/go-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/skolldire/go-engine/actions/workflows/ci.yml)

Go framework for enterprise microservices. Provides a fluent builder that wires AWS clients, databases, messaging, health checks, and observability into a single `*Engine` handle. Not a runnable binary — consumed via `go get`.

---

## Modules

`go-engine` is a **set of Go modules**, one per adapter family. A consumer
downloads the core plus only the families it imports: an HTTP-only service
resolves 13 external modules instead of the 53 the single-module layout forced
on everyone.

| Module | Import path | Contains |
|---|---|---|
| **core** | `github.com/skolldire/go-engine` | `pkg/engine` (Provider core), `pkg/router`, `pkg/health`, `pkg/core`, `pkg/utilities`, `pkg/telemetry/otel`, `pkg/integration/{cloud,observability}`, `pkg/testutil`, `provider/otel`, `preset/http` |
| **aws** | `github.com/skolldire/go-engine/aws` | Cognito, SQS, SNS, SES, S3, SSM, DynamoDB, the AWS facade, their providers and `aws/preset` |
| **messaging** | `github.com/skolldire/go-engine/messaging` | Kafka, RabbitMQ, gRPC client/server and their providers |
| **http** | `github.com/skolldire/go-engine/http` | REST client (`http/pkg/rest`) and its provider |
| **database/sql** | `github.com/skolldire/go-engine/database/sql` | GORM wrapper (`gormsql.DBClient`) and its provider |
| **database/redis** | `github.com/skolldire/go-engine/database/redis` | Redis client (go-redis/v9) and its provider |
| **database/mongodb** | `github.com/skolldire/go-engine/database/mongodb` | MongoDB client and its provider |
| **database/memcached** | `github.com/skolldire/go-engine/database/memcached` | Memcached client and its provider |
| **preset/full** | `github.com/skolldire/go-engine/preset/full` | The everything preset; depends on every module above **except `database/sql`** (see below) |

The four database engines are separate modules rather than one because their
drivers share no dependency: a service using Redis has no reason to resolve
GORM's dialects or the MongoDB driver.

Local development uses the `go.work` at the repository root, so a change to the
core is visible to every family without a tagged release in between.

Each family has its own README with configuration reference and usage examples:
[`aws/`](aws/README.md) · [`messaging/`](messaging/README.md) · [`database/sql/`](database/sql/README.md) · [`database/redis/`](database/redis/README.md) · [`database/mongodb/`](database/mongodb/README.md) · [`database/memcached/`](database/memcached/README.md)

---

## Quick Start

The core knows no adapter. Each one implements `engine.Provider` in its own
package and is registered explicitly, or in bulk through a preset.

```bash
go get github.com/skolldire/go-engine
go get github.com/skolldire/go-engine/preset/full   # only if you want everything
```

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/skolldire/go-engine/aws/provider/sqs"
    "github.com/skolldire/go-engine/pkg/engine"
    "github.com/skolldire/go-engine/pkg/health"
    "github.com/skolldire/go-engine/pkg/router"
    presetfull "github.com/skolldire/go-engine/preset/full"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
    go func() { <-sig; cancel() }()

    // The preset supplies router, health probes and every component the YAML
    // declares; anything after it refines that baseline.
    opts := append(presetfull.Options(),
        engine.WithHealthConfig(health.Config{Timeout: 5 * time.Second}),
        engine.WithMiddleware(func(r router.Service) {
            r.Use(router.JWTAuth(router.JWTAuthConfig{
                JWKSURL:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
                Issuer:    "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
                Audience:  "your-client-id",
                SkipPaths: []string{"/health", "/ping", "/live", "/ready"},
            }))
        }),
    )

    eng, err := engine.New(ctx, opts...)
    if err != nil {
        os.Exit(1)
    }
    defer func() { _ = eng.Close(ctx) }()

    eng.Router().AddRoute("GET", "/users", usersHandler)

    // Typed retrieval replaces the old getters: the type travels with the caller.
    orders, err := sqs.From(eng, "orders")
    if err != nil {
        os.Exit(1)
    }
    _ = orders

    if err := eng.Run(ctx); err != nil {
        os.Exit(1)
    }
}
```

An HTTP-only service registers no adapter at all and needs the core module only:

```go
import (
    "github.com/skolldire/go-engine/pkg/engine"
    presethttp "github.com/skolldire/go-engine/preset/http"
)

eng, err := engine.New(ctx, presethttp.Options()...)
```

Migrating from the removed `pkg/app` builder: [`docs/migration-engine.md`](docs/migration-engine.md).

---

## Configuration (application.yaml)

```yaml
log:
  level: "info"        # debug | info | warn | error

router:
  port: "8080"
  # Durations are Go duration strings. A bare number is nanoseconds:
  # `read_timeout: 10` means 10ns, not 10 seconds.
  read_timeout: 10s
  write_timeout: 30s
  shutdown_timeout: 30s
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
      enable_logging: true

kafka:
  brokers: ["kafka:9092"]
  group_id: "my-service"
  topic: "events"

telemetry:
  service_name: "my-service"
  exporter_endpoint: "localhost:4317"   # OTLP/gRPC
  sampling_rate: 0.1
  enabled: true
  insecure: true                        # TLS by default; explicit opt-out
```

A section with no registered provider builds nothing. That is what lets a
service not pay for what it does not use, but it also means a forgotten
`WithProvider` shows up as a missing component rather than an error —
`eng.ComponentNames()` lists what was actually built.

### Why SQL is not in `preset/full`

Every other adapter can be built from configuration alone. GORM cannot: it needs
a `gorm.Dialector`, and picking one from a string such as `"postgres"` would mean
importing the Postgres, MySQL, SQLite and SQL Server dialects together, so every
consumer would link all four to use one.

The driver is therefore supplied by the caller, and the SQL provider is
registered explicitly rather than discovered:

```go
import (
    sqlprovider "github.com/skolldire/go-engine/database/sql/provider/sql"
    "gorm.io/driver/postgres"
)

eng, err := engine.New(ctx,
    engine.WithProvider(sqlprovider.New("main", postgres.Open(dsn))),
)

db, err := sqlprovider.From(eng, "main")
```

Its `sql_clients` section holds pool and behaviour settings only; the DSN travels
with the dialector.

Full schema: see [CLAUDE.md](CLAUDE.md).

---

## Engine API

| Option | What it does |
|---|---|
| `engine.WithConfigDir(dir)` | Directory to read the YAML from (default: `CONF_DIR`, else `./config`) |
| `engine.WithConfigFiles(names...)` | File names to read inside that directory |
| `engine.WithConfig(cfg)` | Supplies an already-decoded configuration |
| `engine.WithLogger(l)` | Injects an external logger |
| `engine.WithTelemetry(t)` | Injects the telemetry every provider records through |
| `engine.WithRouter()` | Creates the chi router |
| `engine.WithMiddleware(fn)` | Runs `fn` against the router once it exists |
| `engine.WithHealth()` | Creates the health service with defaults |
| `engine.WithHealthConfig(cfg)` | Same, with an explicit `health.Config` |
| `engine.WithProvider(p...)` | Registers adapters explicitly |
| `engine.WithProviderFunc(fn)` | Registers adapters derived from the configuration — how presets discover instances |

Options carry no ordering rules: `engine.New` applies the steps in dependency
order, not in call order.

```go
eng.Router()          // router.Service (AddRoute, Use, Mount)
eng.Health()          // *health.HealthService
eng.Logger()          // logger.Service
eng.Telemetry()       // engine.Telemetry
eng.Component(name)   // (any, bool)
eng.ComponentNames()  // everything that was actually built
eng.RegisterCloser(name, fn)
eng.Run(ctx)
eng.Close(ctx)
```

### Typed retrieval

Each provider exposes a `From` helper, so the type travels with the caller
instead of with a getter on the engine:

```go
orders, err := sqs.From(eng, "orders")     // sqs.Service
cache,  err := redis.From(eng, "cache")    // *redis.RedisClient
api,    err := rest.From(eng, "api")       // rest.Service
auth,   err := cognito.From(eng)           // cognito.Service

// And for anything else, including your own components:
thing, err := engine.Get[*MyThing](eng, "mything")
```

Asking for a component that does not exist, or with the wrong type, returns an
error listing what *is* registered rather than a silent `nil`.

### Writing your own adapter

No core change is needed — implement `engine.Provider` in your own package. The
contract and a worked example are in
[`docs/migration-engine.md`](docs/migration-engine.md).

---

## Health checks

`GET /health`, `/live` and `/ready` are mounted automatically when both
`engine.WithRouter()` and `engine.WithHealth()` are in play — every preset
includes them. Providers register their own checker during `Init`.

```go
// Available checkers — no external import needed for SQL and Redis:
health.NewSQLChecker(db)                          // any type with Ping(ctx) error
health.NewRedisChecker(client)                    // any type with Ping(ctx) error
health.NewHTTPChecker("https://svc/ping", 2*time.Second)

// Custom checker:
eng.Health().RegisterCheck("my-dep", func(ctx context.Context) error { return nil })
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
result, err := svc.Execute(ctx, func(ctx context.Context) (any, error) {
    return callExternalAPI(ctx)
})
```

Database and AWS clients accept `WithResilience: true` in their `Config` to
enable this automatically. The REST client does not: it composes the two
decorators separately through its `retry` and `circuit_breaker` blocks, so a
nil block means that decorator is simply not installed.

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
error_handler.HandleApiErrorResponse(err, w, eng.Logger())
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
// Wire as a provider (reads the `telemetry:` section of the YAML):
import otelprovider "github.com/skolldire/go-engine/provider/otel"

p := otelprovider.New()
eng, _ := engine.New(ctx, engine.WithProvider(p), engine.WithTelemetry(p.Telemetry()))

// HTTP middleware (auto-propagates W3C traceparent header):
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
eng.Router().Use(pkgotel.NewMiddleware(cfg))
```

---

## JWT authentication

`pkg/router` provides a chi middleware that validates RS256 Bearer tokens offline using JWKS public key caching. No Cognito SDK call is made per request — keys are fetched once and cached (default TTL: 1 h).

```go
import "github.com/skolldire/go-engine/pkg/router"

// 1. Install it on the router:
eng, _ := engine.New(ctx,
    engine.WithRouter(),
    engine.WithMiddleware(func(r router.Service) {
        r.Use(router.JWTAuth(router.JWTAuthConfig{
            JWKSURL:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX/.well-known/jwks.json",
            Issuer:    "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXX",
            Audience:  "your-app-client-id",
            SkipPaths: []string{"/health", "/ping", "/live", "/ready"},
            CacheTTL:  time.Hour,
        }))
    }),
)

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
r := eng.Router()
r.With(router.RequireGroup("admins")).Get("/admin/users", adminHandler)
r.With(router.RequireGroup("teachers", "admins")).Get("/content", contentHandler)
```

**`Claims` fields:** `Sub`, `Email`, `Username` (`cognito:username`), `Groups` (`cognito:groups`), `TokenUse` (`"id"` or `"access"`), `Raw` (full payload map for custom attributes like `custom:school_id`).

**Error responses** use the same `error_handler.CommonApiError` shape as the rest of the API (`{"code","msg","details":{"reason":...}}`). The `details.reason` field carries a stable machine-readable value:
- Missing / malformed header → `401 {"code":"ER-401","msg":"authentication token is missing","details":{"reason":"missing_token"}}`
- Invalid token → `401 {"code":"ER-401","msg":"authentication token is invalid","details":{"reason":"invalid_token"}}`
- Expired token → `401 {"code":"ER-401","msg":"authentication token has expired","details":{"reason":"expired_token"}}`
- Wrong group → `403 {"code":"ER-403","msg":"access forbidden: insufficient permissions","details":{"reason":"forbidden"}}`

## Components the engine does not build

Anything can be registered as a component and retrieved with the same typed
lookup, including a connection the engine knows nothing about:

```go
import (
    gormsql "github.com/skolldire/go-engine/database/sql/pkg/database/gormsql"
    "github.com/skolldire/go-engine/pkg/engine"
)

dbClient, _ := gormsql.New(gormsql.Config{MaxOpenConnections: 20}, dialector, log)

eng, _ := engine.New(ctx, presethttp.Options()...)
eng.RegisterCloser("main-db", func(ctx context.Context) error { return dbClient.Close() })

db, err := engine.Get[*gormsql.DBClient](eng, "main-db")
```

There is no `gormsql` provider: the dialector is a Go value, not something a
YAML file can name, so the connection is built by the application and handed to
the engine.

See [`database/sql/README.md`](database/sql/README.md) for the hexagonal
architecture pattern.

---

## Messaging — when to use what

| Pattern | Use when | go-engine | Example |
|---|---|---|---|
| **REST** | Sync request/response; caller needs the result now | `rest.From(eng, name)` | Payment charge, third-party API |
| **SQS** | Fire-and-forget tasks; AWS-native; simple retry | `sqs.From(eng, name)` | Enqueue report generation after exam |
| **Kafka** | High-throughput events; replay; multiple consumers | `kafka.From(eng)` | ExamCompleted → analytics + notifications |
| **gRPC** | Low-latency internal calls; typed contracts | `grpcclient.From(eng, name)` / `grpcserver.From(eng)` | Calibration sidecar (IRT parameters) |
| **RabbitMQ** | Flexible routing; on-prem or non-AWS | `rabbitmq.From(eng, name)` | Notification fanout (email + SMS + push) |

Details: [`messaging/README.md`](messaging/README.md)

---

## Architecture enforcement

Most of the boundary is now structural: the core is its own module, so it
cannot import an adapter without a `require` line appearing in its `go.mod`.
`make lint-arch` reads that manifest, plus the two ways the boundary can still
be crossed inside the core module.

```bash
make lint-arch   # fails if the core module resolves an adapter SDK or family module
make lint        # golangci-lint over every module
make test        # go test -race in every module
make lint-deps   # external dependency footprint, per module
make tidy        # go mod tidy in every module, then go work sync
```

---

## Repository conventions

- `entity.go` — structs, interfaces, constants. `service.go` — implementation. `service_test.go` — tests.
- Multi-instance clients use `[]map[name]Config` in YAML (e.g. `redis_clients`). The singular form is not read by any provider.
- `client.SafeTypeAssert[T](raw)` — safe type assertion on `any` values.
- Comments explain *why*, not *what*. No godoc that restates the function name.
- Minimum test coverage: 80% per package. Use `testify/assert` + `testify/mock`.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Contributing

See [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md).

## License

MIT.
