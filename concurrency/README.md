# concurrency — release candidate, not released

Independent Go module for bounded local concurrency. Go 1.27.4; **zero require
entries**, including tests. No engine, client, config loader, logger, telemetry,
retry SDK or database. Proposed version `concurrency/v1.0.0`; stable acceptance
still requires E6 (version manifest and user authorization) and E7 (consumer
against the real published tag). No tag is created by this work.

See [candidate evidence](../docs/modules/concurrency.md),
[API contract](../docs/modularization/concurrency-api.md) and
[migration](../MIGRATION.md#experimental-concurrency-candidate-not-released).

## Managed jobs

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cfg := concurrency.DefaultPoolConfig()
cfg.Workers = 4
cfg.QueueCapacity = 16
cfg.MaxInFlightWeight = 20 // Default max individual weight is one.
pool, err := concurrency.NewPool(cfg,
    func(ctx context.Context, n int) (int, error) { return n * 2, ctx.Err() })
if err != nil { return err }
future, err := pool.Submit(ctx, concurrency.Job[int]{
    ID: "answer", Weight: 1, Value: 21,
})
if err != nil { /* close the pool using the application's shutdown context */ return err }
result, err := future.Wait(ctx)
// err cancels only this wait. The task's error is result.Err.
// On every exit path, the application owns pool.Shutdown with a bounded,
// separate shutdown context; cancel/escalate explicitly if draining times out.
```

Runnable lifecycle examples: [minimal consumer](../examples/standalone/concurrency/minimal)
and [composed consumer](../examples/standalone/concurrency/composed).

| Surface | Public contract |
|---|---|
| `PoolConfig`, `DefaultPoolConfig`, `Validate` | Explicit defaults, no replacement of literal zero values |
| `Job`, `Task`, `Result`, `Future` | Typed input/outcome; ID/key/sequence/timestamps preserved |
| `NewPool`, `Submit` | Fixed workers + one coordinator; block/reject; atomic count/weight/key/ID admission |
| `Future.Wait` / `Done` / `Cancel` | Repeatable outcome, independent waits, cooperative job cancellation |
| `Pool.Stats` / `Done` / `Shutdown` | Coherent counters, actual termination, drain/cancel/escalation |
| `Map`, `StreamConfig`, `StreamResult` | Completion-order bounded streaming; cancellation may discard results |
| `From`, `FanOut`, `FanIn` | Finite source, work distribution (not broadcast), unordered merge |
| `Batch`, `BatchConfig` | Item/weight/time flushing and separate storage per output batch |
| `Limiter`, `LimiterConfig` | Weighted FIFO local semaphore and idempotent release |

## Ownership and bounds

The caller owns input until Submit succeeds. Then the pool retains the input
until the Future finishes; do not mutate referenced data during that interval.
Futures own outputs; retaining them retains caller-owned memory. The pool keeps
no completed-ID cache, result history or output channel that can stall workers.

Weights are caller-supplied positive logical units; using bytes requires a
valid estimator. They limit queued+running inputs, **not arbitrary heap usage,
output values or caller goroutines**. QueueCapacity counts only pending jobs;
zero permits handoff only to a free worker. Blocking Submit keeps its input on
the caller's stack rather than an unbounded internal waiter list.

Per-key mode requires MaxActiveKeys, PerKeyCapacity and KeyFailure explicitly.
One task per key runs at a time; ready keys are scheduled round-robin. FIFO means
accepted order, not goroutine launch order. Retries in the task keep the same
key/slot. `cancel_pending` aborts jobs pending when a task fails or panics; it is
not a permanent account lock. Explicit cancellation does not trigger that policy.
ID uniqueness only covers live local jobs; it is not durable idempotency.

## Cancellation, outcomes and shutdown

Submit's context governs admission and accepted-job lifetime. AdmissionTimeout
only bounds admission. TaskTimeout signals the executing function but **does not
detach it** or release its slot early. Go cannot forcibly terminate arbitrary
code. Tasks must not call runtime.Goexit; child goroutine panics belong to user code.

Wait(ctx) can stop waiting without canceling a task. Future.Cancel requests job
cancellation. A canceled running task is not terminal until it returns. Canceled
queued jobs finish without execution and have zero StartedAt. Cancellation and
function errors remain reachable through errors.Is/As. Panics are PanicError
with bounded stack and no automatic logging; Value/Stack may need redaction.

Shutdown stops admission first. Drain runs accepted jobs under their original
contexts; cancel escalates to cancel pending and signal active jobs. The shutdown
context bounds only that call's wait. A timed-out drain leaves admission closed
and may be escalated/reawaited. Done closes only after workers exit. Abandoning a
Future does not block this lifecycle. The application owns every shutdown path.

## Streams and limiter

Callers own inputs; each primitive closes its outputs. Consume every output or
cancel the shared context. Map has fixed workers and bounded output; input buffer
sizes remain the caller's responsibility. There is no exactly-one outcome promise
on streaming cancellation. Use Pool/Future for accepted-job local tracking.

Batch flushes its final partial slice on normal input closure. Cancellation drops
it. Invalid/oversized weight emits a terminal StreamResult error and closes the
stage; pending partial data is discarded. The consumer must cancel upstream on
that error. MaxWait is time until offering, not delivery under backpressure.
Every batch owns its storage. Weight functions must be pure and nonpanicking.

Limiter reserves positive weights atomically. FIFO intentionally allows a large
head waiter to delay smaller ones. Waiting is cancelable; release is idempotent
and valid after its context ends. It limits simultaneous work, not requests/second
or work on other processes. Callers control how many waiting calls they launch.

## Production qualification

From the repository root, with Go 1.27.4 in PATH:

```sh
make check-concurrency-production
make check-concurrency
```

The production check is offline, uses only stdlib, and writes logs to a temporary
results directory. Set `CONCURRENCY_RESULTS_DIR` to retain them in a chosen path.
Individual stages are available through `scripts/check-concurrency-production.sh`
with `quick`, `stress`, `fuzz`, or `bench`. The full run includes race repetitions
at default/GOMAXPROCS=1/4, concurrent shutdown, sustained load, weak-reference
retention checks, goroutineleak profiles, two 60-second fuzz campaigns and repeated
benchmarks. `CONCURRENCY_FUZZ_TIME` and `CONCURRENCY_BENCH_TIME` adjust campaign
length; record overrides with the results. No network or services are required.

See [production qualification evidence](../docs/modules/concurrency-production.md)
for configurations, measured results, platform coverage and remaining release gates.
Benchmark figures describe synthetic workloads and hardware, not a production SLA.

## Tuning

Start from `DefaultPoolConfig()` and set resource budgets before constructing the
pool. A Go literal is validated as supplied; zero values are never silently defaulted.

| Setting | Operational effect |
|---|---|
| `Workers` | Maximum simultaneously executing tasks, including noncooperative tasks awaiting return |
| `QueueCapacity` | Maximum pending jobs; zero requires an available execution slot at acceptance |
| `Admission` | `reject` returns saturation immediately; `block` propagates pressure to submitting callers |
| `AdmissionTimeout` | Bounds blocked admission only; zero disables this additional timeout |
| `TaskTimeout` | Signals task cancellation; zero disables this additional timeout |
| `MaxJobWeight` / `MaxInFlightWeight` | Maximum individual weight and total accepted queued+running weight |
| `MaxActiveKeys` / `PerKeyCapacity` | Bound live keys and queued work per key in per-key mode |
| `KeyFailure` | Continue a key after failure or cancel its currently pending work |
| Stream/Batch `Buffer` | Bounds output buffering; a full buffer propagates backpressure |
| Batch `Size` / `MaxBatchWeight` / `MaxWait` | Bound item count, estimated weight and time until offering a batch |
| Limiter `Capacity` | Bounds shared weighted work across pools; FIFO may delay small requests behind a large one |

Use representative task durations and payload sizes when selecting these values.
Increasing workers cannot accelerate a single serialized key. Increasing queue size
absorbs bursts while increasing waiting time and retained inputs. If weights model
bytes, include retained input data in the estimate; outputs and arbitrary task
allocations remain outside this budget. Bound producer goroutines in the application.

Sample `Stats()` for accepted, terminal, running, queued, rejected and weight counters;
export them through the application's chosen observability stack. A sustained full
queue indicates that admission exceeds completion capacity. Treat task errors and
cancellations separately; `Wait` errors describe the wait rather than `Result.Err`.

On shutdown stop producers, call drain with a separate bounded context, then request
cancel if required and await actual completion. A noncooperative task can delay
completion indefinitely; no timeout releases its resource reservation early. Cancel
stream contexts when abandoning output, and always release limiter acquisitions.
Use durable application mechanisms for retries of uncertain external side effects.
