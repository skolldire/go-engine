# Contratos de persistencia y operación

Estado: fundamentos, no implementación. Los contratos exactos de v1 están en
[public-contracts](public-contracts.md); esos prevalecen sobre capacidades futuras
descritas aquí. Complementa la [estrategia](../modularization.md).

### Transaction boundary and database cancellation

`database/atomic` must accept a caller-owned database and expose a transaction
callback with a minimal query/exec surface. Use `BeginTx(ctx, options)`,
`ExecContext` and `QueryContext`; commit only after callback success and roll
back on error and panic. Code inside the callback must use the supplied
transaction, including business updates, idempotency completion, outbox inserts
and any audit record that must be atomic with the business write. It must never
open a second transaction behind the caller's back.

Nested units of work need an explicit contract: either reuse the current
transaction or reject nesting; savepoints must be an explicit dialect feature.
Do not make arbitrary SQL transaction objects interchangeable with Redis,
MongoDB or distributed transactions. Atomicity here means one SQL database and
one transaction. A saga handles separate resource boundaries.

Context cancellation requests interruption through the driver; it is not proof
that a server-side write never committed. Connection loss at commit can leave
an ambiguous outcome. Report that uncertainty and resolve it using durable
operation keys; do not blindly retry the entire callback. Retry only recognized
transient conflicts and only when the operation contract permits replay.
Isolation level, lock behavior and retryable error codes differ between
PostgreSQL and MySQL and must be tested separately.

### Durable idempotency

V1 idempotency is a short SQL transactional store: unique scope/key,
request fingerprint, completion and response retention in the same transaction
as business writes. It does not own leases or run arbitrary external work.
Claims disappear on rollback; a different fingerprint is a conflict. A committed
result is replayed; an uncertain outcome requires reconciliation.

Long-running external operations belong to saga/outbox plus downstream
idempotency. Their own leases require ownership fencing. Do not conflate these
contracts or claim global exactly-once. The normative store signatures, limits,
cleanup and SQL collation rules are in [public-contracts](public-contracts.md).

### Transactional outbox and inbox

Persist the event alongside the business update in the caller's transaction.
Dispatch after commit with bounded batches. Initial outbox v1 dispatcher is
sequential; parallel workers and aggregate ordering are separate future contracts. Multiple dispatchers
claim eligible events atomically, using a dialect-appropriate locking/lease
strategy; acknowledge only with the current ownership token. Persist attempts,
next availability, lease expiry, last error and terminal/dead-letter state.
Publish outside the claim transaction so broker latency does not hold SQL locks.

A crash after publish and before acknowledgment causes redelivery: delivery is
at least once. Consumers need an inbox/deduplication key persisted with their
own business update. Ordering must be explicitly scoped, e.g. per aggregate;
parallel dispatch does not guarantee global ordering. Backoff must survive
process restart through stored next-attempt timestamps, not only timers in RAM.

### Sagas

Give each saga and step stable IDs. Persist state and checkpoint transitions
using optimistic versions or row ownership, so two orchestrators cannot advance
the same instance independently. Steps and compensations need idempotency keys.
Compensate successful steps in reverse order; persist compensation progress,
retry failures and support resume after a crash. Compensation is a business
operation that can itself fail, not a database rollback. Retain an inspectable
manual-intervention state when automatic recovery is exhausted.

Do not hold a SQL transaction while calling remote services. The gap between a
remote success and a local checkpoint requires downstream idempotency or
reconciliation; a local state table alone cannot remove that failure window.
Start with orchestration; broker choreography is a separate integration rather
than a hidden dependency of the saga module.

### Audit trails

Use append-oriented records with event ID, actor, action, resource, occurrence
and recording times, outcome, tenant and correlation identifiers. Put a required
business audit record in the same transaction as the business change; exporting
it asynchronously can use outbox. Define whether a failed audit write aborts
the business transaction. Diagnostic logs and spans are not the system of record.

Redact sensitive values and bound metadata size. Retention, access controls and
immutability need database/deployment enforcement; an append-only Go interface
alone does not make a table tamper-proof. Hash chaining or external immutable
storage is a separate option with an explicit threat model.

### Retry, jitter and breaker composition

Use a capped exponential ceiling with full jitter in `[0, ceiling]`, safe
arithmetic at high attempt counts and interruptible waits. Validate initial
backoff, multiplier, cap, attempts and total deadline. Pass context to every
attempt. Keep retry classification explicit: cancellation, invalid input and
business conflicts are not blanket retry candidates. A concurrency limiter is
not a requests-per-second rate limiter.

Choose whether the breaker counts each attempt or the whole logical operation;
make the ordering visible when composing retry and breaker. In half-open state,
limit probes and avoid letting retry multiply those probes. Connection pools,
retries and outbox dispatchers must not each independently multiply concurrency.
Logs/metrics use callbacks or optional middleware, never mandatory SDK imports.

### Logs, traces, metrics and profiling

Use `log/slog` as the default logging boundary; enrich records with trace/span
IDs through an optional handler. OTel initialization is explicit and returns a
shutdown function. Avoid mutating global providers behind a constructor.
Instrument adapters through optional wrappers or hooks, and propagate the
incoming context through workers, SQL, HTTP and messaging.

Prometheus is an optional metrics exposition adapter, not a requirement for
logs or traces. The separate `telemetry/otlphttp` and `telemetry/otlpgrpc` modules should not force both transports
onto consumers. Bound metric cardinality; request IDs, idempotency keys and
saga IDs belong in logs/traces, not metric labels. Specify trace sampling,
export queue capacity, resource identity and shutdown deadline.

pprof handlers belong to the operational HTTP server. CPU/memory/goroutine
profiling is distinct from distributed telemetry. Benchmarks live in
`*_test.go`, report allocations and run with `go test -bench . -benchmem`; they
must not introduce runtime packages into a consumer's dependency graph.

### Two routers, two lifecycles

Return an application mux that the developer extends with domain endpoints,
and a separate operations handler containing ping and health. Add pprof only
when explicitly enabled, registering on that mux rather than the global default
mux. Expose Prometheus through an injected optional metrics handler.

Bind the operations server separately so deployment controls can keep it
internal. Keep profiling operational in production when deliberately enabled;
do not rely on a profile name as the sole access boundary. Readiness and liveness
must have different failure semantics. Health dependencies use bounded contexts.

Constructing routers must not start listeners. Optional server helpers manage
read/header/idle timeouts and context-triggered graceful shutdown. Profiling
endpoints need timeout treatment appropriate to requested profile durations,
independent of short application-handler budgets.



## Relación con la estrategia completa

Los nombres finales, configuración, ownership y ruta de migración se rigen por
la [estrategia](../modularization.md), [configuración](configuration.md) y
[composición](composition-and-migration.md). Este anexo conserva los contratos
de consistencia y operación de la propuesta anterior; ninguno implica una
implementación durable existente. Los stores se separan por capacidad y dialecto
para evitar que el adaptador de conexión incorpore todos los workflows.
