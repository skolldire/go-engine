# Migration — experimental, not released

Legacy pkg/router JWTAuth/RequireGroup and AWS Cognito production/consumers stay unchanged. No aliases, compatibility bridges or deprecations.

Before: `router.JWTAuth(router.JWTAuthConfig{...})` constructs verifier/client/cache and injects a mutable *router.Claims. After: construct `jwt.NewVerifier(jwtConfig,jwt.Dependencies{HTTPClient: client})` explicitly, then `jwthttp.Middleware(httpConfig,jwthttp.Dependencies{Verifier: verifier})`; install the returned wrapper on any net/http handler/router. Core issuer/audience opt-outs are removed; configure algorithms, AudienceClaim/GroupsClaim and StaleGrace explicitly (see ../MIGRATION.md).

| Before | New boundary |
|---|---|
| Middleware(verifier,Config) proposal | Middleware(Config,Dependencies), optional OnError |
| router-specific Claims pointer, shallow Raw | jwt.Claims copied into/out of context, JSON Custom domain |
| Cognito fields/implicit aud-client_id fallback | core claim-name configuration, token_use checked by caller |
| legacy skip exact OR subtree for each entry | exact unless final slash declares subtree |
| Bearer case-sensitive | case-insensitive scheme; one configured header |
| CommonApiError ER-401/details.reason | fixed problem+json or caller-owned OnError |
| all verification errors mapped401 | key-set unavailable503, unexpected failure500 |
| RequireGroup chi middleware | RequiredGroups OR, or jwt.RequireGroup in handler |

ClaimsFromContext returns a value and bool. ContextWithClaims is for trusted application composition/handler tests, not proof of a valid JWT; its parent context must be nonnil and Custom must be an acyclic JSON tree. The application owns any custom OnError's response privacy, WWW-Authenticate challenge and writer-failure handling. Default responses never publish errors or tokens. There is no automatic resource Close, router registration, SDK selection or logger.
