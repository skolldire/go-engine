# D52 L3 — approved contract and initial evidence

Branch feature/split, HEAD dffbefd0084b6282702ec4ecc5b1881013381d24; initial git status --porcelain empty. Go1.27.4 darwin/arm64. Root resolves github.com/golang-jwt/jwt/v5 v5.3.1; exact version reused without dependency/directive updates. L1/L2/health/utilities are pre-existing committed work, not attributed here. No index, commit, push, tags or release workflow changes/dispatch. Never touch files containing " 2".

D52 architect-approved. Core Config: required Issuer/Audience/JWKSURL; AudienceClaim D aud, GroupsClaim D groups; Algorithms D [RS256] subset RS256/384/512, PS256/384/512, ES256/384; CacheTTL1h>0, RefreshTimeout5s>0, ClockSkew30s>=0, StaleGrace0>=0, MaxTokenBytes16KiB>0, MaxJWKSBytes1MiB>0. Snake_case mapstructure/json tags. Dependencies.HTTPClient *http.Client required, borrowed. DefaultConfig, Validate, NewVerifier(Config,Dependencies), Verify(ctx,token)(Claims,error), Refresh(ctx)error, RequireGroup(Claims,group)error. Claims: Subject/Issuer/ID strings, Audience/Groups []string, Expiry/NotBefore/IssuedAt time.Time, Custom map[string]any. Errors ErrInvalidConfig/ConfigError{Field,Reason}, ErrInvalidCall, ErrNilContext, ErrInvalidToken, ErrExpiredToken and ErrUnknownKey also matching ErrInvalidToken, ErrKeySetUnavailable, ErrForbidden. No network/goroutine on construction. HTTPS endpoint, HTTP only explicit loopback/localhost.

D52.1 user-approved: AudienceClaim and GroupsClaim nonempty in declaration-order validation, literal empties invalid. D52.2 user-approved: unknown-kid refresh failure wraps ErrKeySetUnavailable and cause, consumes30s window; within window reuse last failed outcome (503) or successful-missing outcome (ErrUnknownKey/401). Known fresh keys unaffected. Explicit Refresh bypasses unknown-kid throttle. No map per hostile kid.

HTTP: Config{Header D Authorization, SkipPaths optional, RequiredGroups optional OR}, Dependencies{Verifier local interface Verify(context.Context,string)(jwt.Claims,error), OnError ErrorWriter optional}. Middleware(Config,Dependencies)(func(http.Handler)http.Handler,error), ClaimsFromContext and ContextWithClaims. No router/JWKS-client construction. Stable status/code mapping, problem+json fixed members type/status/title/code, no descriptions/causes. Missing/invalid401, expired401, forbidden403, unavailable503, other500; RFC6750 Bearer challenge without error_description. OnError once with original error, no body/status writes by middleware. Context copy-domain precision is separately awaiting approval before HTTP helper implementation.

## Actual inventory and migration map

| origin | destination | action | proof required |
|---|---|---|---|
| pkg/router jwksCache, manual RSA n/e, RS256 | auth/jwt RSA/EC and explicit algorithms | adapt/develop | algorithm confusion and signed fixtures |
| AWS JWKSClient via jwx, RSA-family token parser | same provider-neutral verifier | future replacement only; preserve production | no AWS/jwx in graph |
| optional issuer/audience opt-outs | required literal configuration | retire from new API | validation and mismatch tests |
| implicit aud OR client_id, cognito groups/username/token_use | configurable claims and Custom | adapt | Cognito-shaped fixture without Cognito imports |
| router singleflight/proactive refresh/stale6h since fetch | stdlib flight, lazy TTL, explicit stale grace | develop | concurrent/canceled/virtual time tests |
| AWS mutex-held fetch and unbounded stale fallback | bounded current key snapshot | replace in future, not migrate | real inventory distinction |
| router HTTPClient default10s | required borrowed client | adapt | New no fetch/no close |
| legacy shouldSkip exact or p+slash for every entry | exact unless trailing slash explicitly declares subtree | adapt | bypass fuzz and boundaries |
| Bearer case-sensitive prefix and CommonApiError ER-401 | RFC6750 scheme comparison/fixed problem JSON or OnError | adapt | exact wire/no cause |
| mutable *Claims context, shallow Raw copy | per-call Claims/Custom copy | develop | mutation isolation |
| RequireGroup router middleware | core exact group predicate and HTTP OR groups | separate | success/forbidden/invalid call |

Confirmed deviations from architect's starting facts: AWS uses lestrrat-go/jwx/v2/jwk.Fetch/PublicKey.Raw, not manual n/e; only router uses singleflight, AWS uses a mutex held during fetch; AWS accepts *SigningMethodRSA variants, whereas router pins RS256. Router stale cap is6h since last successful fetch (not6h after failure); AWS fallback has no matching age cap. Legacy shouldSkip is already path==p OR HasPrefix(path,p+"/"), so /health does not skip /healthz but does skip /health/a; it is not an unrestricted raw prefix. The new exact/subtree distinction is still deliberate. Legacy Raw copies only top-level entries. Consumers include preset/full quickstart and tests, Cognito session/mfa/token methods, and router JWT/group tests. None are migrated.

SDK v5.3.1 source/go doc inspected: WithValidMethods precedes keyfunc; signature verification precedes Claims validation; exp not required by default, iat opt-in, nbf checked by default. Use explicit WithExpirationRequired, WithIssuedAt, WithLeeway, WithJSONNumber and fixed expected issuer. SDK error strings may include values; new errors use fixed categories instead of reflecting parser text. JWK data/HTTP response bodies never become errors verbatim.

E1 contract/inventory recorded before code. E2–E5 pending. E6 proposed independent v0.1.0 unapproved, unreleased/-; E7 authorized tag consumers deferred. Actual local platform only; Linux CI not run/dispatched.

## Core implementation and internal decisions

Config/errors compiled first, then RSA/EC JWKS parsing and stdlib single-flight cache, then signed verification and boundary tests. Only after core race tests passed were stress/fuzz/bench and independent consumers registered. D52.1/.2 are implemented; HTTP copy-domain precision remains unanswered and the HTTP package has not been implemented or registered. L3 as a whole is not complete.

Internal choices: one transient worker per fetch (necessary so the initiator can cancel its wait without aborting shared work), no maintenance goroutine or goroutine per waiter. Worker/HTTP/client callbacks run outside mu. A completion channel publishes exactly one result; a late result observed after RefreshTimeout is not installed. Callers that supply uncooperative transports/readers can retain that one flight; no forced cancellation or Close is invented. Configuration and default algorithm slices are copied. HTTPClient is borrowed; per-fetch shallow copy preserves its transport/jar/timeout/custom redirect policy while enforcing configured endpoint scheme constraints on redirects, without mutating/closing the original.

A successful download atomically replaces one current key map; no historical maps or negative-kid map. One last unknown-triggered error/time record enforces D52.2. Unknown lookups join an active unknown-triggered flight; within its completed window they reuse its outcome even if an unrelated explicit refresh is in flight. Known fresh keys never wait for refresh. Expiry/grace comparisons use chained time.Time.Add calls, avoiding duration sum overflow. Initial and expired-TTL loads are distinct from fresh-cache unknown-kid throttling. Explicit Refresh bypasses that throttle. Old data is served only within explicit grace and only for a key actually present.

JWK malformed entries are discarded; duplicate usable kid entries are discarded together rather than selecting arbitrarily. Native ecdsa.ParseUncompressedPublicKey validates points; no deprecated coordinate mutation/unsafe is used. RSA modulus/exponent parsing checks nonempty positive odd values and exponent representability. Additional cryptographic native rules are enforced during signature verification; no new strength-config field is invented. alg/use/key family/curve checks happen before native signature verification. Token-provided key URLs never affect the configured endpoint.

Strict base64url/UTF-8/single JSON-value checks close an observed v5.3.1 UseNumber decoder gap (trailing JSON otherwise accepted). Critical JOSE extensions and unencoded payloads are unsupported and rejected with fixed reasons; ordinary untrusted key URL headers are ignored. Native parser validates signature, required exp, optional nbf/iat, exact issuer and skew. Configured audience/groups and optional subject/jti types are checked explicitly. No raw SDK validation text, token, signature, kid or claim values are reflected by module-owned invalid-token errors. External HTTP failures preserve their causes and can contain foreign endpoint data; they are not redacted. Resource response bodies close on successful/failed refresh, including exact/over-size boundaries. A reentrant known-key Verify during the HTTP callback proves callbacks execute without the state lock.

## Executed core evidence

Actual Go1.27.4 darwin/arm64, Apple M1 Max; no Linux/CI execution claim. The external /private/tmp/logging-env.sh supplies this session's toolchain/cache paths; it is not a repository runtime prerequisite. No root directives/dependencies or existing production/consumers changed. Current module requires only golang-jwt/v5 v5.3.1; standalone minimal includes the same dependency, composed adds real health. No socket/service in JWT tests/consumers.

```sh
(cd auth/jwt && GOWORK=off go test -race -count=10 ./...)
(cd auth/jwt && GOWORK=off GOMAXPROCS=1 go test -race -count=10 ./...)
(cd auth/jwt && GOWORK=off go vet ./...)
make check-module M=auth/jwt MODE=deps
make check-module M=auth/jwt MODE=consumers
make check-module M=auth/jwt MODE=quick
make check-module M=auth/jwt MODE=stress
make check-module M=auth/jwt MODE=fuzz
make check-module M=auth/jwt MODE=bench
scripts/modules.sh validate
scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
make check-module M=router MODE=consumers
make check-module M=router/errors MODE=consumers
```

Commands pass. Generic quick additionally runs race/count20 at default/1/4 plus vet; no script/override edits. Final security profile includes forged HS256-with-RS256-header, corrupted signature and wrong EC curve, alongside all8 allowed algorithms and the required claim/cache/cancellation cases. TTL/grace/skew/30s-window tests use testing/synctest, channels and barriers, no precision sleeps. Unknown failure repeats return ErrKeySetUnavailable with cause; a known fresh token succeeds during the outage; successful missing refresh repeats ErrUnknownKey without network. Initiator cancel preserves cause while32 other callers share one fetch.

Separate stress: generic count3, eight rotations/outages per run with64 concurrent Verify calls and controlled cancellations, real RSA keys, virtual TTL. Native goroutineleak reports0; retained-state test uses1000 hostile unknown-kid lookups and observes one key map/no active worker, not per-kid history. Fuzz30s each: FuzzToken328,413, FuzzJWKS712,469, FuzzJOSE461,064 executions, all pass. FuzzToken repeated after refining the privacy property to avoid accidental equality with fixed error text:180,308 executions pass. The production code is unchanged between these campaigns and final security tests.

Benchmarks use b.Loop/benchmem,500ms/count3, warmed key cache for RS256/ES256 versus direct golang-jwt with same key/valid claims/issuer/skew. No key generation, fetch or simulated network wait in timed loop. The wrapper additionally enforces configurable audience/groups/copies/strict encoding, so valid-path fixture equivalence is not a claim of identical policy. No zero-allocation/SLA claim. Last coverage diagnostic89.8%, used for gaps, not proof.

Root lint/arch/docs/version/test and related router/router-errors consumers passed. Root tests/related router consumers required loopback access outside sandbox; core JWT never did. Local logs under evidence/ are ignored and not shipped/versioned evidence. Reproducible commands, summary and SOURCES/EVIDENCE digests remain versionable; key logs final-security-profile.log, final-profile-graph.log, check-*.log, fuzz/fuzz-*.log, final-fuzz-token.log, root-*.log, related-*.log, gorelease.log.

Gorelease outside go.mod: copied production files/go.mod/go.sum to a fresh /private/tmp directory, `GOWORK=off GOPROXY=off /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0`; exit0, valid semantic version. First version/no published base, no invented history or tag/proxy.

Core E1–E4 complete for the executed scope; E5 passes first-version check; E6 proposed independent v0.1.0 unapproved, catalog unreleased/-; E7 real-tag consumers deferred. HTTP E1 contract draft only/E2–E5 pending the requested copy-domain precision. No commit, push, tags, index mutation, release workflow edit/dispatch, aliases or legacy migration.

## D52.3 user approval — resume

Custom is a JSON tree, without arbitrary Go objects or cycles; ContextWithClaims requires a nonnil context. This closes the pending public precision and supersedes the earlier pending-checkpoint text. HTTP helpers deep-copy the JSON domain (map[string]any, []any, strings/bools/numbers/nil, also []string) and Audience/Groups; they do not promise universal arbitrary-object cloning. No earlier D52.1/.2 point is reopened. Resumed worktree changes are the existing L3 core files/references, preserved and not reattributed to HTTP implementation.

## L3 completion after D52.3

The earlier HTTP-pending checkpoint is superseded: auth/jwt/http is implemented, registered and qualified with copied JSON context helpers, exact/subtree skip, explicit verifier interface, safe default mapping and complete OnError response ownership. Its own README/MIGRATION/CHANGELOG/RELEASE_NOTES, consumers, graph and source/evidence hashes are separate. The real composed HTTP consumer verifies known fresh keys during a JWKS outage, repeats unknown-kid503 consistently without another fetch, and makes health readiness fail through Refresh. No public queries remain.

Final review additionally rejects a JWKS body containing invalid UTF-8, instead of encoding/json silently replacing invalid bytes in key metadata. This implements the existing invalid-JSON failure guarantee without new fields/defaults/errors. An explicit memory test and full race/count10/default+1 profiles/lint pass after this production change; targeted FuzzJWKS30s passes3,942 executions in that run (original broader JWKS campaign712,469 remains historical evidence). No throughput claim is inferred from fuzz counts. Core coverage/bench/stress earlier results refer to their recorded workloads; cached verification benchmark behavior is unaffected by the fetch-only UTF-8 check. Original core full campaigns are not falsely claimed rerun in this phase.

Both core and HTTP consumers pass again offline; root lint/arch/docs/versions/test and related router/router-errors consumers pass after integration (root tests use loopback permission only for legacy/router fixtures). Catalog/workspace/consumer external dependency data are updated without Makefile/workflow/script changes. New external composed dependency is the already resolved v5.3.1 for local fixture signing only. No legacy Go production or root dependency/directive was changed.

Core gorelease is rerun on final production sources outside go.mod, no published base/tag comparison, proposed v0.1.0. HTTP gorelease remains an explicit unpublished-core dependency impediment, not a functional integration failure. E1–E4 are complete for both modules on actual macOS ARM64; core E5 passes, HTTP E5 executed/blocked; E6/E7 approvals/tags remain deferred. Neither module is declared published/stable. Local ignored logs are not versioned deliverables; commands/results here and manifests identify the candidate. Final phase logs: evidence/d52-3-final-profile.log, d52-3-fuzz-jwks.log, d52-3-deps.log, d52-3-consumers.log and HTTP evidence/final-root-*.log. No commit, push, tags or release workflow dispatch.

Final read-only Git audit: HEAD remains dffbefd0084b6282702ec4ecc5b1881013381d24. The index acquired staged L3 changes during the session outside this agent's commands; they were preserved, never reset/restaged. No git add/reset/checkout/commit/push/tag command was executed. Remaining worktree differences from that staged snapshot are final documentation/manifests; no legacy/pkg/aws, root go.mod/go.sum, Makefile or workflow edits appear in the staged diff. Source hashes describe the final worktree candidate, not a promise that its staged snapshot is identical.
