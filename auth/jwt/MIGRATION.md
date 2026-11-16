# Migration — experimental candidate

No production in pkg/router or aws/pkg/clients/cognito is changed. No aliases, compatibility bridges or deprecations.

| Before | Explicit replacement |
|---|---|
| JWTAuthConfig combines network and middleware | jwt.Config plus separate jwthttp.Config |
| AllowEmptyIssuer/AllowEmptyAudience | required issuer and audience |
| router pins RS256; AWS RSA parser variants | explicit allowlist, RSA/PSS/P-256/P-384 |
| implicit aud OR client_id | AudienceClaim chosen by caller |
| cognito:groups and provider-specific Claims | GroupsClaim plus provider-neutral Claims.Custom |
| router proactive refresh and stale cap6h since fetch | lazy TTL and StaleGrace0 fail closed |
| AWS mutex-held fetch and unbounded fallback | stdlib coalesced refresh and explicit grace |
| HTTPClient default10s | borrowed HTTPClient required |
| legacy path==entry OR entry+slash subtree | exact entry unless a trailing slash declares subtree |
| case-sensitive Bearer/CommonApiError ER-401 | case-insensitive scheme and safe problem JSON/OnError |
| RequireGroup chi middleware | RequireGroup(Claims,group), or HTTP RequiredGroups OR |

Before: `router.JWTAuth(router.JWTAuthConfig{JWKSURL: url, Issuer: issuer, Audience: app})` implicitly constructs the cache/client. After: populate `cfg := jwt.DefaultConfig()`, set required fields and configurable claim names, call `jwt.NewVerifier(cfg,jwt.Dependencies{HTTPClient: client})`, and explicitly pass it to the independent HTTP middleware. Refresh can preload at startup or be called by a health.CheckerFunc; caller context and refresh timeout remain independent budgets.

Claims uses Subject rather than Sub; no implicit Email/Username/TokenUse. Custom preserves unextracted JSON claims and returned trees are independent. For Cognito configure AudienceClaim=client_id for access tokens or aud for ID tokens, GroupsClaim=cognito:groups, explicit pool Issuer/JWKSURL; validate token_use from Custom in the handler. There is no SDK dependency.
