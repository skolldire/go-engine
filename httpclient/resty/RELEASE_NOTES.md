# D53 L4 — httpclient/resty: approved contract, decisions and evidence

Experimental candidate, not released. Proposed version v0.1.0 is **unapproved**; the catalog row stays `unreleased` / `-`.

## Initial state

Branch `feature/split`, HEAD `bde907834b48ebeab6c8481629aeb0363cdea1b0`. `git status --porcelain` was empty at the start of the lot; the `httpclient` module this adapter depends on is new work of the same lot (see [../RELEASE_NOTES.md](../RELEASE_NOTES.md)), not an earlier change. Go go1.27.4 darwin/arm64. `go list -m github.com/go-resty/resty/v2` resolves v2.17.2 in the repository, and that exact version is required here; no dependency or `go` directive was updated. No file with " 2" in its name was touched. No commit, push, tag or workflow dispatch was made.

## D53 as approved (architect)

Module `github.com/skolldire/go-engine/httpclient/resty`, depending on `httpclient` and `github.com/go-resty/resty/v2`. Catalog row: kind `composition`, track `unreleased`, version `-`, checks `production`, deps `httpclient,github.com/go-resty/resty/v2`.

API: `Dependencies{HTTPClient *httpclient.Client}` (required, borrowed), `Client`, `New(deps)`, `Client.R(ctx)`, `Client.Get/Post/Put/Patch/Delete`, `CheckStatus(*resty.Response)` returning an equivalent `*httpclient.StatusError`. Built with `resty.NewWithClient` over `HTTPClient.HTTPClient()`, so transport, origin, limits and redirects are inherited; `BaseURL` comes from the httpclient; no `Config`. resty retry at 0 with no conditions and the native client not exposed. A non-2xx returns the response and a nil error. Nil dependencies are a `ConfigError`.

No signature, exported field, default, exported error or format was added, removed or changed relative to D53.

## Inventory and migration map

| Origin (`http/pkg/rest`) | Destination | Action | Proof required |
|---|---|---|---|
| `NewClient(ctx, cfg, logger)` with its own `Config` | `New(Dependencies{HTTPClient})`, no `Config` | adapt | invalid dependencies test |
| `Service.R(ctx)` | `Client.R(ctx)` | keep | JSON round trip |
| `Service.Get/Post/Put/Patch/Delete` | same shapes on `*Client` | keep | verb table |
| verb helpers: non-2xx as response plus error with body preview | response and nil error; `CheckStatus` | adapt (breaking) | non-2xx test, message without body |
| `Service.WithLogging` | none | retire | documented |
| resty retry configured from `RetryConfig` | fixed at 0, native client not exposed | retire | retry-off test, including a request-level condition |
| resty default transport and redirect policy | inherited from `httpclient` | adapt | inherited origin, limit, redirect and timeout tests |

The starting facts and their deviations are recorded once, in [../RELEASE_NOTES.md](../RELEASE_NOTES.md).

Facts checked in resty v2.17.2's source before relying on them: `NewWithClient` keeps the given `*http.Client` and creates a transport only when it has none; the default retry count is 0 and `Request.Execute` takes the single-attempt path when the client count is 0, ignoring request-level conditions; resty does not set `Accept-Encoding` itself and inflates a gzip body only when the response still carries `Content-Encoding: gzip`; a body read error is returned as the call's error; `SetDigestAuth`, `SetTimeout`, `SetTransport`, `SetRedirectPolicy` and `SetCookieJar` assign fields on the `*http.Client` resty holds.

## Internal decisions

The ones marked **(contract precision)** define observable behavior the approved text left open.

1. **resty receives a shallow copy of the shared `*http.Client`.** D53 says the adapter is built over `HTTPClient.HTTPClient()` and that this client is shared and must not be modified. resty's `SetDigestAuth` on a request swaps the transport of the client it holds for the duration of the call. The copy has the same guarded transport and redirect policy, so everything is inherited, and the shared client stays untouched (tested).
2. **The base URL is obtained with `NewRequest(ctx, GET, "", nil)`.** `httpclient` has no accessor for it and adding one would change the approved API; an empty path yields the normalized base URL without any network activity.
3. **(contract precision) A request that sets `Accept-Encoding` is refused with `httpclient.ErrInvalidCall`.** With the header set by hand the transport stops decoding gzip, the size limit counts compressed bytes, and resty then inflates the body without a bound: a few kilobytes on the wire could become gigabytes in memory, defeating `MaxResponseBytes`. Refusing is explicit; silently deleting the header would be a hidden policy. Implemented with resty's pre-request hook, which the adapter can own because the native client is not exposed.
4. **(contract precision) The verb helpers return `httpclient.ErrNilContext` for a nil context.** `R(nil)` cannot return an error; resty fails that request when it is executed.
5. **A `*httpclient.Client` not built by `httpclient.New`** (zero value) is a `ConfigError`, detected by its nil `HTTPClient()`, instead of a panic.
6. **`ConfigError` is `httpclient.ConfigError`**, as the adapter has no configuration type of its own; this follows `auth/jwt/http`.
7. **`CheckStatus` delegates to `httpclient.CheckStatus` on `RawResponse`.** `StatusError.Response` is that raw response, whose body resty has already read and closed; the buffered body stays on the resty response.
8. **A composed consumer was added** next to the `minimal` one D53 lists. The generic checker runs `minimal` and `composed` for every production module and editing it is out of scope. It shows a resty call retried by `resilience`.
9. **One fuzz target, `FuzzPath`.** D53 asks for no fuzzing of its own in the light profile, but the catalog gate requires at least one `Fuzz*` in a production module. The target is small and checks the property that matters: no path given to the adapter sends a request to another origin.
10. **No stress test**, per the light profile.
11. **Package name `resty`**, imported by consumers under an alias (`httpresty`) next to the upstream package.

## Consumers and independent graph

Each consumer has its own `go.mod`, replaces only the in-repository modules it uses, and is verified offline by the generic checker. resty v2.17.2 arrives through the adapter, so `consumer-deps.tsv` is unchanged.

- `examples/standalone/httpclient/resty/minimal`: a JSON request and typed result, `CheckStatus` on a non-2xx and the inherited origin rule, against an `httptest` server on loopback.
- `examples/standalone/httpclient/resty/composed`: `httpclient`, `httpclient/resty`, `client`, `resilience` and `retry`. A `GET` answered 503 then 200 succeeds after two requests; a 410 is returned after one.

`go.mod` requires resty v2.17.2 and, as an indirect requirement, `golang.org/x/net` v0.62.0. resty itself asks for v0.43.0; v0.62.0 is the version the repository already selects (`http/go.mod`), so the adapter resolves the same one and builds offline from the existing module cache. Checksums in `go.sum` were copied from `http/go.sum`; `go mod tidy -diff` reports no difference.

## Executed evidence

Local platform only: go1.27.4 darwin/arm64, Apple M1 Max. Linux was not run and no CI was dispatched. Only what follows was executed.

```sh
(cd httpclient/resty && GOWORK=off go test -race -count=5 ./...)
(cd httpclient/resty && GOWORK=off go vet ./...)
(cd httpclient/resty && GOWORK=off go test -count=1 -cover ./...)
make check-module M=httpclient/resty MODE=deps
make check-module M=httpclient/resty MODE=consumers
make check-module M=httpclient/resty MODE=quick
make check-module M=httpclient/resty MODE=fuzz
make check-module M=httpclient/resty MODE=bench
gorelease -base=none -version=v0.1.0   # in a copy outside the repository
```

The root commands (`modules.sh validate`, `lint-lists`, `make lint`, `lint-arch`, `lint-docs`, `check-modules`, `make test`) were run once for the lot with both modules registered and are recorded in [../RELEASE_NOTES.md](../RELEASE_NOTES.md); all pass, with 0 lint issues in this module.

- **Race**: `-race -count=5` passes (3.0s). The generic quick mode adds `-race -count=20` at `GOMAXPROCS` default, 1 and 4 (7.9s, 8.1s, 9.1s) and `go vet`.
- **Fuzz**: `FuzzPath` 30s, 745,014 executions, passes.
- **Benchmark** (500ms, count 3, one loopback GET): 48.2–51.5 µs/op, 10.7 KB/op, 105 allocs/op. A minimal reference, not a performance claim.
- **Coverage**: 96.2% of statements, as a diagnostic.
- **No stress campaign**, per the light profile; a normal race test runs 16 goroutines × 20 JSON round trips.
- **gorelease: exit 1, not a pass.** `go: github.com/skolldire/go-engine/httpclient@v0.0.0: module lookup disabled by GOPROXY=off`. The adapter requires an unpublished sibling at the development placeholder, which gorelease cannot resolve without the local replace. This is a distribution impediment that lasts until `httpclient` has a published version; no tag, proxy or dependency change was made to get around it. `httpclient` itself passes gorelease.

Tests cover every item D53 lists for this module: inherited origin rule, response limit (fixed, streamed, gzip and streaming with `SetDoNotParseResponse`), redirects and timeout; resty retry off, including a request-level retry condition and a transport failure; non-2xx without an error and `CheckStatus`; invalid dependencies. Servers are `httptest` on 127.0.0.1.

Raw logs are under `httpclient/resty/evidence/`, which is git-ignored. `SOURCES.sha256` and `EVIDENCE.sha256` list the SHA-256 of every source of the candidate and of every log; this file is not listed in them so that it can name them.

## Status

E1 contract and documentation: complete. E2 functional quality and security, light profile: complete locally. E3 HTTP integration on loopback: complete. E4 independence and real composition: complete. E5 gorelease without a base: executed, **blocked** by the unpublished `httpclient` dependency. E6 version approval: **pending**; v0.1.0 is proposed, not approved. E7 consumers against a real tag: deferred until tags exist.

No `git add`, commit, push or tag was executed by this work.

Manifest hashes at the time of writing:

```
41c9dde3e4d4edb744777b68e134b25595d5453ffabb1574a05a098164b16320  httpclient/resty/SOURCES.sha256
5fb458c816eb43175bc2ae0c72b6d316e20ce2e44826208608281c7bc47bf92a  httpclient/resty/EVIDENCE.sha256
```
