# Changelog

## Experimental (not released)

- D53: stdlib-only outbound HTTP client for a single origin, with a bounded own transport or a borrowed `RoundTripper`, required response size limit, whole-request timeout released at end of body, explicit bounded redirects and stricter credential stripping across origins.
- `NewRequest` with replayable bodies, `CheckStatus`/`StatusError` without body or URL, `WrapTransport` for instrumentation, and protections shared by `HTTPClient()`.
- No retry, breaker, rate limit, logging or telemetry; composition with `resilience` and `client/logging` shown by independent consumers.
- Proposed v0.1.0 unapproved; no published base or legacy migration.
