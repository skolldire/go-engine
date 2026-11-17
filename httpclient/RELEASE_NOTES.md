# D53 L4 — httpclient: approved contract, decisions and evidence

Experimental candidate, not released. Proposed version v0.1.0 is **unapproved**; the catalog row stays `unreleased` / `-`.

## Initial state

Branch `feature/split`, HEAD `bde907834b48ebeab6c8481629aeb0363cdea1b0`. `git status --porcelain` was empty at the start: no earlier uncommitted change existed in `modules.tsv`, `go.work`, `CHANGELOG.md`, `MIGRATION.md` or any module, so every difference in the worktree belongs to this lot. Go go1.27.4 darwin/arm64. No `go` directive or dependency version was changed. No file with " 2" in its name was touched. No commit, push, tag or workflow dispatch was made.

## D53 as approved (architect)

Module `github.com/skolldire/go-engine/httpclient`, standard library only including tests. Catalog row: kind `tool`, track `unreleased`, version `-`, checks `production`, deps `stdlib`.

API: `TransportConfig`, `Config`, `Dependencies{RoundTripper, TLSConfig, WrapTransport}`, `Client`, `DefaultConfig`, `Config.Validate`, `New`, `Client.NewRequest`, `Client.Do`, `Client.HTTPClient`, `Client.CloseIdleConnections`, `StatusError{StatusCode, Response}`, `CheckStatus`. Errors: `ErrInvalidConfig` (`ConfigError{Field, Reason}`), `ErrInvalidCall`, `ErrNilContext`, `ErrResponseTooLarge`, `ErrOriginNotAllowed`, `ErrRedirectNotAllowed`. Defaults: `RequestTimeout` 10s, `ConnectTimeout` 5s, `ResponseHeaderTimeout` 5s, `IdleTimeout` 90s, `MaxIdleConns` 100, `MaxIdleConnsPerHost` 10, `FollowRedirects` false, `MaxRedirects` 0; `BaseURL`, `MaxResponseBytes` and `MaxConnsPerHost` required.

Approved changes against the earlier contract: transport fields grouped in `TransportConfig`; TLS files and `InsecureSkipVerify` removed from `Config` in favor of a cloned `Dependencies.TLSConfig`; `NewRequest`, `HTTPClient`, `WrapTransport`, `CheckStatus`/`StatusError`, `ErrOriginNotAllowed` and `ErrRedirectNotAllowed` added. Sections C (construction and transport), D (Do, limits and security), E (no hidden policies) and F (recommended composition) are implemented as written; the README states each rule.

No signature, exported field, default, exported error or format was added, removed or changed relative to D53.

## Inventory and migration map

| Origin (`http/pkg/rest`, `http/provider/rest`) | Destination | Action | Proof required |
|---|---|---|---|
| `rest.NewClient(ctx, cfg, logger)` returning `Service` over resty with `BaseClient` | `httpclient.New(cfg, deps)` returning `*Client` and an error | develop | construction tests, `New` without network |
| `Config.BaseURL`, concatenated with the endpoint | `Config.BaseURL` validated; relative paths joined under it | adapt | BaseURL table, join table |
| `Config.TimeOut`, zero normalized to `DefaultTimeout` (resty `SetTimeout`) | `Config.RequestTimeout`, validated, covering headers, redirects and body | adapt | header and body timeout tests, release on Close and EOF |
| resty default transport: no per-host limit, no response size limit | `TransportConfig` and required `MaxResponseBytes` | develop | transport field test, limit tests at the exact boundary |
| `RetryConfig`, `ConservativeRetryCondition`, `AllowNonIdempotentRetryCondition`, `RetryAfter`, `RetryableStatus`, `IsIdempotent` | none here; `resilience` with a caller classifier | retire from the client | composed consumer: 503 then success, POST without key not replayed |
| `breakerTransport` inside resty's retry loop, `ErrCircuitOpen` | none here; `circuitbreaker` through `resilience` | retire from the client | documented in MIGRATION; no breaker in the graph |
| `validateResponse`: non-2xx as an error with status and 200 body characters | response with nil error; `CheckStatus` and `StatusError` with the code only | adapt | non-2xx test, message without body or URL |
| `EnableLogging`, `WithLogging`, `BaseClient.Execute` | none here; `client/logging` outside the operation | retire from the client | composed consumer: one record per logical call |
| any absolute URL accepted through `R(ctx)` | single origin, `ErrOriginNotAllowed` | develop | origin table, fuzz |
| resty default redirect policy | off by default, bounded, credentials dropped across origins | develop | redirect tests |
| `http/provider/rest` (`engine.Provider`, section `rest`) | none | keep legacy unchanged | no legacy file modified |

### Starting facts verified against the code, with deviations

Confirmed: `rest.NewClient(ctx, cfg, logger.Service)` returns a `Service` over resty with `BaseClient`; zero `TimeOut` becomes `DefaultTimeout` (10s); the transport is resty's default; retry uses resty's engine with `Conservative`/`AllowNonIdempotent` conditions and `Retry-After`; the breaker is a `RoundTripper` inside the retry loop; the non-2xx error text is `HTTP <code>: <status> - <first 200 body characters>`; the only consumer is `http/provider/rest`.

Deviations found:

- On a non-2xx the verb helpers return the **response and** the error, not the error alone. Callers can already read status and body from the response; the exposure is the body preview in the error text.
- `R(ctx)` bypasses `BaseClient.Execute` and `validateResponse` altogether: through the builder, a non-2xx is already a response with a nil error, and nothing is logged.
- The legacy retryable set is 429, 500, 502, 503, 504 **and 507**, wider than the 429/502/503/504 recommended in D53.F. The 500 difference is deliberate and documented in MIGRATION.
- resty buffers the body except with `SetDoNotParseResponse` or `SetOutput`.
- The breaker hands an unhealthy status back as a response while counting it as a failure.

## Internal decisions

Each is inside the approved signatures. The ones marked **(contract precision)** define observable behavior the approved text left open, and are the ones to look at first when validating the lot.

1. **(contract precision) A zero `Transport` passes `Validate`.** D53.C requires `TransportConfig{}` with a borrowed `RoundTripper`, while every transport field is required to be positive. `Validate` cannot see `Dependencies`, so a zero `Transport` is read as "borrowed" and accepted there; `New` rejects it when no `RoundTripper` is supplied. In `New` the borrowed-mix check runs before `Validate`, so a partial `Transport` next to a borrowed transport is reported as field `Transport`, not as one missing timeout.
2. **(contract precision) `MaxRedirects` other than 0 without `FollowRedirects` is invalid.** A limit for something that is switched off is a contradiction worth reporting rather than ignoring.
3. **(contract precision) `NewRequest` rejects an empty method and dot segments.** net/http treats an empty method as GET; here the caller states it. Dot segments in a relative path, encoded or not, are `ErrInvalidCall`: they could climb out of the `BaseURL` prefix. An absolute same-origin URL given to `Do` is not path-restricted, since the caller wrote the whole URL.
4. **(contract precision) An absolute or scheme-relative path in `NewRequest` matches both `ErrInvalidCall` and `ErrOriginNotAllowed`.** The call is malformed, and code watching for attempts to leave the origin sees it too.
5. **(contract precision) Exceeding `MaxRedirects`, or redirecting to a non-HTTP scheme, is `ErrRedirectNotAllowed`.** D53 names that error only for https→http; a second redirect error was not in the contract. `Do` returns a nil response when a redirect is refused; `HTTPClient()` used directly keeps net/http's behavior of also returning the closed last response.
6. **The request timeout lives in the internal `RoundTripper`, not in `http.Client.Timeout`.** The first implementation used `Client.Timeout` and a test showed it keeps its timer and context until `Close` even after EOF, contradicting D53.D; it also starts a goroutine per request for a `RoundTripper` it does not recognize. The guard derives a context with the deadline, releases it on EOF, read error or `Close`, and passes the deadline to redirect hops through a context value read from the previous response's request, so a chain shares one budget. For that, `resp.Request` is set to the request as the guard sent it.
7. **`Close` closes the body before releasing the context**, and EOF releases only after the transport reported it, so releasing never costs a reusable connection (verified by the single-connection tests).
8. **A body read cut by the deadline always matches `context.DeadlineExceeded`**, wrapping whatever the transport reported.
9. **`TLSHandshakeTimeout` is `ConnectTimeout`.** The zero value means no handshake limit; D53 has no separate field.
10. **`BaseURL` path is a plain prefix** (no dot, empty or encoded segments, no query, fragment or userinfo), and a relative path is concatenated to it rather than resolved by RFC 3986, where `/x` would drop the prefix. This matches the legacy `BaseURL + endpoint`.
11. **Origin comparison** lowercases scheme and host, compares the port as a number with scheme defaults, and applies no IDNA or trailing-dot folding: another spelling fails closed. Userinfo is refused even on the right host.
12. **Redirect hops are recognized by `Request.Response`**, which net/http sets only on the requests it creates to follow a redirect. Code holding `HTTPClient()` could set that field by hand to skip the origin rule; that is trusted application code, not the untrusted URL data the rule defends against. Documented.
13. **`HTTPClient()` with a relative URL is `ErrInvalidCall`.** Resolving it in the `RoundTripper` would leave net/http's redirect logic working from a URL without a host.
14. **Credential stripping is sticky**: once a chain has left the first origin the three headers stay removed even if it is redirected back, because that hop was chosen by a server that never held them.
15. **`HEAD` is exempt from the `Content-Length` check**, a nil response body from a borrowed transport becomes `http.NoBody`, and a body that is also an `io.Writer` (101, CONNECT) keeps its write side.
16. **`ProxyURL`** accepts `http`, `https`, `socks5` and `socks5h` with a host and no path; userinfo is allowed, as it is how proxy credentials are given, and `ConfigError` never echoes it.
17. **A typed-nil `RoundTripper`, or a `WrapTransport` returning one, is a `ConfigError`** instead of a later nil dereference.
18. **`Do` closes the request body when it rejects a call early**, as net/http does.
19. **`NewRequest` does not copy `body`**; documented, and it avoids an allocation per request.
20. **`ConfigError.Field` holds the Go field name** (`Transport.MaxConnsPerHost`), as in the sibling modules.
21. **Test placement.** D53 lists "retry with resilience resending the whole body" under httpclient, whose tests must be stdlib-only. The module test replays a body over two manual attempts (new request per attempt, and one request rebuilt with `GetBody`); the real `resilience` retry is in `examples/standalone/httpclient/composed`.
22. **Fuzz targets drive the guard directly and cap inputs at 512 bytes.** Through `http.Client`, Go's fuzzing engine spent most of its time minimizing inputs over net/http's instrumented coverage and executed almost nothing. This was first suspected to be a hanging input and ruled out: with `-fuzzminimizetime=1s` throughput returned at once.
23. **Stress** is `TestStressHostileUpstream`, enabled by `HTTPCLIENT_STRESS=1`, the variable the generic checker sets.

## Consumers and independent graph

Each consumer has its own `go.mod`, replaces only the modules it uses, and is verified offline by the generic checker (`GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly`). No external module is needed, so `consumer-deps.tsv` is unchanged.

- `examples/standalone/httpclient/minimal`: `httpclient` alone against an `httptest` server on loopback: `NewRequest`, `Do`, `CheckStatus`, the size limit and the origin rule.
- `examples/standalone/httpclient/composed`: `httpclient`, `client`, `resilience`, `retry`, `client/logging` and `logging`. Per-attempt operation, classifier by method and status, a 503 followed by success, a `PUT` and a keyed `POST` whose second attempt carries the whole body, a `POST` without an idempotency key that is not replayed, a 500 on an unkeyed `POST` returned for reconciliation, and one log record per logical call. `MaxConnsPerHost` is 1 and the server counts one connection for all eight attempts, which proves that every attempt closed its body.

## Executed evidence

Local platform only: go1.27.4 darwin/arm64, Apple M1 Max. Linux was not run and no CI was dispatched. `make lint` used golangci-lint 2.14.0 from the session helper `/private/tmp/logging-env.sh` (PATH and caches only; not a repository dependency). Only what follows was executed.

```sh
(cd httpclient && GOWORK=off go test -race -count=10 ./...)
(cd httpclient && GOWORK=off GOMAXPROCS=1 go test -race -count=10 ./...)
(cd httpclient && GOWORK=off go vet ./...)
(cd httpclient && GOWORK=off go test -count=1 -cover ./...)
make check-module M=httpclient MODE=deps
make check-module M=httpclient MODE=consumers
make check-module M=httpclient MODE=quick
make check-module M=httpclient MODE=stress
make check-module M=httpclient MODE=fuzz
make check-module M=httpclient MODE=bench
./scripts/modules.sh validate
./scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
make check-module M=client MODE=consumers
make check-module M=resilience MODE=consumers
make check-module M=client/logging MODE=consumers
gorelease -base=none -version=v0.1.0   # in a copy outside the repository
```

Every command exited 0.

- **Race**: `-race -count=10` passes with the default `GOMAXPROCS` (9.7s) and with `GOMAXPROCS=1` (9.9s). The generic quick mode adds `-race -count=20` at default, 1 and 4 (22.5s, 18.2s, 17.8s) and `go vet`.
- **Stress**: `TestStressHostileUpstream`, `-race -count=3`. 64 workers × 40 calls against a loopback server with slow headers, stalled bodies, cut responses, oversized fixed, streamed and gzip bodies, and callers cancelling mid-flight. One run: 634 complete, 396 too large, 1331 timed out or cancelled, 199 cut; the other two are within a few percent. After each run every server connection was closed, the goroutine count returned to its baseline and the standard `goroutineleak` profile reported 0.
- **Fuzz**: `FuzzOrigin` 30s, 717,586 executions; `FuzzRedirect` 30s, 703,925 executions; both pass with no finding.
- **Benchmarks** (500ms, count 3, one loopback GET with an 11-byte body read to EOF and closed): `Do` 44.7–47.4 µs/op, 7.2 KB/op, 83 allocs/op; a bare `http.Client` on the same transport 42.2–42.8 µs/op, 5.4 KB/op, 68 allocs/op. The guard costs roughly 2–5 µs, 1.8 KB and 15 allocations per request on this machine. This is a comparison, not a throughput or latency claim.
- **Coverage**: 97.3% of statements, as a diagnostic.
- **Root**: `modules.sh validate` and `lint-lists` pass; `make lint` reports 0 issues in every catalog module; `lint-arch`, `lint-docs`, `check-modules` and `make test` pass; the consumers of `client`, `resilience` and `client/logging` still pass.
- **gorelease**, run outside the repository on a copy of the module with `-base=none -version=v0.1.0`: "v0.1.0 is a valid semantic version for this release". First version, no base to compare against.

Tests cover every item D53 lists for this module. Timeouts that do not depend on the real network run under `testing/synctest` with exact virtual durations; the rest use `httptest` servers on 127.0.0.1, including a TLS server negotiating HTTP/2. No external service is contacted.

Raw logs are under `httpclient/evidence/`, which is git-ignored. `SOURCES.sha256` and `EVIDENCE.sha256` list the SHA-256 of every source of the candidate and of every log; this file is not listed in them so that it can name them.

## Status

E1 contract and documentation: complete. E2 functional quality, security and resources: complete locally (race, stress, fuzz, benchmarks, leak checks). E3 HTTP integration on loopback: complete. E4 independence and real composition: complete. E5 gorelease without a base: executed, passes. E6 version approval: **pending**; v0.1.0 is proposed, not approved. E7 consumers against a real tag: deferred until a tag exists.

`make all` was not run. The worktree shows some of these files as staged; no `git add`, commit, push or tag was executed by this work, and the staging was left as found.

Manifest hashes at the time of writing:

```
b9f6e39a21634109c3b715bd2d560be00d8549ea3db3363544f5f9fd88d98155  httpclient/SOURCES.sha256
6cff38391332d5877ae43e765b9ded57e39cac83cb4c251ee8fc87c97bed0629  httpclient/EVIDENCE.sha256
```
