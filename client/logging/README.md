# client/logging (unreleased candidate)

D42/D42.1–D42.4 approved: typed completion middleware for client I/O, depending only on client and stdlib. The backend is a borrowed consumer-side interface, not our logging factory or slog.Handler.

```go
type Config struct {
    Level string
    IncludeDuration bool
}
type Logger interface {
    Enabled(context.Context, slog.Level) bool
    LogAttrs(context.Context, slog.Level, string, ...slog.Attr)
}
type Dependencies struct { Logger Logger }
func DefaultConfig() Config
func (Config) Validate() error
func New[T any](Config, Dependencies) (client.Middleware[T], error)
type ConfigError struct { Field, Reason string }
var ErrInvalidConfig error
```

Config has mapstructure/json snake_case tags. Defaults are debug/true. Only debug/info are accepted. Literal zeros are validated unchanged; false disables duration. New validates Config before Logger, reports field-only errors, supports errors.Is(ErrInvalidConfig)/errors.As(*ConfigError) and rejects nil plus *slog.Logger(nil). Other typed nil implementations are the application's responsibility.

New does not query the logger/clock, emit, execute next, start background work or change globals. It copies Config and borrows Logger; no Close, ownership transfer or backend configuration. Applying its middleware to nil returns a nil handler. client.Chain already rejects a nil terminal. Do validates calls; a manually executed chain assumes valid ctx/Invocation/operation.

On invocation, Enabled gates all observation work. Enabled normal returns emit exactly one LogAttrs attempt with message `operation completed` and fields component, operation, success, optional duration (time.Duration). Level is fixed for both success and errors. Duration includes next and its inner middleware, excludes final emission and uses monotonic Now/Since. Disabled and IncludeDuration=false paths do not read the clock for measurement. No start record, payload/result/error text, context extraction, severity classification or timeout/retry is added.

The default debug level is filtered by a standard info-threshold logger. Select Config.Level="info" or explicitly configure the backend for debug when completion records are desired; New never changes that backend policy. The wrapper retains no per-call history; a caller-owned backend may retain records or buffer them under its own limits.

The exact context, Invocation and operation go to next; result and error return unchanged, including partial results, identity and errors.Is/As. Canceled contexts still reach next. Panic/Goexit do not produce completion records; no recover/deferred logging is used. Logger panics propagate before/after I/O as they occur, potentially preventing delivery of an already-completed result. Handler errors ignored by slog.Logger do not replace I/O results. The backend may filter again on emission, buffer/drop/write multiple records or block; this middleware guarantees an emission attempt, not durable delivery or exactly one physical write. Loggers and next handlers must be concurrency-safe for concurrent reuse.

Invocation labels are caller-owned static low-cardinality names, without IDs/tenant/secrets. This wrapper does not inspect labels beyond client.Do's checks or redact arbitrary backend attributes.

## Connect a backend

| Backend | Connection | Validation |
|---|---|---|
| Standard slog JSON/text | slog.New(slog.NewJSONHandler/TextHandler(writer, opts)) | tested in module and minimal consumer |
| Application logger without slog.Handler | Implement Enabled and LogAttrs directly | tested slice logger and disabled implementation |
| Our independent logging module | logging.New(cfg,deps) returns *slog.Logger | tested only in composed consumer, not module dependency |
| Logrus with slog bridge | slog.New(handler provided by bridge) | illustrative; no bridge module imported/tested here |
| zap with zapslog | slog.New(handler supplied by zapslog) | illustrative, no version pinned or dependency/test claim |
| OpenTelemetry with otelslog | slog.New(handler supplied by otelslog) | illustrative, no version pinned or dependency/test claim |

Backend-specific constructors/options are deliberately not assumed. Nothing in execution asserts *slog.Logger or calls Handler(). Only construction recognizes the specific nil *slog.Logger case.

## Composition

```go
mw, err := clientlogging.New[Record](clientlogging.DefaultConfig(),
    clientlogging.Dependencies{Logger: logger})
if err != nil { return err }
value, err := client.Do(ctx, client.Invocation{
    Component: "repository", Operation: "fetch",
}, operation, mw)
```

For one logical observation with resilience, pass the operation already returned by resilience.Wrap to client.Do with this middleware outside it. That duration includes admission, retry attempts and backoff. Explicitly placing observation inside each attempt produces per-attempt events instead. Neither order is discovered or changed automatically. Do not duplicate the middleware unintentionally.

## Verification

From root use scripts/check-module.sh client/logging deps|consumers|quick|stress|fuzz|bench|all. Campaign prefix is CLIENTLOGGING; durations and results use CLIENTLOGGING_FUZZ_TIME, CLIENTLOGGING_BENCH_TIME, CLIENTLOGGING_RESULTS_DIR. Standard tools, synctest, race, goroutineleak, memory-backed integration, three independent Logger implementations, fuzzing and benchmarks exercise the contract. See RELEASE_NOTES.md for executed commands, source hashes, platforms, limitations and E1–E7. No stable tag is declared.

## D44 backend policy

Slog is the library standard. This module still accepts its two-method Logger interface, which *slog.Logger satisfies. See [Elegir backend](../../logging/README.md#elegir-backend-d44) for native slog, the redacting logging.New factory and application-owned foreign adapters. There is no planned logging/logrus module; foreign backends do not inherit logging.New's redaction or limits.

D43.5 uses the same clock at start and end: now().Sub(start). Production passes time.Now, preserving monotonic duration semantics; private fake-clock tests verify an exact7s advance and unchanged no-measurement paths.
