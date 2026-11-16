# router — D51, wave 2 L2

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

## Independent server increment (before policy clarifications)

ServerConfig/DefaultServerConfig and Serve/ServeListener implemented independently of pending host/CORS/middleware wire decisions. Literal validation follows declaration order; no implicit Listen or duration defaults. ServerConfig tags are snake_case. Nil ctx/handler/listener return existing ErrInvalidCall. An already-canceled ctx opens no listener in Serve; ServeListener closes its supplied listener. Validated calls transfer listener lifetime to http.Server. TLS uses ServeTLS only when both certificate paths are configured; a preconfigured TLS listener with no paths uses Serve unchanged.

Internal implementation decisions: use one bounded-lifetime serving goroutine and a buffered one-result channel; no signal watcher, environment reads or hooks. Shutdown derives from context.WithoutCancel(ctx) plus configured timeout. After a failed graceful shutdown, Close releases remaining server connections; handlers that ignore cancellation are still caller code and are not forcibly terminated. Await the serving goroutine before return. Startup/serve/shutdown errors preserve causes with errors.Join. Constructors do not run a server.

Go1.27.4 darwin/arm64: `(cd router && GOWORK=off go test -race -count=5 ./...)` passed for the server increment, including actual 127.0.0.1:0 listeners with sandbox listener access enabled. Existing tests cover config literals/order, validation, occupied port, canceled context, graceful request/drain and deterministic accept-loop cancellation via synctest. This is not the full L2 acceptance suite: policy, mux, middleware, adapters, consumers, qualification and root regression remain pending. Local log router/evidence/server-increment.log is ignored and not a versioned deliverable.

## D51 clarifications approved by user, resumed at HEAD 2c0f1bf

The worktree is now clean: the user committed the prior increment/L1 externally; this task did not commit. Accept only *.example.com as a single-label host wildcard, not .example.com. CORS uses * for any origin, rejects empty origins and * with credentials. operations exports ErrInvalidConfig and ConfigError{Field,Reason}; ready/deps use {ready:bool,checks:[{name,status,reason,duration_ms:int64}]} with [] when empty; foreign health errors produce500 without causes. RequestID empty Header defaults to X-Request-ID, incoming IDs require visible ASCII1..128, default generator crypto/rand16 bytes hex; invalid generated IDs panic without their content. Timeout(d) returns Middleware, d<=0 passes through. Recover(nil) returns empty500; ErrAbortHandler is absorbed without callback/write. These are approved public precisions, not inferred behavior.

## D51.1 and D51.2 — final user amendment

User approved a more precise contract after the first implementation increment; it supersedes the earlier clarification text wherever different. Host group is selected first (exact, wildcard, hostless), then its ServeMux matches method/path; a missing route in the selected group does not fall through to another host group. Wildcards contain exactly one DNS label, never IPs. CORS exact scheme://host[:port] values are lowercased, no path/trailing slash/null; * must be alone and cannot use credentials. Default methods GET/HEAD/POST; default headers safelisted; Headers[*] invalid with credentials. MaxAge seconds floor, zero omitted. Always Vary Origin, allowed explicit/credential origins echo request origin, invalid preflight403 empty without routing; disallowed normal origin adds no access-control headers. Recover(callback) and Timeout(d) both return (router.Middleware,error); callback mandatory, ErrAbortHandler re-panics; d>0 required. RequestID incoming ASCII printable1..128 only if opted in. These replace the earlier nil-Recover/no-op-Timeout/absorbed-Abort rules.

## Final implementation and internal choices

NewPolicy/New, Handle/Use/Freeze/ServeHTTP, ClientIP, ServerConfig/DefaultServerConfig/Serve/ServeListener and middleware subpackage implement D51 with final D51.1/D51.2. No Config.Validate/DefaultConfig was invented for the neutral Config: New/NewPolicy validate its required host policy. Constructors copy lists; CORS absence is the approved pointer-presence exception. RequestID empty Header default is the explicitly approved exception, not general literal normalization.

Host dispatch selects the existing group before native pattern matching, so exact/wildcard group404/405 never falls through. Patterns are validated on a throwaway ServeMux; the live mux never uses recover. Registration is locked, middleware factories execute outside the lock, and an immutable handler is published only after Freeze completes. Concurrent Freeze calls wait on a single completion channel; before publication requests503. Nil factory output becomes ErrInvalidCall; a factory panic propagates and leaves the sealed mux unavailable. Factories must not recursively Freeze the same mux. Route registration history is dropped after publication. No lock surrounds request callbacks.

Proxy parsing fails closed at malformed encountered XFF entries; an untrusted prefix before the first untrusted client is not inspected. An all-trusted chain selects its leftmost address. Empty/unparseable peer yields no ClientIP. No remote address mutation or forwarded-host authorization. CORS validates exact origins, lowercases for comparison, echoes the original request origin when approved, and preserves separate Vary values. Safelisted request header names are Accept, Accept-Language, Content-Language, Content-Type, Range; preflight cannot validate unsent header values. MaxAge floor below1s is omitted. Incoming ID printable ASCII includes space; generators must return a valid ID, otherwise a fixed panic without the ID. Recovery does not try to replace committed bytes.

Server listener ownership is protected by a once-close wrapper, covering TLS setup failures before native Serve takes ownership. Missing-certificate startup errors propagate and close the listener; already configured TLS listeners retain native TLS connections. Successful TLS handshake is tested using an in-memory self-signed certificate, a trusted client root and ephemeral loopback, never InsecureSkipVerify. Native HTTP defaults/transport behavior remain native; no signal, PORT/SCOPE, shutdown-hook registry or default handlers are added. Failed graceful drain closes server connections but cannot terminate arbitrary noncooperative handler code.

## Increments and final evidence

Policy/mux/middleware race tests passed before adapter implementation. Chi tests passed before operations tests. Consumers/guards were then integrated without script/Makefile/workflow edits. Final D51.1/D51.2 replaced earlier tentative CORS/middleware/JSON behavior; logs without final/acceptance names are historical increment evidence only. TLS listener cleanup and unbracketed request IPv6 rejection were added after review, followed by fresh profile, router fuzz/stress, consumers, guards and root regression. TLS handshake fixture was subsequently added without production changes and included in the final profile/root regression.

Commands (use installed Go1.27.4 and golangci-lint, local cache prepared; /private/tmp/logging-env.sh was this session's external PATH/cache helper, not a repository prerequisite):

```sh
(cd router && GOWORK=off go test -race -count=10 ./...)
(cd router && GOWORK=off GOMAXPROCS=1 go test -race -count=10 ./...)
(cd router && GOWORK=off go vet ./...)
make check-module M=router MODE=deps
make check-module M=router MODE=consumers
make check-module M=router MODE=quick
make check-module M=router MODE=stress
make check-module M=router MODE=fuzz
make check-module M=router MODE=bench
scripts/modules.sh validate
scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
scripts/check-module.sh health consumers
scripts/check-module.sh logging consumers
scripts/check-module.sh router/errors consumers
```

Actual platform macOS darwin/arm64, Apple M1 Max, Go1.27.4; Linux was not run or CI dispatched. Loopback tests run outside the sandbox because it forbids bind. No external service/listener was used. All commands passed in final applicable runs. Generic quick conservatively adds race/count20 at default/1/4 plus vet; no generic automation was modified to reduce it. Stress uses ROUTER_STRESS=1 via generic mode, count3: 10 rounds each of concurrent Freeze/request tests (32×50 requests) and 8 admitted requests draining during cancellation. Every blocked test handler/gate is released; native goroutineleak reports0. Freeze tests accept only503 before publication or the registered response afterward.

Fuzz30s per target: FuzzPolicy (host/Origin/XFF) 696,357 executions, FuzzPatterns 616,914 in the last acceptance run, both pass. Chi's FuzzHost and operations' FuzzSeconds have separate evidence in their modules. Source changes after extended fuzz affect only TLS startup cleanup and its tests where applicable; final post-cleanup syntax fuzz and seed suites are included. Benchmark b.Loop/benchmem,500ms/count3 covers1/50/500 routes against bare ServeMux with setup outside timing and bounded ResponseRecorder body. Measurements are local overhead, not simulated network latency/SLA. Full native route/policy work adds allocations; no zero-allocation claim. Last full coverage diagnostic before added TLS fixture:94.2% router/92.0% middleware; coverage is not proof.

Key ignored logs: evidence/l2/profile-final-tls-handshake.log, evidence/acceptance-fuzz.log, evidence/acceptance-stress.log, evidence/acceptance-consumers.log, evidence/post-tls-quick.log, evidence/acceptance-bench.log, evidence/l2/root-test-final-tls-handshake.log, root-lint-acceptance.log/root-arch-acceptance.log/root-versions-acceptance.log, final-root-lint-docs.log, related-*.log, graph-coverage-acceptance.log. Environment and original worktree in evidence/l2/environment-final.log. A sandbox-only module-cache stat warning in an earlier diagnostic graph run was superseded by the successful graph run with cache access; no guard was bypassed. Local evidence files are ignored, not claimed as versioned deliverables; summaries and source/evidence hashes are versioned.

The router composed consumer repeats explicit host policy, ID middleware, error mapping/recovery, caller logging, real health registry, separate operations handler and context-controlled server. These assembly relationships and the cross-check WriteTimeout>MaxProfileDuration belong in presets/httpapi; none becomes an implicit tool default here. Chi/operations supply composed companions required by catalog convention without creating artificial external dependencies.

Gorelease executed outside module go.mod in a temporary production-source copy with `GOWORK=off GOPROXY=off /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0`: exit0, valid first-version semantic version, no published base comparison. No tags/proxies or dependency updates.

E1 complete (approved contract/inventory/docs); E2 complete locally for stated profiles; E3 complete in-memory HTTP plus explicit loopback/TLS lifecycle; E4 stdlib-only module/minimal and real composed offline graphs pass; E5 first-version gorelease passes for router; E6 proposed independent v0.1.0 unapproved, unreleased/- retained; E7 real-tag consumers deferred. make all was not executed or claimed; requested individual root checks were. No production legacy edit, commit, push, tag or release dispatch. Existing L1 work is not attributed to L2.
