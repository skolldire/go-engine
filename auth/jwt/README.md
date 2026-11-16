# auth/jwt

Experimental D52/D52.1/D52.2 verification candidate, not released. Go1.27.4; only stdlib and the repository's existing golang-jwt/v5 v5.3.1. No provider SDK, engine, router, logger or x/sync. No token issuer, login, OAuth/session lifecycle or network in NewVerifier.

```go
cfg := jwt.DefaultConfig()
cfg.Issuer = "https://issuer.example"
cfg.Audience = "application"
cfg.JWKSURL = "https://issuer.example/.well-known/jwks.json"
verifier, err := jwt.NewVerifier(cfg, jwt.Dependencies{HTTPClient: myClient})
// Handle err; explicit preload is optional:
err = verifier.Refresh(ctx)
claims, err := verifier.Verify(ctx, token)
// After handling verification errors:
err = jwt.RequireGroup(claims, "staff")
```

Config.Validate checks fields in declaration order without normalization or completing zero literals. Issuer/Audience/JWKSURL and AudienceClaim/GroupsClaim are nonempty. Defaults: AudienceClaim aud, GroupsClaim groups, Algorithms [RS256], CacheTTL1h, RefreshTimeout5s, ClockSkew30s, StaleGrace0, MaxTokenBytes16KiB, MaxJWKSBytes1MiB. Required names stay empty in DefaultConfig. Every returned default slice is independent; NewVerifier copies Algorithms. Durations TTL/refresh and limits must be positive; skew/grace nonnegative. All Config fields have snake_case mapstructure/json tags.

HTTPS JWKSURL is required except explicit HTTP loopback IPs or localhost. Userinfo, opaque URLs and fragments are rejected. Redirects are checked against the same endpoint policy; the borrowed client's transport/jar/timeout and custom redirect callback are preserved through a per-fetch shallow client copy. It is never mutated or closed, nor are idle connections closed. Do not mutate the supplied client/transport concurrently. Endpoint policy does not perform DNS pinning or promise an SSRF sandbox for caller-supplied transports/proxies/redirect callbacks. Tokens never supply key URLs: jku/x5u are ignored.

Allowed algorithms: RS256/384/512, PS256/384/512, ES256/384. HS*, none and other names are invalid configuration. The JOSE alg allowlist is enforced before key lookup; kid is required. JWK alg (if present), use=sig (if present), RSA/EC type and P-256/P-384 curve must correspond. Invalid keys are discarded; duplicate usable kid entries are discarded together to avoid ambiguous selection. An empty usable set fails. Key material is decoded only from RSA n/e or EC crv/x/y, not private keys/certificates or token-provided material. Native ECDSA point validation rejects invalid/off-curve/infinity points. RSA verification uses native crypto and golang-jwt rules; no extra configurable minimum/maximum key strength policy is invented.

Verify rejects nil ctx, empty/oversized input before decoding, and canceled contexts before work. Signature validation uses WithValidMethods. Base64url decoding is strict; UTF-8 and exactly one JSON value per header/payload are checked before the SDK. This closes v5.3.1's UseNumber decoder accepting trailing JSON. Unsupported critical JOSE extensions and b64=false are rejected; ordinary unknown header metadata does not control fetching. The signature is checked before accepting claims. exp is mandatory; nbf and iat are checked with ClockSkew using native NumericDate precision. Non-number/nonfinite/unrepresentable dates are rejected. Issuer equality is exact. AudienceClaim accepts string or a list of strings containing Audience. GroupsClaim, when present, must be a list of strings; absent means empty. Subject and jti, when present, must be strings. No Cognito claim is implicitly special.

Claims returns value fields and independently owned Audience/Groups/Custom JSON trees. Custom contains remaining claims (configured audience/groups names and iss/sub/jti/exp/nbf/iat are extracted); UseNumber preserves JSON numeric values. Expiry is required, NotBefore/IssuedAt are zero when absent. Modifying a result cannot modify the verifier or a subsequent result. RequireGroup is exact and case-sensitive, empty group is ErrInvalidCall and missing membership ErrForbidden.

New is pure; Refresh and cold/expired Verify start only demand-driven fetches, never proactive maintenance. One HTTP fetch is in flight per instance; waiters use a channel and can leave on their own context. The shared fetch uses WithoutCancel(initiator)+RefreshTimeout and preserves its values, so initiator cancellation/deadline does not abort other callers. One transient worker owns each fetch; no goroutine per waiter, timer/ticker maintenance or history. Borrowed HTTP transports/readers must cooperate with cancellation and must not panic/Goexit; arbitrary noncooperative callbacks cannot be forcibly terminated. A refresh result observed after its deadline is not installed.

Only status200 and a complete UTF-8 JSON body within MaxJWKSBytes are accepted, including the exact-size boundary. Body is closed; response text is never reflected in errors. Cache freshness starts at successful completion; TTL is lazy. A failed refresh retains the current snapshot but default StaleGrace0 fails closed for expired keys. Positive grace permits an existing expired key strictly before fetched+TTL+grace; no duration addition overflow is used. Signature failure with a known kid does not automatically fetch; providers should rotate kid, or the application calls Refresh.

Unknown kid with fresh keys triggers one refresh at most every30s, a fixed contract constant. A successful refresh still missing that kid yields ErrUnknownKey. Failed unknown-kid refresh yields ErrKeySetUnavailable and its cause; failure also consumes the30s window. Repeats within it return that last outcome without a new fetch; known fresh keys remain usable. Explicit Refresh bypasses this throttle, joins an existing fetch, never creates a second simultaneous fetch, and does not evict good keys on failure. Initial/TTL loads are separate from the unknown-kid throttle. Only one key-set snapshot and one bounded last-outcome record are retained, not a negative cache by hostile kid. MaxJWKSBytes bounds input, not all CPU/memory or arbitrary callback allocations.

Errors: ConfigError{Field,Reason} wraps ErrInvalidConfig without values; ErrNilContext, ErrInvalidCall, ErrInvalidToken, ErrExpiredToken/ErrUnknownKey (both also invalid), ErrKeySetUnavailable and ErrForbidden. Invalid-token errors use fixed reasons, no token/signature/claim values or SDK error text. HTTP failures preserve causes with errors.Is/errors.As; foreign transport errors may contain their own endpoint/callback data and are not a redaction service. Cancellation preserves ctx.Err/context.Cause. No panic recovery, observability, globals or resource Close is added.

Cognito recipe (configuration only): Issuer `https://cognito-idp.{region}.amazonaws.com/{pool_id}`, JWKSURL that issuer plus `/.well-known/jwks.json`, AudienceClaim `client_id` for access tokens or `aud` for ID tokens, GroupsClaim `cognito:groups`. Check Custom["token_use"] explicitly in your handler before using the intended token type. No AWS endpoint inference or token_use classification occurs here.

See MIGRATION, RELEASE_NOTES and independent minimal/composed consumers with memory RoundTripper, no sockets. Proposed v0.1.0 remains unapproved/unreleased; this is not published stability.
