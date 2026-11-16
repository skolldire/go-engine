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
