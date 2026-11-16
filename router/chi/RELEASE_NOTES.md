# router/chi — D51, wave 2 L2

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

D51.1/D51.2 final policy amendment applies through router.Policy without duplicated CORS or host logic; native routes still have no host grouping or implicit middleware.

## Implementation, scope and evidence

New takes router.Config and delegates policy validation/copying to router.NewPolicy, then constructs native chi/v5 v5.5.0. Native returns *chi.Mux, Handler returns the shared policy wrapper. No host route translation, hidden middleware, Freeze or runtime registration synchronization is invented. Native registrations must finish before serving. Library and tests depend only on router, chi and stdlib; standalone graphs verify local replaces. No change to root's resolved chi version or legacy production.

Tests cover native path parameters, host policy421, invalid Config and absence of implicit middleware; FuzzHost30s completes1,186,476 executions without failure. Light profile: `(cd router/chi && GOWORK=off go test -race -count=5 ./... && GOWORK=off go vet ./...)` passes. Generic deps/consumers/quick/fuzz/bench all pass; quick conservatively includes count20 at default/1/4 and vet. No separate stress. BenchmarkHandler compares bare chi and Policy-wrapped chi with equivalent no-I/O handlers, b.Loop/allocations,500ms/count3; construction excluded, no SLA/zero-allocation claim. Diagnostic coverage100%, not proof of independence; graph/import/replaces guards establish that boundary.

Reproduce with `make check-module M=router/chi MODE=deps` and modes consumers, quick, fuzz, bench. All consumers run offline GOWORK=off/GOTOOLCHAIN=local/GOPROXY=off/GOFLAGS=-mod=readonly; minimal and composed both use the real adapter/native route registration. Catalog/workspace discovery requires no Makefile/workflow change. Root validate/lint-lists/lint/lint-arch/lint-docs/check-modules/test and health/logging/router-errors consumers pass; full command/environment/root results are also recorded in router/RELEASE_NOTES.md. Actual platform Go1.27.4 darwin/arm64 only, no Linux/CI claim. Logs evidence/profile-final.log, final-check-*.log, acceptance-consumers.log, acceptance-bench.log, graph-coverage-acceptance.log are ignored local evidence; hashes and this summary are versioned.

Gorelease was actually run outside go.mod in a temporary source-tree copy preserving local replace layout, `GOWORK=off GOPROXY=off gorelease -base=none -version=v0.1.0`. Exit1: package preparation cannot resolve github.com/skolldire/go-engine/router@v0.0.0 (module lookup disabled by GOPROXY=off). Gorelease intentionally needs resolvable published dependencies despite development replaces. This is a distribution impediment, not a passed compatibility check. No base is published, no artificial proxy/tag created and no dependency changed to hide it. See evidence/gorelease-final.log.

E1 contract/docs and inventory complete; E2 light functional/race/fuzz/bench pass; E3 native chi plus real policy integration pass; E4 offline graph/consumers pass; E5 executed but blocked by unpublished router dependency; E6 proposed independent v0.1.0 not approved; E7 real-tag consumers deferred. No publication/stability claim, commit/push/tags/index or release workflow changes.
