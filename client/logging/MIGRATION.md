# Migration to client/logging (unreleased)

The legacy pkg/core/client.WithLogging and consumers remain unchanged. This is a separate generic module with deliberate behavior differences, not an alias or compatibility bridge. No legacy API is deprecated or removed.

| Legacy | New contract | Difference |
|---|---|---|
| logger.Service | consumer-side Logger with Enabled/LogAttrs | *slog.Logger or an application logger, no backend dependency |
| Invocation.Client | client.Invocation.Component | typed replacement, no alias |
| Debug start and final event | one enabled normal-return completion | no start record; panic/Goexit has no completion |
| error text at error level | success flag at fixed debug/info | no error formatting or automatic severity promotion |
| mutable enabled callback/BaseClient.SetLogging | Logger.Enabled | no flag setter; backend owns filtering |
| absent duration | IncludeDuration default true | false omits measurement and attr |
| any result | generic T | original result/partial result and error preserved |
| implicit position in BaseClient | explicit middleware order | outer observes logical call, inner observes attempts |

Before:

```go
mw := legacy.WithLogging(legacyLogger, enabled)
```

After (a caller-built native logger or direct implementation of Logger):

```go
mw, err := clientlogging.New[Record](clientlogging.DefaultConfig(),
    clientlogging.Dependencies{Logger: logger})
if err != nil { return err }
result, err := client.Do(ctx, client.Invocation{
    Component: "repository", Operation: "fetch",
}, operation, mw)
```

*slog.Logger satisfies Logger without an adapter, but New rejects its typed nil. Other foreign typed nil instances remain caller responsibility. No type assertions/backend methods are used during execution.

A prebuilt chain requires a caller-provided terminal and valid arguments. Applying the middleware manually to a nil terminal yields nil. The middleware preserves ctx, values, deadlines, cancellation causes and errors; it does not block a pre-canceled call or replace a confirmed success.

Keep the observer outside resilience.Wrap for one logical event and a duration including waits/retries. Installing it inside attempts intentionally produces per-attempt records. Neither placement changes resilience's policy or implements resilience here.

No delivery guarantee is added. A logger can filter again, block or panic. Emission panics can interrupt return of a completed I/O result; the module does not recover or isolate them in background work. Errors ignored by *slog.Logger remain ignored and do not replace the operation error. Never put secrets in Invocation labels or caller-owned backend attributes.

Proposed version client/logging/v0.1.0 is not approved. Local v0.0.0 requirements/replaces are development-only. Published validation requires approved, resolvable dependency tags.
