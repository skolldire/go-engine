# router/errors — wave 2, L1 candidate

## R1 initial state and approved contract before code

Branch feature/split, HEADdfc3908098462049c3aa8e51b6e0925760c4bd7d (implements health), initial git status --porcelain empty. Go1.27.4 darwin/arm64. Existing health/utilities/telemetry/client work is committed and preserved, not attributed to L1. No index change, legacy production migration, existing directive/dependency update, workflow edit/dispatch, commit, push or tag. Files containing " 2" are excluded.

D49/D49.1/D49.2 are architect-approved. Public API: Config{IncludeRequestID bool; MaxDetailsBytes int}, json/mapstructure snake_case tags; defaults true/4096, literals unchanged, positive MaxDetailsBytes. Problem{Status int; Code,Title,Details string}; Dependencies{Classify func(error)Problem; RequestID func(context.Context)string}; Responder/NewResponder(Config,Dependencies)(*Responder,error); Write(context.Context,http.ResponseWriter,error)error. ConfigError{Field,Reason} unwraps ErrInvalidConfig; other sentinels ErrInvalidCall/ErrNilContext/ErrInvalidProblem. Classify is mandatory after Config validation. RequestID optional. No logger, generic HTTPError, options, context lookup, global provider or lifecycle.

Write validates ctx, writer (including nil pointer hidden in interface), then input error before callbacks. Classify receives the original error once, with no Error/String/Unwrap inspection by this module. Problem validates status400..599, ASCII Code1..64 ([A-Za-z0-9_.-]), nonempty valid UTF-8 Title≤256 bytes. Invalid core uses fixed500/internal_error/Internal Server Error with no detail and returns ErrInvalidProblem after the attempt, joining a writer error if present. Valid handled business error is not returned.

RFC9457 DTO is private, concrete and emits exactly type="about:blank", status, title, code; optional detail from Problem.Details and optional request_id. No instance or extra members. Content-Type application/problem+json; Cache-Control no-store; X-Content-Type-Options nosniff. Details omitted entirely if empty/invalid UTF-8/over MaxDetailsBytes (content bytes before JSON escaping). RequestID called once only when enabled/present, on exact ctx; valid UTF-8≤128 bytes and no control characters, otherwise omitted without normalization/error. ID only in body, no header inference/generation.

Internal decisions (authorized by the L1 rule): use unicode.IsControl (Cc including C0/C1) to implement the approved control-character rule; length checks precede UTF-8 scanning for over-limit inputs. Compact encoding/json.Marshal is used before Header/WriteHeader; field ordering follows the private DTO but clients should use JSON members. Valid request IDs may accompany the fallback because the approved pipeline evaluates RequestID after selecting the safe Problem. Exactly one WriteHeader and one Write are attempted, no retries; short count with nil error becomes io.ErrShortWrite, writer error is retained. No body newline is required/added. Only the three owned headers are changed; existing caller headers are neither inferred nor scrubbed. No lock, cache, goroutine, timeout or recovery is introduced.

## Inventory and migration map

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/utilities/error_handler/service.go CommonApiError/taxonomy/builders | caller Classify→Problem | adapt explicitly, no universal mapping | typed/wrapped error classification |
| HandleApiErrorResponse/WithRequest/Ctx | NewResponder + Write | replace new scope, preserve legacy | original error not serialized; writer errors returned |
| CommonApiError.Error includes wrapped cause | no automatic error formatting | exclude from public body | Error method that panics never invoked |
| cloned RequestID/Details map helpers | injected ID + bounded public string detail | deliberate schema/type change | exact JSON/limits/omission |
| HandleApiErrorResponseCtx logger and nil→Background | caller logging, nil ctx invalid | remove from new contract | no backend dependency, validation order |
| legacy WriteHeader then ignored Marshal/Write errors | prepare safe DTO first, one write, return failures | develop | partial/short/error writer tests |
| pkg/router/jwt_middleware.go writeAPIError | later HTTP/JWT adapter | preserve production | root regression, no auto auth classification |
| example/internal/handler/order.go writeError/writeJSON | local classifier integration recipe | preserve production | in-memory handler test |
| pkg/router/ping.go Encoder error then http.Error | outside error-response policy | exclude migration | no second response in new module |
| pkg/health/handler.go public dependency error strings | health Report.Err privacy + caller mapping | preserve legacy, compose new health | real composed consumer |
| chi RequestID middleware / implicit router middlewares | application-supplied accessor | exclude router coupling | no headers/context-key/global lookup |

Confirmed: legacy CommonApiError carries code/msg/request_id/details map, cause and HttpCode excluded from JSON; Error formats its underlying cause, errors.Is matches codes, copy builders avoid sentinel mutation. The handler classifies that concrete type, may log, writes application/json, ignores Marshal/Write errors and falls back for unknown errors. Its nil context is replaced with Background. JWT helper writes the concrete API error. Example maps selected sentinels but publishes err.Error for validation; it emits {error:message} and ignores Encoder failures. Legacy ping may call http.Error after Encoder failed. There is no existing independent responder with the D49 contract; docs alone are not implementation evidence. All these production files/consumers remain unchanged.

Verified toolchain docs: ResponseWriter headers must precede committed status/body; WriteHeader returns no error, Write can fail. JSON Marshal escapes strings and replaces invalid UTF-8, so public fields are validated/omitted before encoding rather than relying on replacement. DTO contains only int/string, no user MarshalJSON/any graph. Primary standard: https://www.rfc-editor.org/rfc/rfc9457.html sections3/3.2/4.2.1; application/problem+json and extensions are compatible, about:blank title has a SHOULD for the standard status phrase, not a MUST. Caller-specific titles remain approved. No copied RFC implementation/code.

Initial checkpoint (superseded by completion evidence below): R2–R8 pending; E1–E5 pending; E6 proposed independent v0.1.0 unapproved, catalog unreleased/-; E7 real-tag consumers deferred. Light risk profile: no separate stress campaign or Linux-local validation; many-response concurrency lives in the normal race suite, fuzz30s per target, only JSON-equivalent/simple/fallback benchmarks. Generic quick currently runs race/count20 at default/1/4 plus vet; reuse it unchanged alongside the explicitly requested count5 run, documenting its conservative extra checks rather than editing repository automation for one module. Linux CI catalog coverage is not claimed as executed locally.

## R2–R8 completed — reproducible evidence

R2 configuration/types/errors compiled and passed before R3; R3 safe DTO/fallback passed before R4; R4 ID/limits passed before R5; R5 write/partial-error tests passed before R6 qualification; R6 race/fuzz/bench fixtures passed before R7 independent consumers. Per-increment local logs are evidence/r2.log through evidence/r7-consumers.log. R8 adds README/MIGRATION/CHANGELOG and catalog/workspace/root references. Production legacy, existing dependencies/directives and shared automation are unchanged. Ignored local contract documents were updated before code; this file preserves the approved contract as a versionable artifact.

Executed on macOS darwin/arm64, Apple M1 Max, Go 1.27.4; environment/tool versions are in evidence/environment.log. Commands use the externally prepared toolchain/cache, not a repository dependency update:

```sh
source /private/tmp/logging-env.sh
(cd router/errors && GOWORK=off go test -race -count=5 ./... && GOWORK=off go vet ./...)
export ROUTERERRORS_RESULTS_DIR="$PWD/router/errors/evidence/campaigns"
make check-module M=router/errors MODE=deps
make check-module M=router/errors MODE=consumers
make check-module M=router/errors MODE=quick
make check-module M=router/errors MODE=fuzz
make check-module M=router/errors MODE=bench
scripts/modules.sh validate
scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
scripts/check-module.sh health consumers
scripts/check-module.sh logging consumers
(cd router/errors && GOWORK=off go test -coverprofile=evidence/coverage.out ./...)
```

All commands above passed in the final runs. Quick additionally runs race/count20 with default GOMAXPROCS, 1 and 4, and vet. Consumers run/test with race, GOWORK=off, GOTOOLCHAIN=local, GOPROXY=off and -mod=readonly; dependency/module/graph/replace guards pass. Minimal resolves only this module and stdlib. Composed resolves this module, health and logging; all are real implementations, no replacement imitation or remote service. A local ServeMux/ResponseRecorder maps real failed readiness to 503, keeps Report.Err/PanicError private and observes causes separately through caller logging.

Fuzz defaults were retained at 30s per target: FuzzConfig 1,135,780 executions, FuzzWrite 946,983 executions, both pass. The independent oracle checks core field/ID/detail validity, exact schema/fallback and callback counts through reuse. No stress campaign: approved light profile instead uses a normal race test with 32 callers × 50 responses, independent writers. The standard goroutineleak profile reports zero leaks; weak-reference/GC tests verify original errors are not retained. Controlled blocking writers are released, never abandoned.

Exactly three benchmarks, b.Loop, 500ms/count3, benchmem, io.Discard-backed writer: stdlib JSON equivalent 453–489 ns/op, 336 B/op, 6 allocs/op; simple responder 502–608 ns/op, 336 B/op, 6 allocs/op; fallback 530–578 ns/op, 376 B/op, 8 allocs/op. These local measurements include serialization/write-method overhead, no simulated I/O waits, and are not throughput/SLA promises. Coverage 96.8% is diagnostic; the concrete DTO's encoding/json.Marshal failure branch is unreachable for its int/string fields and is retained defensively.

The first sandboxed make test failed because legacy tests open local listeners (bind: operation not permitted), including pkg/health, pkg/router and legacy adapters. Repeated with listener access outside the sandbox: complete catalog make test passed, including router/errors. evidence/root-test.log preserves the sandbox failure; evidence/root-test-unsandboxed.log records the successful regression. No legacy fixes or hidden skips. Root lint, lint-arch, lint-docs and check-modules passed; health/logging consumer regressions passed. make all was not requested/executed for this light lot and is not claimed. Linux was not executed locally or dispatched; catalog-driven CI coverage is future execution, not local evidence.

Gorelease executed outside the module go.mod, in a fresh temporary directory containing go.mod and the three production files, with GOWORK=off/GOPROXY=off and `/private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0`: exit0, valid semantic version. This is the first version with no published base, not a historical API comparison. No artificial tags/proxy, no publication.

Local logs under evidence/ are ignored and are not versioned deliverables. SOURCES.sha256 records candidate/source and consumer hashes; EVIDENCE.sha256 records log hashes for local verification. Reproduce commands above rather than assuming ignored logs ship with the module. No files containing " 2" touched; git diff --check passes, index remains unchanged.

## Acceptance and remaining scope

- E1 complete: D49/D49.1/D49.2 contract, real inventory and migration documented; logger removed explicitly.
- E2 complete locally: unit/race/count matrix, bounded fuzz, standard leak/retention checks and three benchmarks. No remote durability/confidentiality guarantee beyond the approved structured DTO boundary.
- E3 complete: in-memory HTTP handler, controlled writer failures, no sockets/framework.
- E4 complete: stdlib-only module/minimal and real health/logging composition, offline graphs/replaces verified.
- E5 complete: gorelease first-version/no-base check outside go.mod, no publication claim.
- E6 pending user version approval: proposed independent v0.1.0, catalog stays unreleased/-.
- E7 deferred until an authorized real tag. Linux CI execution is not claimed.

No public contract changes beyond approved D49. Internal choices and tradeoffs are recorded above. No commit, push, tags, release workflow changes or dispatches.
