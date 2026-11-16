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

## Historical checkpoint (superseded by D52.3 and completion below)

auth/jwt core implementation/qualification and real core+health consumers are complete locally (see ../RELEASE_NOTES.md). HTTP implementation, go.mod/catalog/workspace registration, consumers and E2–E5 are deliberately pending the unanswered public copy guarantee clarification: define Custom as JSON trees and ContextWithClaims with nonnil ctx, rather than promising universal deep copying of arbitrary/cyclic Go objects. D52.1/.2 are approved and not reopened. No HTTP candidate or passing HTTP tests are claimed.

## D52.3 user approval — resume

Custom is a JSON tree, without arbitrary Go objects or cycles; ContextWithClaims requires a nonnil context. This closes the pending public precision and supersedes the earlier pending-checkpoint text. HTTP helpers deep-copy the JSON domain (map[string]any, []any, strings/bools/numbers/nil, also []string) and Audience/Groups; they do not promise universal arbitrary-object cloning. No earlier D52.1/.2 point is reopened. Resumed worktree changes are the existing L3 core files/references, preserved and not reattributed to HTTP implementation.

## Completed implementation and internal choices

D52.3 is implemented before the HTTP helpers: ContextWithClaims requires a nonnil parent and copies Audience/Groups plus the acyclic JSON Custom tree; ClaimsFromContext copies again. Standard time.Time/string fields are values. No universal arbitrary-object/cycle clone or authenticity inference from a context value. A private key prevents accidental package collisions, but trusted application code can intentionally inject any claims.

Config/Dependencies, consumer-side Verifier/ErrorWriter and Middleware follow the approved signatures. No new exported HTTP error types/sentinels were invented: validation uses existing auth/jwt.ConfigError/ErrInvalidConfig with fixed field/reason. Header literals validate as HTTP tokens; skip entries must be canonical absolute paths; empty required-group entries are invalid. Optional empty groups permit authenticated users without a group gate. Captured config lists are copied. Nil verifier interface/typed nil pointer is rejected in construction; nil next yields nil. Neither construction stage runs Verify/OnError, starts resources, derives a context or creates goroutines.

Internal security choices: duplicate authentication header values are treated as absent rather than choosing one; space-separated case-insensitive Bearer accepts repeated surrounding spaces but no embedded token whitespace. Skip matching never canonicalizes input into a public route. Dot/double/backslash/percent/RawPath ambiguity fails closed to authentication, including benign encoded public URLs; query strings do not affect path comparison. Exact skip versus explicit trailing-slash subtree is deliberate and covered by an independent bounded fuzz property. Caller rewrites must be deliberately ordered outside this boundary.

Verifier receives the exact caller context and token. It remains responsible for verification; no SDK/issuer/claims heuristics are added. Successful claims are stored in a child context preserving values/deadline/cause; cancellation does not independently turn a successful custom verifier result into a failure. No mutation of invocation headers, URL, method or body. Group gate is exact OR through jwt.RequireGroup. Unavailable precedence before invalid/expired prevents an infrastructure cause matching another sentinel from becoming401; expired precedes general invalid. All failures short-circuit next.

Default problem JSON is a private fixed4-member DTO, fixed titles/status/code, no error formatting or token/claim publication. A verifier error whose Error method panics still produces a safe500. Missing token has a synthetic error wrapping jwt.ErrInvalidToken; other verifier errors preserve identity for OnError, forbidden uses jwt.ErrForbidden. Default401 challenge is Bearer with invalid_token parameter for invalid/expired, never error_description. With OnError the middleware does not even touch Header/WriteHeader/Write: the callback owns the complete response/challenge/privacy. User callbacks/panics are not isolated/recovered; this module adds no background work, retries or logging. Default writer errors are not retried or followed by another body; net/http.Handler has no error return.

## Real consumers and independent graph

minimal: local fake verifier, no jwt.NewVerifier/HTTPClient, no network, claims/context/group/header/skip/default errors on httptest.ResponseRecorder. The graph honestly includes auth/jwt and its JWT dependency because the public Claims/error types live there; this is not falsely advertised as stdlib-only or free of the base module.

composed: real auth/jwt, router, router/errors and health, memory EC JWKS RoundTripper, generated signed verification fixtures. Native router policy/Freeze plus HTTP middleware protect a staff route; exact public route skips verification and prefix lookalike does not. OnError writes with router/errors and explicitly emits the Bearer challenge. Health.CheckerFunc calls Verifier.Refresh: failed JWKS availability makes readiness fail, while a known fresh token still verifies. Unknown kid yields503 and repeated failure within30s does not fetch again. No imitation, listener, service, engine, logging backend or SDK. Compile-time assignment proves *jwt.Verifier satisfies the local interface.

The composed consumer directly imports golang-jwt/v5 v5.3.1 only to sign local fixtures. This exact external module is declared in scripts/modules/consumer-deps.tsv for composed only; no new checker exception or script/Makefile/workflow edit. Base HTTP library/tests do not directly import the JWT library, only auth/jwt types/errors and stdlib. Replaces/requirements/graph are verified offline GOWORK=off, GOTOOLCHAIN=local, GOPROXY=off, GOFLAGS=-mod=readonly. Initial tidy attempted a sumdb lookup with an empty new sum file; copying the already-approved root v5.3.1 checksums resolved bootstrap, without disabling verification or changing versions.

## Executed evidence

Go1.27.4 darwin/arm64, Apple M1 Max; no Linux run/CI dispatch claimed. External session PATH/cache helper /private/tmp/logging-env.sh is not a repository dependency. Commands:

```sh
(cd auth/jwt/http && GOWORK=off go test -race -count=5 ./...)
(cd auth/jwt/http && GOWORK=off go vet ./...)
make check-module M=auth/jwt/http MODE=deps
make check-module M=auth/jwt/http MODE=consumers
make check-module M=auth/jwt/http MODE=quick
make check-module M=auth/jwt/http MODE=fuzz
make check-module M=auth/jwt/http MODE=bench
scripts/modules.sh validate
scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
make check-module M=auth/jwt MODE=consumers
make check-module M=router MODE=consumers
make check-module M=router/errors MODE=consumers
```

All functional/guard/regression commands pass. Generic quick conservatively includes race/count20/default,1,4 plus vet; no new automation exception. Light profile has no separate stress. A normal race test uses32 callers×32 requests with a shared fake verifier and independent writers; standard goroutineleak reports0. Tests cover all status/error types, exact4 JSON members/challenge/no reflection, config capture, ambiguous skip paths, custom header/duplicates, typed nil, no callbacks in construction, OnError exact identity/no writes, group OR and copies on set/get (including nested tree/json.Number), canceled context preservation and panic propagation.

FuzzPathsAndAuthorization:30s,883,466 executions, passes; bounded4096-byte combined paths/headers, no sockets. BenchmarkMiddleware b.Loop/benchmem,500ms/count3:254–260ns/op,576B/op,5allocs/op on this machine. It includes fake verifier's atomic count/group fixture and context copies, no signing/network/sleep or serialization on the success path, not a zero-allocation/throughput/SLA claim. Coverage97.6% diagnostic, not proof. Local raw logs under evidence/ are ignored; versionable summary/commands/source and log hashes remain. Key logs profile-lint.log, check-{deps,consumers,quick,fuzz,bench}.log, graph-coverage.log, environment.log, final-root-*.log, root-lint-docs.log and related-*.log.

Gorelease actually executed outside this go.mod in a fresh temporary source-tree copy preserving development replace layout, GOWORK=off/GOPROXY=off, `gorelease -base=none -version=v0.1.0`. Exit1: cannot resolve github.com/skolldire/go-engine/auth/jwt@v0.0.0 with module lookup disabled. First version without a published base; this is a distribution impediment, not passing compatibility evidence. No tag/proxy or dependency change was invented. Core gorelease passes separately. See evidence/gorelease.log.

E1 contract/inventory/docs complete; E2 light functional/security/race/fuzz/bench complete locally; E3 in-memory HTTP/default+OnError integration complete; E4 independent graph and real composition pass; E5 executed, blocked by unpublished base dependency; E6 independent v0.1.0 proposed, unapproved/unreleased/-; E7 real-tag consumers deferred. No remaining public query, commit, push, tags, index mutation, legacy migration or release workflow edit/dispatch. make all was not run/claimed; requested individual checks were.

Final read-only Git audit: HEAD remains dffbefd0084b6282702ec4ecc5b1881013381d24. The index acquired staged L3 changes during the session outside this agent's commands; they were preserved, never reset/restaged. No git add/reset/checkout/commit/push/tag command was executed. Remaining worktree differences from that staged snapshot are final documentation/manifests; no legacy/pkg/aws, root go.mod/go.sum, Makefile or workflow edits appear in the staged diff. Source hashes describe the final worktree candidate, not a promise that its staged snapshot is identical.
