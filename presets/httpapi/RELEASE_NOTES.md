# L8 httpapi — D58 approved version2

## P1 preflight, contract and inventory

2026-11-17, feature/split HEAD42dccd3f8051a238a57201efbca6e727c13ab0c7, initial git status --porcelain empty. D56 is committed, no overlapping L10 changes at entry. Go1.27.4 darwin/arm64 from /Users/seven/sdk/go1.27.4; src/fmt has sources and GOTOOLCHAIN=local go test fmt passed. No toolchain download/directive/dependency update. Initial router consumer bind failed in sandbox; it is repeated with explicit loopback access, not replaced by a fake.

D58/D58.1–D58.4 approved version2: Config contains router/server and separate operations/router/server policy, RequestIDConfig(enabled/header/accept_incoming), recovery flag and nonnegative request timeout. Dependencies contain routes/middleware, operations.Health, optional metrics, ID generator and mandatory recovery callback when enabled. New returns Application with immutable handlers/server-value getters. Run/RunListeners own only HTTP listeners/serving goroutines, execute once, join failures and drain peers. No engine/lifecycle/externally-owned resource close. ServerConfig.Validate is an additive router method reusing the existing validator. WriteTimeout0 is unlimited and passes cross-budget constraints. Public errors ErrInvalidConfig/ConfigError(Field,Reason), ErrInvalidCall, ErrAlreadyStarted. Full API/defaults in README; no remaining architect question inferred from internal choices.

| origin | destination | action | proof |
|---|---|---|---|
| preset/http.Options returns WithRouter/WithHealth engine options | httpapi.New | adapt, explicit borrowed health and host/listen policy | pure construction/handlers |
| preset/full registers every declared provider/family | caller-selected dependencies | exclude equivalence | dependency guard/no SDK |
| legacy router implicit logger/compress/recover/ID/timeout | approved preset ID/recovery/timeout flags only | adapt | exact middleware order |
| engine reads Viper/CONF_DIR/SCOPE/PORT, owns signals/hooks | main flags/signal context plus Run | separate | no constructor I/O, startup/drain tests |
| legacy operations share API router/port | separate policy/handler/listener | develop | host/pipeline isolation |
| example engine.Get Redis repository/TTL | memory order store owned by example | adapt only HTTP demonstration | CRUD/errors/concurrent memory tests |
| L2 router composed repeats ID/recovery/errors/health/server setup | preset plus explicit callback/resources | reduce assembly | real composed consumer |

No production legacy/example migration, no L10/10b/10c, no workflow/Makefile/script edits. D56.5 approves v0.1.0, publishing remains L21/E7. Linux/CI not run. E1 in progress, other evidence pending.

## Current checkpoint — not complete

Handlers, Run/RunListeners, memory example, consumers and catalog registration are implemented. Router adds Validate using unchanged checks and now drains admitted connections even after Accept failure; a net.Pipe/synctest regression passed. Initial assembly race tests and lint/dependency guards passed. Full socket campaigns were NOT executed: automatic approval review failed due account usage limit, explicitly not a safety rejection. No attempt bypassed that review. Full profile, Run loopback stress, fuzz/bench, example lifecycle, router regression, root checks, parity promotion, gorelease and final hashes remain pending. No full functional acceptance claimed. Additional non-socket targeted checks are being recorded separately.

Internal ownership rules: invalid calls do not consume startup; valid failed binds do. Listener wrappers close each underlying instance once. Same pointer passed for both listeners is invalid. Factories run outside locks and all route/dependency validation precedes them; arbitrary factory effects cannot be rolled back. Atomic single-use state is the only shared lifecycle state. Borrowed noncooperative handlers remain outside any promise of forced termination.
