# Migration from pkg/health

Legacy pkg/health, engine/family/preset consumers remain unchanged. No alias, compatibility bridge or deprecation is introduced.

Before: NewService(cfg, logger.Service).Register(...) appends without validation; GetStatus spawns fresh goroutines on every poll and copies errors into DependencyStatus.Error; NewHTTPHandler mounts endpoints and concrete Redis/SQL/HTTP checkers live in the same core.

After:

```go
cfg := health.DefaultConfig()
registry, err := health.New(cfg)
// Handle err.
err = registry.Register("database", health.CheckerFunc(db.PingContext))
// Handle err. Register everything before evaluation.
report, err := registry.Ready(ctx) // ErrNotReady is the probe failure.
diagnostic, err := registry.Check(ctx) // inspect diagnostic.Checks[i].Err privately.
```

| Legacy behavior | New contract |
|---|---|
| Timeout0 normalized to5s | seed DefaultConfig; literal zero invalid |
| logger dependency/automatic logging | no logger; caller logs private Err explicitly |
| unlimited/duplicate names | unique ASCII names1..128 bytes, MaxChecks |
| registration during evaluation | first Ready/Check freezes, including nil invocation |
| one goroutine/check/query | live slots survive timeouts; busy entry gets no replacement |
| IsLive/IsReady/GetStatus | Live, Ready probe with ErrNotReady, Check diagnostic |
| failed dependency becomes public error text | fixed Reason + original Err with json:"-" |
| adapter/checker construction in core | caller CheckerFunc or later adapter module |
| HTTP handlers in core | future router/operations owns endpoints/response policy |
| checker panic crashes process | local PanicError with Value/Stack/Goexit, fixed Error/Is(ErrPanic) |

Do not return or serialize Err, Value or Stack from public endpoints. Standard json.Marshal(Report) omits them; custom response mapping must preserve that privacy boundary. Err retains original identity for trusted logs; reports are independent outer snapshots, not deep clones of errors. A checker ignoring context cannot be killed: it keeps its entry/slot until it terminates. The new registry bounds replacements rather than promising forceful cancellation.

Report fields/status/error semantics and duration units differ from the old HealthResponse/HealthStatus types; there is no equivalence promise. Frozen sets, shared evaluation admission budget and explicit limits should be reviewed when moving an application later.
