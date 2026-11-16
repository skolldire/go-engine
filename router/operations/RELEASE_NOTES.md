# router/operations — D51, wave 2 L2

## Initial state and decisions

Branch feature/split, HEAD dfc3908098462049c3aa8e51b6e0925760c4bd7d. Initial git status --porcelain (before L2 files):

```text
 M CHANGELOG.md
 M MIGRATION.md
 M go.work
 M modules.tsv
?? examples/standalone/router/
?? router/
```

These existing changes belong to L1 router/errors, not L2. Shared catalog/workspace/changelog/migration edits will be additive and separately described. Legacy production, existing dependency versions/directives, index and files containing " 2" remain untouched. No commit, push, tags or release workflow execution. Toolchain Go1.27.4 darwin/arm64; resolved chi v5.5.0. D51 is architect-approved; public ambiguities are pending user clarification before implementation. Internal choices will be recorded here.

health and router/errors independent consumers pass offline at L2 entry. Their implementations are not substituted. L1 remains a separate candidate.

## Inventory and migration

| origin | destination | action | required proof |
|---|---|---|---|
| pkg/router.NewService zero normalization | explicit Config/ServerConfig | adapt | literal/default validation |
| chi Mux public routing | neutral ServeMux / optional chi adapter | develop | pattern matching, policy reuse |
| implicit RequestID/Logger/Recoverer/Timeout/Compress | explicit middleware, caller observability/compression | adapt/exclude | observable order and no implicit behavior |
| trustedRealIP rewrites RemoteAddr | ClientIP context value | adapt | forged/untrusted chains, original RemoteAddr |
| go-chi/cors | stdlib explicit CORS | develop | preflight, Vary, credentials |
| any Host accepted | required AllowedHosts | develop | exact/wildcard/IP/421 |
| fixed ping and profile-based pprof | separate operations handler | adapt | fixed public DTOs, explicit opt-in |
| Run signal.Notify, PORT, shutdown hooks | Serve with context and explicit listen | adapt | listener errors and bounded shutdown |
| legacy health HTTP error strings | private operations DTO | adapt | Err/Value/Stack absent |

Confirmed by reading pkg/router/entity.go/service.go/realip.go and pkg/health/handler.go: timeouts normalize as described, HandlerTimeout tracks WriteTimeout; RequestID/chi Logger/Recoverer/Timeout/Compress are implicit; CORS conditional; pprof checks app_profile and PORT is read by setPort. Legacy Run owns signals and hooks, derives shutdown from Background. No host policy. Real IP trusts configured peers then rewrites RemoteAddr, skipping malformed XFF entries. Consumers include pkg/engine, preset/full, example and pkg/testutil. Legacy /deps can return503 and publish dependency Error text; the new diagnostic contract deliberately differs. No legacy migration in this lot.

E1–E5 pending implementation/validation. Proposed independent v0.1.0 is not approved; track remains unreleased. E6 version approval pending; E7 authorized real-tag consumers deferred. Linux CI is not locally executed or dispatched.

## D51 clarifications approved by user, resumed at HEAD 2c0f1bf

The worktree is now clean: the user committed the prior increment/L1 externally; this task did not commit. Accept only *.example.com as a single-label host wildcard, not .example.com. CORS uses * for any origin, rejects empty origins and * with credentials. operations exports ErrInvalidConfig and ConfigError{Field,Reason}; ready/deps use {ready:bool,checks:[{name,status,reason,duration_ms:int64}]} with [] when empty; foreign health errors produce500 without causes. RequestID empty Header defaults to X-Request-ID, incoming IDs require visible ASCII1..128, default generator crypto/rand16 bytes hex; invalid generated IDs panic without their content. Timeout(d) returns Middleware, d<=0 passes through. Recover(nil) returns empty500; ErrAbortHandler is absorbed without callback/write. These are approved public precisions, not inferred behavior.

## D51.1/D51.2 final amendment

D51.2 supersedes earlier wire proposal: /deps includes live in addition to ready/checks; internal500 is exactly {"error":"internal_error"}; pprof400 is {"error":"invalid_seconds"}. All JSON responses carry application/json and Cache-Control no-store, support HEAD and exclude Err/Value/Stack. /ready retains ready/checks, /live live, /ping status/message. The router middleware and CORS amendments are recorded in router/RELEASE_NOTES.md. No remaining public consultation is uncovered; these are the final user-approved definitions.

## Implementation, scope and evidence

Only stdlib and health types are imported. Health is a local consumer-side Live/Ready/Check interface, not a concrete-registry dependency. New validates MaxProfileDuration, then required Health (typed nil pointer included); no lifecycle/resources/observation added. A private fixed DTO copies name/status/reason/duration_ms and excludes diagnostic causes/PanicError internals. Empty checks serializes []; /deps includes Live(), /ready and /deps internal500 are fixed JSON, never foreign error text. HEAD wrapper suppresses bytes even with a ResponseRecorder; underlying metrics/native pprof own their success content types. JSON paths and pprof400 carry application/json and no-store. Writes are not retried after failure.

Explicit pprof registrations use native handlers. Standard net/http/pprof itself registers on DefaultServeMux in its package init; this module does not serve that global mux or call it. The local handler exposes pprof only with Pprof=true. seconds is single, finite, positive, within configured limit; non-trace profiling uses integral seconds, trace permits fractions. Missing defaults are bounded to the configured cap rather than letting native30s/1s bypass it; CPU cap below1s returns400. Delta profiles pass the same check. No live CPU profile or trace collection is necessary for these validation tests; Index and invalid-duration routes are exercised, native accepted duration handlers remain stdlib behavior. Native profiling overhead is not a hard wall-clock cancellation guarantee.

Unit tests cover validation, nil/typed nil, all route status paths200/503/500/404/400/405, exact ping, HEAD suppression, empty checks, private real health failed/panic checks, and fixed error JSON. FuzzSeconds30s completes929,482 executions without failure. Light `(cd router/operations && GOWORK=off go test -race -count=5 ./... && GOWORK=off go vet ./...)` passes. Generic deps/consumers/quick/fuzz/bench pass; quick adds its conservative default count20/1/4 matrix and vet, no separate stress. BenchmarkReady uses10 real fast health checks, b.Loop/benchmem,500ms/count3, no artificial I/O delay. It includes health evaluation/DTO cost, not pure routing overhead. Diagnostic coverage89.9%, not correctness proof.

Reproduce with `make check-module M=router/operations MODE=deps` plus consumers, quick, fuzz, bench. Offline consumer flags GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/GOFLAGS=-mod=readonly. Both consumers select the real health registry, verify failed readiness503 and diagnostic200 without private error text, no socket/router/engine imitation. The router composed consumer additionally tests application assembly with logging/errors and separate operations. Registry slot/timeouts remain health responsibilities, not duplicated workers here.

Root validate/lint-lists/lint/lint-arch/lint-docs/check-modules/test and related health/logging/router-errors consumers pass; common commands/environment in router/RELEASE_NOTES.md. Go1.27.4 darwin/arm64 only, Linux CI not executed/claimed. Raw evidence/profile-final.log, final-check-*.log, acceptance-consumers.log, acceptance-bench.log and graph-coverage-acceptance.log are ignored; summaries/hashes are versioned. WriteTimeout>MaxProfileDuration is deliberately an assembly check for presets/httpapi; README recommends a separate internal listener. No profile/environment inference.

Gorelease actually executed outside module go.mod in a temporary source tree preserving relative replaces, `GOWORK=off GOPROXY=off gorelease -base=none -version=v0.1.0`: exit1, cannot resolve github.com/skolldire/go-engine/health@v0.0.0 with module lookup disabled. First version/no published base; distribution is pending resolvable approved tags. No artificial tag/proxy or dependency update. See evidence/gorelease-final.log.

E1 docs/contracts/inventory complete; E2 light functional/race/fuzz/bench pass; E3 real health/private DTO integration pass; E4 offline independence/composition pass; E5 executed but blocked by unpublished health; E6 proposed independent v0.1.0 unapproved, catalog unreleased/-; E7 real-tag consumers deferred. No production legacy changes, commit, push, tags or release workflow changes/dispatch.
