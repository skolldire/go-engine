# concurrency

Status: experimental prototype, not an approved production API. Further
implementation is paused while the [complete modularization strategy](../docs/modularization.md)
is defined. See the [target concurrency contract](../docs/modularization/concurrency.md)
for admission, IDs, ordering, memory and shutdown requirements not yet covered here.

Independent Go module for bounded streaming concurrency. Uses only the standard
library, including in tests. No engine, logging SDK, configuration loader or
telemetry dependency. The first version will be available after a release tags
`concurrency/vX.Y.Z`; the workspace supports local development now.

```go
import "github.com/skolldire/go-engine/concurrency"
```

| Primitive | Contract |
|---|---|
| `Map(ctx, input, Config, fn)` | Fixed worker count; bounded output; backpressure; completion-order results |
| `From(ctx, values...)` | Unbuffered finite source |
| `FanOut(ctx, input, n, buffer)` | Shares items across streams exactly once, not broadcast |
| `FanIn(ctx, buffer, streams...)` | Merges streams; ignores nil inputs; closes after forwarders exit |
| `Batch(ctx, input, size, maxWait, buffer)` | Size/time flushing; final partial batch on normal input closure |
| `NewLimiter(limit)` / `Acquire(ctx)` | Shared concurrency cap; cancellable waiting; idempotent release |

Compose `Map` stages to build pipelines, including stages with different input
and output types. Pass the same cancellation context throughout. The runnable
[pipeline example](example_test.go) shows explicit propagation of stage errors.
Logging, metrics, rate limiting and application retries belong in the caller's
function or an optional integration package. `Limiter` caps simultaneous work;
it does not enforce requests per second.

## Resource and lifecycle contracts

- `Workers > 0`; buffers and timeouts are nonnegative. Zero buffer means
  synchronous handoff. Invalid configuration returns `ErrInvalidConfig`.
- Each primitive owns and closes its outputs. Callers own inputs and must not
  close outputs. Consume all output streams or cancel their context.
- `Map` has exactly `Workers` execution goroutines and one closing goroutine.
  At most `Workers` functions run at a time. The output holds at most `Buffer`
  results, with at most one additional result retained per worker. Caller-owned
  input queues and values' own memory remain the caller's responsibility.
- `TaskTimeout` signals cancellation to the task. A task that ignores its context
  keeps its worker occupied until it returns. No replacement execution is
  launched to escape the limit. There is no safe forced termination of arbitrary
  Go functions. Use process isolation if forced termination is required.
- Task errors and recovered panics (`errors.Is(err, ErrPanic)`) are results;
  they do not automatically cancel sibling work. Cancel the shared context for
  fail-fast behavior. Errors returned by a task are preserved; otherwise its
  expired context is reported. Panics in goroutines created by user code remain
  the responsibility of that code.
- Cancellation can discard pending input, results and partial batches; it is not
  a durable queue. A result already ready at the cancellation boundary may still
  be delivered. Outputs close after active tasks return.
- Batch timers offer partial batches after `maxWait`; backpressure may delay
  actual delivery. Every emitted slice owns separate storage.
- All contexts must be non-nil. Do not mutate slices passed to `From` until the
  source finishes. Call every acquired limiter release function.

## Migration from task_executor

`pkg/utilities/task_executor` remains compatible in the root module. It does not
wrap this module or acquire a new dependency. For standalone consumers, move to
`concurrency.Map`: replace task maps with a channel of typed jobs, put IDs in
those jobs/results when needed, and handle `Result.Err` explicitly. Priority
scheduling, result callbacks and collecting all results in a map remain caller
policies; the new module does not buffer the complete workload.

```sh
cd concurrency
GOWORK=off go test -race ./...
# From the repository root:
make check-concurrency
```
