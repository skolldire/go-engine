# client — candidate, not released

Proposed version: client/v0.1.0, subject to E6 approval. No stable publication.

## Initial state

Branch feature/split. git status --porcelain: empty. Go 1.27.4 darwin/arm64.
The agent must not commit, push, tag or modify the index/release workflow.
Documentation here is versionable; ignored docs/ is only a local design reference.

## Inventory and migration map

| Origin | Existing functionality / destination | Treatment | Required proof |
|---|---|---|---|
| pkg/core/client/entity.go:BaseConfig | Logging/resilience flags, timeout with 10s default | Keep legacy; no Config in client | Legacy regression, absence of implicit timeout |
| entity.go:Operation/Execute | any output; derived context on every execution | Adapt to Operation[T]/Do; pass original ctx/result/error | Typed results, nonzero+error, context identity |
| entity.go:BaseClient/Use | Embedding, mutable chain with RWMutex | Keep legacy; new chain built explicitly | Reuse/concurrency, no mutable registry |
| entity.go:SetLogging/SetServiceName | Runtime state | Outside new client | No setters/global state |
| middleware.go:Invocation | Client/Operation metadata | Component/Operation value, no alias | Metadata copy, limits/no value leaks |
| middleware.go:Handler/Middleware/Chain | any chain, nil middleware skipped, outermost-first | Generic Chain(next,...), nil is error | Build/execution order, validation |
| helpers.go:SafeTypeAssert | Runtime assertion of any | Not needed by typed API; legacy intact | Multiple T without assertions |
| observability.go:WithLogging/Metrics/Tracing | Logger.Service and recorder/tracer wrappers | Future optional wrapper modules | No observability import in client |
| resilience.go:WithResilience* | Retry/breaker wrapping, service shared by code | Future resilience/wrapper modules | No retry or SDK in client |
| pkg/core/client/*_test.go | Legacy unit tests/examples/benchmarks | Preserve; equivalent new tests and benchmark fixture | make test + direct legacy tests |

Legacy comments say concurrent safety; this inventory does not silently fix or
redefine legacy behavior (e.g. Execute reads chain while Use may replace it).
The new module has no mutable chain setter and does not promise thread safety
for state captured by user middleware.

Consumer scan: 34 external Go files across 15 direct packages (the prompt's
"34 packages" is a file count, not a package count). Package paths:

- `aws/pkg/clients/cognito`
- `aws/pkg/clients/s3`
- `aws/pkg/clients/ses`
- `aws/pkg/clients/sns`
- `aws/pkg/clients/sqs`
- `aws/pkg/clients/ssm`
- `aws/pkg/database/dynamo`
- `database/memcached/pkg/database/memcached`
- `database/mongodb/pkg/database/mongodb`
- `database/redis/pkg/database/redis`
- `database/sql/pkg/database/gormsql`
- `database/sqlc/pkg/database/sqlcdb`
- `http/pkg/rest`
- `messaging/pkg/integration/grpc`
- `messaging/pkg/integration/rabbitmq`

The full workspace regression covers direct and indirect consumers; no consumer
is migrated here. Files with " 2" in their names are excluded from the task.

## Contract recorded before code — D35

```go
type Invocation struct {
    Component string
    Operation string
}
type Operation[T any] func(context.Context) (T, error)
type Handler[T any] func(context.Context, Invocation, Operation[T]) (T, error)
type Middleware[T any] func(Handler[T]) Handler[T]
func Chain[T any](next Handler[T], middleware ...Middleware[T]) (Handler[T], error)
func Do[T any](ctx context.Context, info Invocation, operation Operation[T],
    middleware ...Middleware[T]) (T, error)
```

ErrInvalidCall is the only exported sentinel and supports errors.Is.
No Config/Dependencies/New/Validate/base handler/Close/WithMiddleware is exported.

D35: only Do validates an invocation; a Chain handler does not revalidate ctx,
labels or operation. Caller supplies a terminal and owns invocation preconditions.
Do validation order: ctx, operation, Component, Operation, then Chain construction.
Only label byte lengths 1..128 are checked; no UTF-8/charset validation, trimming
or normalization. Errors identify field/index and length where relevant, never
label values. Invalid Do returns zero T and ErrInvalidCall without running op.

Chain first checks next, then builds from last middleware to first. Each valid
middleware is called once at construction, never by client at execution. A nil
middleware or produced nil handler stops at that index. Middleware already called
before a construction failure is not rolled back; callers own any resources or
side effects in their constructors. Valid Chain(base,A,B) executes A.before,
B.before,base,B.after,A.after. Empty Chain returns next unchanged. Slice not
modified or retained by the library after construction.

Do constructs each call and uses an unexported terminal returning operation(ctx)
exactly. It does not preempt an already canceled context; no background context,
timer, goroutine, retry, panic capture, logging or metrics. Result plus original
error, context values/deadline/cause and pointer/collection identities pass through
unless a caller middleware intentionally changes them. Invocation is passed by
value. Caller wrappers may short-circuit, transform or call next more than once;
there is no exactly-once promise against arbitrary middleware. Panics propagate.
Concurrent reuse requires caller handlers/middleware/operations to be safe.
Only I/O is decorated, never pool tasks, pure functions or individual Tx statements.
No import from config/concurrency/utilities into client or back (D26).

## Acceptance and evidence

| Step | Evidence |
|---|---|
| C1 | Inventory and D35 above, recorded before implementation |
| C2–C4 | Exact public types, ErrInvalidCall, Chain and Do with deterministic validation |
| C5 | Unit/composition tests: context/error/result identity, ordering, panics and concurrent reuse |
| C6 | Quick/stress/fuzz/benchmarks on the platforms below |
| C7 | Offline minimal/composed consumers; stdlib and inverse D26 guards |
| C8 | Versioned README/MIGRATION/CHANGELOG/release evidence and candidate hashes |

| Gate | State |
|---|---|
| E1 | Complete: contract, inventory, migration, ownership and validation |
| E2 | Complete locally: tests/race/stress/leaks/fuzz/benchmarks |
| E3 | Complete: two caller wrappers plus controlled in-memory I/O, no external integration required |
| E4 | Complete locally: separate consumers, GOWORK=off, GOPROXY=off, readonly graph/replace allowlists |
| E5 | Complete for first candidate: gorelease -base=none -version=v0.1.0, exit 0 |
| E6 | Deferred: manifest, exact approved commit and version authorization |
| E7 | Deferred: external consumers downloading the real published tag without replaces |

No stable acceptance/publication is declared. E6/E7 remain required.

No existing package is deprecated or replaced by this candidate.

## Executed validation

Go 1.27.4, macOS ARM64 (Apple M1 Max) and Linux ARM64. No AMD64 run.
Each platform: race x20 default/GOMAXPROCS=1/4; go vet; stress x3; fuzz 30s per
target with two workers. 14 Test functions (including separate stress cases),
two fuzz targets and executable example. Quick skips TestStress; stress explicitly
runs them, so no skipped case is counted as passed. All functional checks passed.

Stress per platform: 64 000 calls ×3 =192 000 mixed success/domain-error/canceled
calls, concurrent reuse and repeated construction, with exact build/run counters.
Retention: 960 chains of 64KiB ×3 =2880 per platform; weak pointers cleared after
GC, heap plateau within the fixture's 4MiB guard after warmup. This is not a hard
heap bound on user callbacks. Standard goroutineleak reports total 0.

| Fuzz target | macOS ARM64 | Linux ARM64 |
|---|---:|---:|
| Labels (30s) | 3,130,934 | 2,448,971 |
| Chain (30s) | 2,791,721 | 2,181,994 |

Total recorded: 10,553,620 fuzz executions, no failing input. Arbitrary label bytes
are accepted/rejected only by byte length; chain construction/execution order,
invalid positions and caller slice preservation are checked.

Observed statement coverage: 100% (diagnostic only, not evidence of correctness
by itself). Root make lint/lint-arch/lint-deps/lint-docs and check-client/config/concurrency
passed. Full make test passed for every MODULES entry, including all legacy
consumers. Direct go test -race -count=1 ./pkg/core/client/... also passed. The
workspace regression required authorized local httptest/gRPC listeners, without
external services. A redundant test variable type was corrected after ST1023;
the linter was rerun without suppression. No production implementation failure
was found by these campaigns.

### Comparative benchmarks

Same machine/Go/flags, same duplicated response{Status,Size}, caller deadline
one hour, nil logger and resilience disabled. Root benchmark uses actual
BaseClient and any; module benchmark uses typed Do. Legacy still derives a
cancel context and boxes output, while client intentionally preserves the caller
context. This is measured boundary overhead, not semantic equivalence or I/O SLA.
Three 500ms repetitions, medians:

| Boundary | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| BaseClient.Execute | 125.70 | 96.00 | 2.00 |
| client.Do[response] | 14.75 | 16.00 | 1.00 |

The full benchmark log separately measures direct op, terminal, Do, chains with
1/4/16 wrappers, structs and concurrent reuse (RunParallel). No zero-allocation
promise; Do reconstructs on every call. Module benchmarks used 500ms ×3 without
race; host may share CPU with the validation VM. The comparative pair was run
sequentially after other campaigns completed. All figures are fixture-specific.

## Reproduction and evidence files

With Go 1.27.4 in PATH and writable Go caches, from repository root:

```sh
CLIENT_RESULTS_DIR=/tmp/client-evidence ./scripts/check-client-production.sh all
make lint lint-arch lint-deps lint-docs check-client check-config check-concurrency
make test
go test -race -count=1 ./pkg/core/client/...
go test -run '^$' -bench '^BenchmarkLegacyBoundaryComparison$' -benchmem -count=3 -benchtime=500ms ./pkg/core/client
GOWORK=off go test -C client -run '^$' -bench '^BenchmarkTypedBoundaryComparison$' -benchmem -count=3 -benchtime=500ms
```

Production script exports GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/-mod=readonly.
CLIENT_FUZZ_TIME and CLIENT_BENCH_TIME extend campaigns; retain overrides with logs.
Linux container used --rm --network none --read-only, 4CPU/4GiB and executable
/tmp tmpfs 3GiB. Image base
sha256:e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602;
official Go 1.27.4 Linux ARM64 toolchain SHA256
3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92.
A cached dependency bundle was supplied only so inverse D26 checks can inspect
config/viper offline; it is not a dependency of client or either consumer.
Both Linux consumers passed without opening ports or using a network transport.

Logs: [evidence](evidence/), [Linux consumers](evidence/linux-arm64/consumers.log),
[workspace regression](evidence/workspace-regression.log),
[gorelease](evidence/gorelease.log), [source snapshot](SOURCE.sha256).
Empty vet.log is successful go vet output. Evidence/SHA256SUMS checksums the logs;
SOURCE.sha256 covers module source, go.mod and module documentation, excluding
execution evidence and itself to avoid recursive hashes. Gorelease ran outside
the module go.mod against those copied sources outside Git, no previous version
published, no fabricated comparison base. It created no tag.

Caveats: user callbacks may block, panic, retain memory or perform side effects;
client does not supervise them or own their resources. Fixed labels/cardinality
and thread-safe callback state remain caller responsibilities. A successful local
consumer replace is not E7. No imports were added from root to the new module,
and root go.mod/go.sum, concurrency/config production code and release workflow
were not changed by this task. Only a legacy test benchmark was added in root.
