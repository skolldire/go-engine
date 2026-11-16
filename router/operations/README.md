# router/operations

Experimental D51/D51.2 candidate, not released. Independent of router/chi/engine; only health types and stdlib. `New(Config,Dependencies)(http.Handler,error)` returns a local handler, no listener. Dependencies.Health is the consumer-side interface Live/Ready/Check, satisfied by *health.Registry; it is required with Config.Health and typed nil pointers are rejected. Metrics is an optional borrowed http.Handler. No resource is built or closed, no logging, environment/profile lookup or polling.

Defaults: Ping=true, Health=true, Pprof=false, MaxProfileDuration=30s (>0 even when pprof disabled). Literal configs remain literal; Config.Validate and construction errors use ConfigError{Field,Reason} wrapping ErrInvalidConfig. Tags are snake_case.

GET and HEAD endpoints:

| Path | Status/body |
|---|---|
| /ping | 200 {"status":"ok","message":"pong"} |
| /live | 200 or503 {"live":bool} |
| /ready | 200 if Ready nil,503 for errors.Is(health.ErrNotReady), otherwise500 |
| /deps | 200 if Check nil, otherwise500; adds live to ready/checks |
| /metrics | injected handler only when present |
| /debug/pprof/* | native pprof only when explicitly enabled |

Ready/deps checks contain exactly name, status, reason, duration_ms (integer milliseconds), never Err, PanicError.Value or Stack. Empty checks is []. Internal500 is {"error":"internal_error"}; invalid pprof seconds400 is {"error":"invalid_seconds"}. JSON responses use application/json and Cache-Control:no-store. HEAD suppresses the body; metrics/pprof retain their native/injected success representations. Other paths404 and unsupported methods405 follow ServeMux.

Pprof uses explicit registrations on this handler. The native net/http/pprof package itself has its documented init registration on http.DefaultServeMux; this module never uses that mux to serve. Enable profiling only on an explicitly controlled internal listener. seconds must be a single finite positive value within MaxProfileDuration; non-trace durations must be integral. Missing profile seconds is capped to min(30s,floor(max)); missing trace to min(1s,max). A profile limit below1s cannot run an integer-second CPU profile and returns400. All delta-profile seconds also pass the same guard. This bounds requested durations, not overhead of native profiling or uncooperative injected handlers. No production-profile auto-disable. The optional router server's WriteTimeout must exceed MaxProfileDuration; the future preset validates that assembly relationship.

Health callbacks preserve their own timeout/cancellation contracts; the request context is passed through. A failed diagnostic check is not a /deps transport failure. Writers and Health/Metrics are borrowed; serving concurrently requires their concurrent safety. No automatic logging or retry after a partial write. Compose with health.Registry and an application-owned logger, not a driver-specific checker inside this module. See MIGRATION/RELEASE_NOTES; proposed v0.1.0 unapproved.
