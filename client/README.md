# client — experimental candidate, not released

Independent typed composition for I/O on Go 1.27.4. Only stdlib, including tests;
no require entries. No engine, logger, configuration reader, SDK, retry, timeout,
registry or lifecycle. Use client only where an I/O boundary is decorated;
concurrency/config/utilities do not import it.

## One call

```go
result, err := client.Do(ctx, client.Invocation{
    Component: "repository",
    Operation: "Fetch",
}, func(ctx context.Context) (Record, error) {
    return transport.Fetch(ctx)
})
```

The operation's T and error pass through. A nonzero result accompanying an error
is preserved. No middleware means exactly one call with the same context,
including an already canceled context. Cancellation is interpreted by the
operation. client adds no deadline or retry and never substitutes Background.

## Prebuilt chain

```go
terminal := func(ctx context.Context, _ client.Invocation,
    operation client.Operation[Record]) (Record, error) {
    return operation(ctx)
}
chain, err := client.Chain(client.Handler[Record](terminal), first, second)
if err != nil { return err }
result, err := chain(ctx, invocation, operation)
```

The caller writes the terminal: no base handler or validation helper is exported.
Only Do validates invocation arguments. The prebuilt chain assumes a valid
context, labels and operation supplied by its caller; it does not revalidate.
`Chain(base, A, B)` constructs B then A, once each, and executes
A.before → B.before → base → B.after → A.after. Chain without middleware returns
next unchanged. It does not modify or retain the supplied middleware slice.
Do constructs its chain on every call; reuse a prebuilt chain when appropriate.
There is no mutable Use method, automatic cache or global state.

## Public API

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

ErrInvalidCall is the local sentinel; use errors.Is. Do validates ctx, operation,
Component, Operation, then construction in that order. Labels must be 1..128
bytes; whitespace, NUL and invalid UTF-8 are allowed. No trimming, normalization
or charset rules. Errors identify field or middleware index and applicable byte
length, never label values. Invalid Do returns zero T without running operation.
Chain rejects nil next, nil middleware or a nil returned handler, constructing
from last to first. Construction does not invoke the handler/operation. Side
effects of already-invoked middleware constructors are not rolled back.

Invocation is copied by value, contains only fixed low-cardinality labels and
has no payload/metadata. The caller must avoid IDs, secrets and resolved paths
in these labels; client cannot prove cardinality. User middleware may short-circuit,
transform invocation/result/context or invoke next multiple times. client cannot
promise exactly-once execution against arbitrary wrappers. Use durable application
mechanisms when external effects require replay safety; do not wrap individual
transaction statements to imply atomicity.

Panics from construction, handlers or operations propagate unchanged. No automatic
logging, instrumentation, goroutine, timer, connection or listener is created.
Caller middleware owns its synchronization, deadlines and resources; a derived
context should preserve parent cancellation/values and error causes. Concurrent
reuse requires thread-safe caller state. The module does not close resources
borrowed by a wrapper and does not introduce an artificial Close method.

## Independent examples and validation

[minimal](../examples/standalone/client/minimal) exercises typed calls, chain,
validation and original context/errors. [composed](../examples/standalone/client/composed)
uses an injected in-memory HTTP RoundTripper, two wrappers and explicit deadlines,
without network access. Both have independent go.mod files, one local replace and
run offline with GOWORK=off/-mod=readonly. No config/concurrency/engine import.

From repository root, Go 1.27.4 in PATH:

```sh
make check-client check-client-quick check-client-stress check-client-fuzz check-client-bench
```

CLIENT_RESULTS_DIR controls log output (default a temporary directory).
CLIENT_FUZZ_TIME defaults 30s per target; CLIENT_BENCH_TIME defaults 500ms with
three repetitions. Stress is separate from quick and checks concurrent reuse,
alternating outcomes/cancellation, weak references, heap plateau and the standard
goroutineleak profile. No third-party testing library. Benchmarks separate building
with Do from prebuilt chains; measured allocations are observations, not an SLA.

Inventory, D35, commands, results, source hashes and E1–E7 are in
[RELEASE_NOTES.md](RELEASE_NOTES.md). Migration is explicit; legacy BaseClient
remains in place. Optional logging/OTel/resilience wrappers are separate future
modules, not APIs supplied by this candidate.
