# auth/jwt/http

Experimental D52/D52.1–D52.3 candidate, not released. Package `http`; import as `jwthttp` alongside stdlib `net/http`. It depends only on auth/jwt types/errors and stdlib, not a router, backend or JWKS construction. The auth/jwt module brings its declared golang-jwt dependency transitively; a fake verifier needs no HTTP client or network.

```go
cfg := jwthttp.DefaultConfig()
cfg.SkipPaths = []string{"/health", "/public/"}
cfg.RequiredGroups = []string{"staff", "admin"} // OR
wrap, err := jwthttp.Middleware(cfg, jwthttp.Dependencies{Verifier: verifier})
// Handle err, then:
handler := wrap(myHandler)
```

The consumer-side Verifier interface is `Verify(context.Context,string)(jwt.Claims,error)`. *jwt.Verifier satisfies it; a local implementation works equally. Dependencies.OnError is optional `func(http.ResponseWriter,*http.Request,int,error)`. Config has snake_case mapstructure/json tags. Default Header is Authorization; SkipPaths/RequiredGroups default nil. Literals validate unchanged: Header must be a nonempty HTTP token, skip paths canonical absolute paths and required group entries nonempty. Validation/construction use existing *jwt.ConfigError/ErrInvalidConfig, not new universal errors. Config is validated before the verifier; nil interface or typed nil pointer is rejected. Other invalid implementations are not universally detectable.

Middleware construction copies slices/functions, performs no Verify or error callbacks, starts no goroutines/timers and owns no resource. It returns a middleware; nil next yields nil rather than a substitute handler. Shared use is safe when verifier, OnError and next satisfy their own concurrent contracts. No lock surrounds callbacks, no history/cache lives in this module. Callback/handler panics propagate; no retry, timeout, logger or recovery is installed.

Bearer scheme comparison is case-insensitive; the separator is a space and repeated spaces around the token are accepted. Missing/other scheme, empty token, ambiguous multiple header values or embedded whitespace in a token are treated as missing. The configured header is read without cookies/body fallbacks. Verification receives the exact request context/token and its result is trusted; this layer does not reinterpret issuer, dates, token_use or algorithms. A custom verifier is responsible for its own verification guarantees and returns/wraps auth/jwt sentinels to select the documented status mapping; arbitrary SDK errors are not heuristically classified.

A SkipPaths entry without final slash is exact: /health does not skip /healthz, /health-admin, /health/ or /health/a. A trailing slash declares a subtree: /public/ skips itself and /public/a, not /public. Query strings do not participate. Root / explicitly makes all canonical paths public; use it only intentionally. The middleware never cleans a dangerous path into a public path: dot segments, double slashes, backslashes, percent escapes or nonempty RawPath cannot bypass authentication. Encoded Unicode/otherwise benign encoded public paths also require authentication under this conservative policy. Paths are passed on unchanged; downstream rewrites must be ordered deliberately, not relied on to repair authentication boundaries.

If RequiredGroups is nonempty, any exact membership grants access; require all groups explicitly in the handler with jwt.RequireGroup when needed. Failed authorization is403. A canceled request is still delegated to the verifier; no context is substituted/derived until claims are stored. Successful verification preserves cancellation, cause, deadline and values through a child context and forwards the same request metadata/body/URL.

Default failure responses contain exactly type="about:blank", status, fixed standard title and code, Content-Type application/problem+json. No detail, token, claims, cause, error text or stack is encoded.

| Failure | Status | Code |
|---|---|---|
| absent token | 401 | missing_token |
| invalid/unknown key | 401 | invalid_token |
| expired | 401 | expired_token |
| missing required group | 403 | forbidden |
| key set unavailable | 503 | auth_unavailable |
| other verifier failure | 500 | internal_error |

Unavailable takes precedence over invalid/expired if a transport cause happens to match another sentinel. Expired precedes general invalid. A401 default adds WWW-Authenticate: Bearer; invalid/expired adds error="invalid_token", without error_description. The module never formats verifier errors, even ones whose Error method panics. Writes are not retried or followed by another response.

OnError takes complete ownership of headers/status/body and is invoked once with the original verifier error; synthetic missing-token failure wraps jwt.ErrInvalidToken with a fixed reason, authorization uses jwt.ErrForbidden. The middleware itself does not even call writer.Header in this branch. The application must emit its desired challenge and public response, and must not serialize the private error. The composed consumer shows router/errors and an explicit Bearer challenge. Foreign callbacks can deliberately publish data; the default privacy guarantee cannot constrain their implementation. Logging and writer-failure observation remain application responsibilities.

ContextWithClaims(ctx,claims) and ClaimsFromContext(ctx)(jwt.Claims,bool) copy Audience, Groups and Custom on set/get. D52.3: ctx passed to ContextWithClaims must be nonnil, as for context.WithValue. Custom is an acyclic JSON tree: map[string]any, []any, strings/bools/numbers/json.Number/nil (also string slices), no arbitrary Go objects. The helper does not promise universal object cloning or validate cycles. ClaimsFromContext returns false on absence/nil context. These helpers do not authenticate: anyone in the application can inject claims. Security comes from the middleware/trusted verifier, not possession of a context value.

See MIGRATION/RELEASE_NOTES and independent minimal/composed consumers. Minimal uses a local verifier; composed uses real auth/jwt, router, router/errors and health with an in-memory JWKS RoundTripper. No SDK, socket, service or framework. Proposed v0.1.0 remains unapproved/unreleased; no published stability claim.
